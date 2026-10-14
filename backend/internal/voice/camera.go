package voice

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os/exec"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/disgoorg/godave"
	"github.com/disgoorg/snowflake/v2"
	dvoice "github.com/mattcalayo/omarchy-discord/backend/internal/voicewire"
)

// ponytail: one bounded H264 viewer; no recording or other-codec fallback.
const cameraFrameLimit = 4 << 20

type CameraStream struct {
	UserID string `json:"user_id"`
	SSRC   uint32 `json:"ssrc"`
}
type CameraSnapshot struct {
	Streams          []CameraStream `json:"streams"`
	UserID           string         `json:"user_id"`
	Image            string         `json:"image"`
	Sequence         uint64         `json:"sequence"`
	Packets          uint64         `json:"packets"`
	Dispatched       uint64         `json:"dispatched"`
	SSRCMismatch     uint64         `json:"ssrc_mismatch"`
	PacketAgeMS      int64          `json:"packet_age_ms"`
	AgeMS            int64          `json:"age_ms"`
	Error            string         `json:"error"`
	AssemblyFrames   uint64         `json:"assembly_frames"`
	AssemblyDropped  uint64         `json:"assembly_dropped"`
	ReorderWaits     uint64         `json:"reorder_waits"`
	DuplicatePackets uint64         `json:"duplicate_packets"`
	Quality          int            `json:"quality"`
}
type camera struct {
	mu           sync.Mutex
	streams      map[snowflake.ID]uint32
	videoSSRCs   map[uint32]bool
	audioSSRCs   map[uint32]bool
	selected     snowflake.ID
	ssrc         uint32
	decoder      *cameraDecoder
	packets      uint64
	dispatched   uint64
	ssrcMismatch uint64
	packetAt     time.Time
	err          string
	assembly     h264Assembly
	quality      int
}

func newCamera() *camera {
	return &camera{streams: make(map[snowflake.ID]uint32), videoSSRCs: make(map[uint32]bool), audioSSRCs: make(map[uint32]bool)}
}
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
	if d.AudioSSRC != 0 && len(v.audioSSRCs) < 512 {
		v.audioSSRCs[d.AudioSSRC] = true
	}
	if d.VideoSSRC != 0 && len(v.videoSSRCs) < 1536 {
		v.videoSSRCs[d.VideoSSRC] = true
	}
	for _, stream := range d.Streams {
		if stream.SSRC != 0 && len(v.videoSSRCs) < 1536 {
			v.videoSSRCs[stream.SSRC] = true
		}
	}
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
		v.assembly.reset()
		if v.decoder != nil {
			v.decoder.setError("Camera changed. Select it again.")
		}
	}
}
func (v *camera) announceAudio(ssrc uint32) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if ssrc != 0 && len(v.audioSSRCs) < 512 {
		v.audioSSRCs[ssrc] = true
	}
}
func (v *camera) remove(id snowflake.ID) { v.announce(dvoice.GatewayMessageDataVideo{UserID: id}) }
func (v *camera) close() {
	v.mu.Lock()
	d := v.decoder
	v.decoder = nil
	v.selected = 0
	v.ssrc = 0
	v.err = ""
	v.packets = 0
	v.packetAt = time.Time{}
	v.assembly.reset()
	v.mu.Unlock()
	if d != nil {
		d.close()
	}
}
func (v *camera) fail(message string) { v.mu.Lock(); v.err = message; v.mu.Unlock() }
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
	v.mu.Lock()
	quality := v.quality
	v.mu.Unlock()
	d, err := newCameraDecoderQuality(quality)
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
	s := CameraSnapshot{Streams: []CameraStream{}, AgeMS: -1, Packets: v.packets, Dispatched: v.dispatched, SSRCMismatch: v.ssrcMismatch, PacketAgeMS: -1}
	s.Quality = v.quality
	if s.Quality == 0 {
		s.Quality = 360
	}
	s.AssemblyFrames = v.assembly.completed
	s.AssemblyDropped = v.assembly.dropped
	s.ReorderWaits = v.assembly.reordered
	s.DuplicatePackets = v.assembly.duplicates
	if !v.packetAt.IsZero() {
		s.PacketAgeMS = time.Since(v.packetAt).Milliseconds()
	}
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
	if v.err != "" {
		s.Error = v.err
		s.Image = ""
	}
	return s
}
func (v *camera) missingPackets() (uint32, []uint16) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.ssrc, v.assembly.missing(time.Now())
}
func (v *camera) receive(p *dvoice.Packet, ds godave.Session) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.dispatched++
	if p.SSRC != v.ssrc || v.decoder == nil {
		v.ssrcMismatch++
		return
	}
	v.packets++
	v.packetAt = time.Now()
	frame, err := v.assembly.push(p, time.Now())
	if err != nil {
		v.decoder.setError(err.Error())
		return
	}
	if len(frame) == 0 {
		return
	}
	plain := make([]byte, ds.MaxDecryptedFrameSize(godave.UserID(v.selected.String()), len(frame)))
	n, err := ds.Decrypt(godave.UserID(v.selected.String()), frame, plain)
	if err != nil {
		v.decoder.setError("Camera frame decryption failed")
		return
	}
	v.assembly.accept(v.assembly.candidateTimestamp)
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

// Never set a global "any":0: unspecified media includes the call's audio.
// Disable only announced video SSRCs; keep the server's audio subscription intact.
func (v *camera) sinkWants(selected uint32) dvoice.GatewayMessageDataUnknown {
	v.mu.Lock()
	defer v.mu.Unlock()
	wants := make(map[string]any, len(v.videoSSRCs)+1)
	for ssrc := range v.videoSSRCs {
		wants[fmt.Sprint(ssrc)] = 0
	}
	for ssrc := range v.audioSSRCs {
		wants[fmt.Sprint(ssrc)] = 100
	}
	if selected != 0 {
		wants[fmt.Sprint(selected)] = 100
		// The window's output cap must not request a low-detail remote layer.
		wants["pixelCounts"] = map[string]int{fmt.Sprint(selected): 1920 * 1080}
	}
	data, _ := json.Marshal(wants)
	return dvoice.GatewayMessageDataUnknown(data)
}

type cameraDecoder struct {
	quality  atomic.Int64
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

func newCameraDecoder() (*cameraDecoder, error) { return newCameraDecoderQuality(360) }
func newCameraDecoderQuality(height int) (*cameraDecoder, error) {
	if height == 0 {
		height = 360
	}
	if height != 360 && height != 720 && height != 1080 {
		return nil, errors.New("choose 360, 720 or 1080 quality")
	}
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("camera viewing requires ffmpeg")
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &cameraDecoder{cancel: cancel, frames: make(chan []byte, 4), done: make(chan struct{})}
	d.quality.Store(int64(height))
	d.cmd = exec.CommandContext(ctx, path, "-hide_banner", "-loglevel", "error", "-threads", "1", "-probesize", "32", "-analyzeduration", "0", "-protocol_whitelist", "pipe", "-max_pixels", "8294400", "-f", "h264", "-i", "pipe:0", "-an", "-vf", "scale=w='min(1920,iw)':h='min(1080,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2", "-threads", "1", "-f", "image2pipe", "-vcodec", "mjpeg", "-q:v", "2", "pipe:1")
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
				due := time.Since(d.at) >= time.Second/60
				d.mu.Unlock()
				if due {
					resized := resizeVideoJPEG(b, int(d.quality.Load()))
					d.mu.Lock()
					d.image = resized
					d.sequence++
					d.at = time.Now()
					d.err = ""
					d.mu.Unlock()
				}
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
		return CameraSnapshot{Streams: []CameraStream{}, AgeMS: -1, PacketAgeMS: -1}
	}
	return v.snapshot()
}
func (e *Engine) WatchCamera(ctx context.Context, id snowflake.ID, revision uint64) error {
	e.opMu.Lock()
	defer e.opMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
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
	if err := ctx.Err(); err != nil {
		v.close()
		return err
	}
	wants := v.sinkWants(ssrc)
	if err = c.Gateway().Send(ctx, dvoice.OpcodeVideo, dvoice.GatewayMessageDataVideo{AudioSSRC: c.Gateway().SSRC(), Streams: []dvoice.VideoStream{}}); err == nil {
		err = c.Gateway().Send(ctx, dvoice.OpcodeMediaSinkWants, wants)
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

func resizeVideoJPEG(b []byte, height int) []byte {
	source, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		return b
	}
	bounds := source.Bounds()
	if bounds.Dy() <= height {
		return b
	}
	width := bounds.Dx() * height / bounds.Dy()
	// jpeg.Decode produces YCbCr. Scale its planes directly instead of doing
	// hundreds of thousands of interface color conversions for each picture.
	scaled := image.NewYCbCr(image.Rect(0, 0, width, height), image.YCbCrSubsampleRatio420)
	pixels, ok := source.(*image.YCbCr)
	if !ok {
		pixels = image.NewYCbCr(bounds, image.YCbCrSubsampleRatio444)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				value := color.YCbCrModel.Convert(source.At(x, y)).(color.YCbCr)
				pixels.Y[pixels.YOffset(x, y)] = value.Y
				at := pixels.COffset(x, y)
				pixels.Cb[at], pixels.Cr[at] = value.Cb, value.Cr
			}
		}
	}
	for y := 0; y < height; y++ {
		sy := bounds.Min.Y + y*bounds.Dy()/height
		for x := 0; x < width; x++ {
			sx := bounds.Min.X + x*bounds.Dx()/width
			scaled.Y[scaled.YOffset(x, y)] = pixels.Y[pixels.YOffset(sx, sy)]
			if x%2 == 0 && y%2 == 0 {
				dst, src := scaled.COffset(x, y), pixels.COffset(sx, sy)
				scaled.Cb[dst] = pixels.Cb[src]
				scaled.Cr[dst] = pixels.Cr[src]
			}
		}
	}
	var out bytes.Buffer
	if jpeg.Encode(&out, scaled, &jpeg.Options{Quality: 92}) != nil {
		return b
	}
	return out.Bytes()
}
