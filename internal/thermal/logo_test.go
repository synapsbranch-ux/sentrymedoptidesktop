package thermal

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestRasterizeLogoProducesBoundedESCPOSBitmap(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1200, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 1200; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 255), uint8(y % 255), 120, 255})
		}
	}
	var source bytes.Buffer
	if err := png.Encode(&source, img); err != nil {
		t.Fatal(err)
	}
	data, err := RasterizeLogo(source.Bytes(), 384)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 8 || !bytes.Equal(data[:4], []byte{0x1d, 'v', '0', 0}) {
		t.Fatalf("invalid raster header")
	}
	widthBytes := int(data[4]) | int(data[5])<<8
	height := int(data[6]) | int(data[7])<<8
	if widthBytes > 48 || height > maxLogoDotRows || len(data) != 8+widthBytes*height {
		t.Fatalf("unexpected raster dimensions %dx%d, bytes=%d", widthBytes*8, height, len(data))
	}
}

func TestRasterizeLogoGracefullyHandlesAbsentInvalidAndOversized(t *testing.T) {
	if data, err := RasterizeLogo(nil, 384); err != nil || data != nil {
		t.Fatal("absent logo should be skipped")
	}
	if _, err := RasterizeLogo([]byte("not an image"), 384); err == nil {
		t.Fatal("invalid logo accepted")
	}
	if _, err := RasterizeLogo(make([]byte, maxLogoSourceBytes+1), 384); err == nil {
		t.Fatal("oversized logo accepted")
	}
}
