package voice

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// H264FrameEncryptor is deliberately separate from the Opus Session.Encrypt
// API. An adapter must assign H.264 and authenticate a complete access unit;
// the current audio-only dave-go session does not implement this interface.
type H264FrameEncryptor interface {
	EncryptH264(ssrc uint32, accessUnit []byte) ([]byte, error)
}

// VideoWriter owns only RTP video timing, not the account or main voice call.
// It uses the transport's existing write lock and AEAD nonce space.
type VideoWriter struct {
	transport *udpConnImpl
	ssrc      uint32
	sequence  uint16
}

func (u *udpConnImpl) NewVideoWriter(ssrc uint32) (*VideoWriter, error) {
	if ssrc == 0 || ssrc == u.ssrc {
		return nil, errors.New("a separately assigned video SSRC is required")
	}
	if _, ok := u.daveSession.(H264FrameEncryptor); !ok {
		return nil, errors.New("authenticated H264 sending is not supported by this DAVE adapter")
	}
	return &VideoWriter{transport: u, ssrc: ssrc}, nil
}

func (w *VideoWriter) Send(ctx context.Context, accessUnit []byte, timestamp uint32) error {
	u := w.transport
	u.writeMu.Lock()
	defer u.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if !u.daveSession.Ready() {
		return errors.New("video encryption epoch is not ready")
	}
	crypt, ok := u.daveSession.(H264FrameEncryptor)
	if !ok {
		return errors.New("authenticated H264 sending unavailable")
	}
	u.connMu.Lock()
	conn := u.conn
	u.connMu.Unlock()
	if conn == nil {
		return errors.New("video transport is closed")
	}
	if _, ok := u.encrypter.(*AEADEncrypter); !ok {
		return errors.New("authenticated video transport is required")
	}
	if len(accessUnit) == 0 || len(accessUnit) > 4<<20 {
		return errors.New("invalid video access unit")
	}
	// Encryption happens before fragmentation. Never call the Opus Encrypt
	// fallback, and never transmit a plaintext frame when the adapter fails.
	encrypted, err := crypt.EncryptH264(w.ssrc, accessUnit)
	if err != nil {
		return fmt.Errorf("video frame encryption: %w", err)
	}
	packets, err := PacketizeH264(encrypted, 1100)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(time.Second)
	if until, ok := ctx.Deadline(); ok && until.Before(deadline) {
		deadline = until
	}
	if err := conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	defer conn.SetWriteDeadline(time.Time{})
	for i, payload := range packets {
		if err := ctx.Err(); err != nil {
			return err
		}
		var header [12]byte
		header[0], header[1] = 0x80, H264PayloadType
		if i == len(packets)-1 {
			header[1] |= 0x80
		}
		binary.BigEndian.PutUint16(header[2:4], w.sequence)
		w.sequence++
		binary.BigEndian.PutUint32(header[4:8], timestamp)
		binary.BigEndian.PutUint32(header[8:12], w.ssrc)
		if _, err := conn.Write(u.encrypter.Encrypt(header, payload)); err != nil {
			return err
		}
	}
	return nil
}
