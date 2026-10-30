package voice

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCaptureCancellationDuringSelectionAndBackpressure(t *testing.T) {
	for _, body := range []string{
		"import time\ntime.sleep(60)\n",
		"import os, struct\nwhile True:\n os.write(1,struct.pack('!IQ',1024,0)+b'x'*1024)\n",
	} {
		helper := filepath.Join(t.TempDir(), "capture.py")
		if err := os.WriteFile(helper, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		capture, err := startCapture(ctx, helper, 720, 30)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
		cancel()
		done := make(chan struct{})
		go func() { capture.Close(); close(done) }()
		select {
		case <-done:
		case <-time.After(4 * time.Second):
			t.Fatal("capture did not reap after cancellation")
		}
		if capture.cmd.ProcessState == nil {
			t.Fatal("capture process was not reaped")
		}
	}
}
