package voice

import (
	"errors"
	"time"

	"github.com/pion/rtcp"
)

// SendNACK requests only explicitly missing primary sequences, bounded per
// receive interval by the assembler. Authentication uses the transport key.
func (u *udpConnImpl) SendNACK(ssrc uint32, sequences []uint16) error {
	if len(sequences) == 0 {
		return nil
	}
	if len(sequences) > 16 {
		sequences = sequences[:16]
	}
	nacks := make([]rtcp.NackPair, len(sequences))
	for i, seq := range sequences {
		nacks[i] = rtcp.NackPair{PacketID: seq}
	}
	packet, err := (rtcp.TransportLayerNack{SenderSSRC: u.ssrc, MediaSSRC: ssrc, Nacks: nacks}).Marshal()
	if err != nil {
		return err
	}
	u.writeMu.Lock()
	defer u.writeMu.Unlock()
	u.connMu.Lock()
	conn := u.conn
	u.connMu.Unlock()
	a, ok := u.encrypter.(*AEADEncrypter)
	if !ok || conn == nil {
		return errors.New("video transport not ready")
	}
	_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
	defer conn.SetWriteDeadline(time.Time{})
	_, err = conn.Write(a.encryptHeader(packet[:8], packet[8:]))
	if err == nil {
		u.cameraStage("nack_sent")
	}
	return err
}
