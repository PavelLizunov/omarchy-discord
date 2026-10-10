package voice

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/disgoorg/godave"
	"github.com/disgoorg/snowflake/v2"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

// ponytail: one H264 camera, no RTX or recording; add recovery/codecs after live acceptance.
const cameraFrameLimit = 4 << 20

type CameraStream struct {
	UserID string `json:"user_id"`
	SSRC   uint32 `json:"ssrc"`
}
type CameraSnapshot struct {
	Streams  []CameraStream `json:"streams"`
	UserID   string         `json:"user_id"`
	Image    string         `json:"image"`
	Sequence uint64         `json:"sequence"`
	AgeMS    int64          `json:"age_ms"`
	Error    string         `json:"error"`
}
type camera struct {
	mu         sync.Mutex
	streams    map[snowflake.ID]uint32
	selected   snowflake.ID
	ssrc       uint32
	decoder    *cameraDecoder
	frame      []byte
	timestamp  uint32
	sequence   uint16
	assembling bool
}

func newCamera() *camera { return &camera{streams: make(map[snowflake.ID]uint32)} }
func (v *camera) announce(d dvoice.GatewayMessageDataVideo) {
	if d.UserID == 0 {
		return
	}
	ssrc := d.VideoSSRC
	for _, stream := range d.Streams {
		if stream.Active && stream.SSRC != 0 {
			ssrc = stream.SSRC
		}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, known := v.streams[d.UserID]; !known && len(v.streams) >= 512 {
		return
	}
	if ssrc == 0 {
		delete(v.streams, d.UserID)
	} else {
		v.streams[d.UserID] = ssrc
	}
	if v.selected == d.UserID && v.ssrc != ssrc {
		v.ssrc = ssrc
		v.frame = nil
		v.assembling = false
		if v.decoder != nil {
			v.decoder.setError("Camera changed. Select it again.")
		}
	}
}
func (v *camera) remove(id snowflake.ID) { v.announce(dvoice.GatewayMessageDataVideo{UserID: id}) }
func (v *camera) close() {
	v.mu.Lock()
	d := v.decoder
	v.decoder = nil
	v.selected = 0
	v.ssrc = 0
	v.frame = nil
	v.assembling = false
	v.mu.Unlock()
	if d != nil {
		d.close()
	}
}
func (v *camera) selectUser(id snowflake.ID) (uint32, error) {
	v.close()
	if id == 0 {
		return 0, nil
	}
	v.mu.Lock()
	ssrc := v.streams[id]
	v.mu.Unlock()
	if ssrc == 0 {
		return 0, errors.New("camera is no longer available")
	}
	d, err := newCameraDecoder()
	if err != nil {
		return 0, err
	}
	v.mu.Lock()
	v.selected = id
	v.ssrc = ssrc
	v.decoder = d
	v.mu.Unlock()
	return ssrc, nil
}
func (v *camera) snapshot() CameraSnapshot {
	v.mu.Lock()
	defer v.mu.Unlock()
	s := CameraSnapshot{Streams: []CameraStream{}, AgeMS: -1}
	for id, ssrc := range v.streams {
		s.Streams = append(s.Streams, CameraStream{id.String(), ssrc})
	}
	sort.Slice(s.Streams, func(i, j int) bool { return s.Streams[i].UserID < s.Streams[j].UserID })
	if v.selected != 0 {
		s.UserID = v.selected.String()
	}
	if d := v.decoder; d != nil {
		d.mu.Lock()
		defer d.mu.Unlock()
		s.Sequence = d.sequence
		s.Error = d.err
		if !d.at.IsZero() {
			s.AgeMS = time.Since(d.at).Milliseconds()
		}
		if s.AgeMS >= 0 && s.AgeMS < 3000 {
			s.Image = base64.StdEncoding.EncodeToString(d.image)
		}
	}
	return s
}
func (v *camera) receive(p *dvoice.Packet, ds godave.Session) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if p.SSRC != v.ssrc || v.decoder == nil {
		return
	}
	if !v.assembling || p.Timestamp != v.timestamp {
		v.frame = nil
		v.timestamp = p.Timestamp
		v.sequence = p.Sequence
		v.assembling = true
	} else if p.Sequence != v.sequence+1 {
		v.frame = nil
		v.assembling = false
		return
	} else {
		v.sequence = p.Sequence
	}
	if err := appendH264(&v.frame, p.Opus); err != nil {
		v.frame = nil
		v.assembling = false
		v.decoder.setError(err.Error())
		return
	}
	if !p.Marker {
		return
	}
	frame := v.frame
	v.frame = nil
	v.assembling = false
	plain := make([]byte, ds.MaxDecryptedFrameSize(godave.UserID(v.selected.String()), len(frame)))
	n, err := ds.Decrypt(godave.UserID(v.selected.String()), frame, plain)
	if err != nil {
		v.decoder.setError("Camera frame decryption failed")
		return
	}
	select {
	case v.decoder.frames <- plain[:n]:
	default:
		v.decoder.setError("Camera decoder is behind; select the camera again")
	}
}
func appendH264(frame *[]byte, p []byte) error {
	if len(p) == 0 {
		return errors.New("empty camera packet")
	}
	add := func(nal []byte) { *frame = append(*frame, 0, 0, 0, 1); *frame = append(*frame, nal...) }
	switch p[0] & 31 {
	case 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23:
		add(p)
	case 24:
		for off := 1; off < len(p); {
			if off+2 > len(p) {
				return errors.New("invalid H264 aggregation")
			}
			n := int(p[off])<<8 | int(p[off+1])
			off += 2
			if n == 0 || off+n > len(p) {
				return errors.New("invalid H264 aggregation")
			}
			add(p[off : off+n])
			off += n
		}
	case 28:
		if len(p) < 2 || p[1]&31 == 0 {
			return errors.New("invalid H264 fragment")
		}
		if p[1]&128 != 0 {
			add([]byte{(p[0] & 0xe0) | (p[1] & 31)})
		} else if len(*frame) == 0 {
			return errors.New("incomplete H264 frame")
		}
		*frame = append(*frame, p[2:]...)
	default:
		return errors.New("unsupported camera packet")
	}
	if len(*frame) > cameraFrameLimit {
		return errors.New("camera frame exceeds MVP limit")
	}
	return nil
}

type cameraDecoder struct {
	mu       sync.Mutex
	image    []byte
	sequence uint64
	at       time.Time
	err      string
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	frames   chan []byte
	done     chan struct{}
}

func newCameraDecoder() (*cameraDecoder, error) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("camera viewing requires ffmpeg")
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &cameraDecoder{cancel: cancel, frames: make(chan []byte, 4), done: make(chan struct{})}
	d.cmd = exec.CommandContext(ctx, path, "-hide_banner", "-loglevel", "error", "-threads", "1", "-probesize", "32", "-analyzeduration", "0", "-protocol_whitelist", "pipe", "-max_pixels", "8294400", "-f", "h264", "-i", "pipe:0", "-an", "-vf", "scale=640:360:force_original_aspect_ratio=decrease:force_divisible_by=2", "-threads", "1", "-f", "image2pipe", "-vcodec", "mjpeg", "-q:v", "5", "pipe:1")
	in, err := d.cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	out, err := d.cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err = d.cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start camera decoder: %w", err)
	}
	go func() {
		defer in.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case f := <-d.frames:
				if _, err := in.Write(f); err != nil {
					return
				}
			}
		}
	}()
	go func() {
		d.readImages(out)
		err := d.cmd.Wait()
		if ctx.Err() == nil && err != nil {
			d.setError("Camera decoder stopped")
		}
		cancel()
		close(d.done)
	}()
	return d, nil
}
func (d *cameraDecoder) readImages(out io.Reader) {
	r := bufio.NewReaderSize(out, 65536)
	var b []byte
	prev := byte(0)
	for {
		c, err := r.ReadByte()
		if err != nil {
			return
		}
		if len(b) == 0 {
			if prev == 0xff && c == 0xd8 {
				b = []byte{0xff, 0xd8}
			}
		} else {
			b = append(b, c)
			if len(b) > cameraFrameLimit {
				d.setError("Decoded camera image exceeds limit")
				d.cancel()
				return
			}
			if prev == 0xff && c == 0xd9 {
				d.mu.Lock()
				d.image = b
				d.sequence++
				d.at = time.Now()
				d.err = ""
				d.mu.Unlock()
				b = nil
			}
		}
		prev = c
	}
}
func (d *cameraDecoder) setError(s string) { d.mu.Lock(); d.err = s; d.mu.Unlock() }
func (d *cameraDecoder) close()            { d.cancel(); <-d.done }

func (e *Engine) Cameras() CameraSnapshot {
	e.mu.Lock()
	v := e.video
	e.mu.Unlock()
	if v == nil {
		return CameraSnapshot{Streams: []CameraStream{}, AgeMS: -1}
	}
	return v.snapshot()
}
func (e *Engine) WatchCamera(ctx context.Context, id snowflake.ID, revision uint64) error {
	e.opMu.Lock()
	defer e.opMu.Unlock()
	e.mu.Lock()
	if revision == 0 || revision <= e.watchRevision {
		e.mu.Unlock()
		return errors.New("camera request was superseded")
	}
	e.watchRevision = revision
	v, c, status := e.video, e.conn, e.st.Status
	e.mu.Unlock()
	if v == nil || c == nil || status != StatusConnected {
		return errors.New("join voice before watching a camera")
	}
	ssrc, err := v.selectUser(id)
	if err != nil {
		return err
	}
	wants := fmt.Sprintf(`{"any":0,"%d":100}`, ssrc)
	if ssrc == 0 {
		wants = `{"any":0}`
	}
	if err = c.Gateway().Send(ctx, dvoice.OpcodeVideo, dvoice.GatewayMessageDataVideo{AudioSSRC: c.Gateway().SSRC(), Streams: []dvoice.VideoStream{}}); err == nil {
		err = c.Gateway().Send(ctx, dvoice.OpcodeMediaSinkWants, dvoice.GatewayMessageDataUnknown(wants))
	}
	if err != nil {
		v.close()
		return err
	}
	if ssrc != 0 {
		if pli, ok := c.UDP().(interface{ SendPLI(uint32) error }); ok {
			if err = pli.SendPLI(ssrc); err != nil {
				v.close()
				return err
			}
		}
	}
	return nil
}
