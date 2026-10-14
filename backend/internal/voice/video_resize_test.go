package voice

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestVideoResizeCapsWithoutUpscaling(t *testing.T) {
	for _, gray := range []bool{false, true} {
		var input image.Image = image.NewYCbCr(image.Rect(0, 0, 1280, 720), image.YCbCrSubsampleRatio420)
		if gray {
			input = image.NewGray(image.Rect(0, 0, 1280, 720))
		}
		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, input, &jpeg.Options{Quality: 92}); err != nil {
			t.Fatal(err)
		}
		out := resizeVideoJPEG(encoded.Bytes(), 360)
		pic, err := jpeg.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		if pic.Bounds().Dx() != 640 || pic.Bounds().Dy() != 360 {
			t.Fatal(pic.Bounds())
		}
		if !bytes.Equal(resizeVideoJPEG(out, 1080), out) {
			t.Fatal("small source was upscaled")
		}
	}
	var encoded bytes.Buffer
	pixels := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	for y := 0; y < 720; y++ {
		for x := 0; x < 1280; x++ {
			pixels.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	jpeg.Encode(&encoded, pixels, &jpeg.Options{Quality: 92})
	pic, err := jpeg.Decode(bytes.NewReader(resizeVideoJPEG(encoded.Bytes(), 360)))
	if err != nil {
		t.Fatal(err)
	}
	r, g, _, _ := pic.At(100, 100).RGBA()
	if r == 0 || g == 0 {
		t.Fatal("color planes lost")
	}
}

func BenchmarkVideoResize360(b *testing.B) {
	p := image.NewYCbCr(image.Rect(0, 0, 1920, 1080), image.YCbCrSubsampleRatio420)
	var raw bytes.Buffer
	jpeg.Encode(&raw, p, &jpeg.Options{Quality: 92})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resizeVideoJPEG(raw.Bytes(), 360)
	}
}
