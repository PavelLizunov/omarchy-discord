package voice

import (
	"encoding/binary"
	"errors"
	"time"
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
	packet := make([]byte, 12+4*len(sequences))
	packet[0] = 0x81
	packet[1] = 205
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(packet)/4-1))
	binary.BigEndian.PutUint32(packet[4:8], u.ssrc)
	binary.BigEndian.PutUint32(packet[8:12], ssrc)
	for i, seq := range sequences {
		binary.BigEndian.PutUint16(packet[12+i*4:14+i*4], seq)
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
	_, err := conn.Write(a.encryptHeader(packet[:8], packet[8:]))
	if err == nil {
		u.cameraStage("nack_sent")
	}
	return err
}
