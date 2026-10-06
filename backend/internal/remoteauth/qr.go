package remoteauth

import (
	"fmt"

	"github.com/skip2/go-qrcode"
)

func QRPNG(url string, size int) ([]byte, error) {
	if size <= 0 {
		return nil, fmt.Errorf("remoteauth: invalid QR size %d", size)
	}
	png, err := qrcode.Encode(url, qrcode.Medium, size)
	if err != nil {
		return nil, fmt.Errorf("remoteauth: encode QR: %w", err)
	}
	return png, nil
}
