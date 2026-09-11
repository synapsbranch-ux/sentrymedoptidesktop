package thermal

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const maxLogoSourceBytes = 3 << 20

// RasterizeLogo converts PNG, JPEG or WebP artwork to an ESC/POS GS v 0
// monochrome raster. Dithering keeps gradients readable on a thermal head.
func RasterizeLogo(source []byte, maximumWidth int) ([]byte, error) {
	if len(source) == 0 {
		return nil, nil
	}
	if len(source) > maxLogoSourceBytes {
		return nil, errors.New("logo is too large")
	}
	decoded, _, err := image.Decode(bytes.NewReader(source))
	if err != nil {
		return nil, errors.New("logo is invalid")
	}
	bounds := decoded.Bounds()
	if bounds.Dx() < 1 || bounds.Dy() < 1 {
		return nil, errors.New("logo has no pixels")
	}
	if maximumWidth < 64 || maximumWidth > 832 {
		maximumWidth = 384
	}
	width := min(maximumWidth, bounds.Dx())
	height := max(1, bounds.Dy()*width/bounds.Dx())
	if height > 192 {
		height = 192
		width = max(1, bounds.Dx()*height/bounds.Dy())
	}
	gray := image.NewGray(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(gray, gray.Bounds(), decoded, bounds, draw.Over, nil)
	values := make([]float64, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			values[y*width+x] = float64(color.GrayModel.Convert(gray.At(x, y)).(color.Gray).Y)
		}
	}
	rowBytes := (width + 7) / 8
	raster := make([]byte, rowBytes*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := y*width + x
			old := values[i]
			next := 255.0
			if old < 145 {
				next = 0
			}
			if next == 0 {
				raster[y*rowBytes+x/8] |= byte(0x80 >> uint(x%8))
			}
			e := old - next
			if x+1 < width {
				values[i+1] += e * 7 / 16
			}
			if y+1 < height {
				if x > 0 {
					values[i+width-1] += e * 3 / 16
				}
				values[i+width] += e * 5 / 16
				if x+1 < width {
					values[i+width+1] += e / 16
				}
			}
		}
	}
	header := []byte{0x1d, 'v', '0', 0, byte(rowBytes), byte(rowBytes >> 8), byte(height), byte(height >> 8)}
	return append(header, raster...), nil
}
