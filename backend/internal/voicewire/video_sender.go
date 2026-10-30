package voice

import (
	"errors"
	"net"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// WriteVideoPacket keeps Discord's transport AEAD separate from frame-level
// DAVE. Extension values are encrypted; the extension profile/length is AAD.
func (u *udpConnImpl) WriteVideoPacket(packet *rtp.Packet) error {
	wire, err := packet.Marshal()
	if err != nil {
		return err
	}
	headerSize := RTPHeaderSize + 4*len(packet.CSRC)
	if packet.Extension {
		headerSize += 4
	}
	u.writeMu.Lock()
	defer u.writeMu.Unlock()
	u.connMu.Lock()
	conn := u.conn
	u.connMu.Unlock()
	if conn == nil {
		return net.ErrClosed
	}
	a, ok := u.encrypter.(*AEADEncrypter)
	if !ok {
		return errors.New("video requires negotiated transport encryption")
	}
	if err = conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		return err
	}
	defer conn.SetWriteDeadline(time.Time{})
	_, err = conn.Write(a.encryptHeader(wire[:headerSize], wire[headerSize:]))
	return err
}

func (u *udpConnImpl) SendVideoReport(ssrc, timestamp, packets, octets uint32) {
	now := time.Now()
	ntp := uint64(now.Unix()+2208988800)<<32 | uint64(now.Nanosecond())*(1<<32)/1000000000
	report, err := (&rtcp.SenderReport{SSRC: ssrc, NTPTime: ntp, RTPTime: timestamp, PacketCount: packets, OctetCount: octets}).Marshal()
	if err == nil {
		u.sendFeedback(report, "sender_report_sent")
	}
}

// Only the dedicated outbound transport reads here. Authenticate feedback before
// exposing PLI/NACK; no decrypted media or unknown packet type reaches callbacks.
func (u *udpConnImpl) ReadVideoFeedback() ([]rtcp.Packet, error) {
	u.connMu.Lock()
	conn := u.conn
	u.connMu.Unlock()
	if conn == nil {
		return nil, net.ErrClosed
	}
	for {
		n, err := conn.Read(u.receiveBuffer)
		if err != nil {
			return nil, err
		}
		if n < 12 || u.receiveBuffer[0]>>6 != 2 || u.receiveBuffer[1] < 192 || u.receiveBuffer[1] > 223 {
			continue
		}
		body, err := u.encrypter.Decrypt(8, u.receiveBuffer[:n])
		if err != nil {
			continue
		}
		plain := append(append([]byte(nil), u.receiveBuffer[:8]...), body...)
		packets, err := rtcp.Unmarshal(plain)
		if err != nil {
			continue
		}
		return packets, nil
	}
}
