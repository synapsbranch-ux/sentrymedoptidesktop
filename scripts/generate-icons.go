package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

func main() {
	targets := []struct {
		size int
		path string
	}{
		{192, filepath.Join("apps", "web", "public", "icon-192.png")},
		{512, filepath.Join("apps", "web", "public", "icon-512.png")},
		{1024, filepath.Join("build", "appicon.png")},
	}
	for _, target := range targets {
		size := target.size
		canvas := image.NewRGBA(image.Rect(0, 0, size, size))
		black := color.RGBA{0, 0, 0, 255}
		white := color.RGBA{255, 255, 255, 255}
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				canvas.Set(x, y, black)
			}
		}
		cx, cy := float64(size)/2, float64(size)/2
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				dx, dy := (float64(x)-cx)/(float64(size)*0.35), (float64(y)-cy)/(float64(size)*0.20)
				distance := math.Sqrt(dx*dx + dy*dy)
				if distance > 0.88 && distance < 1.0 {
					canvas.Set(x, y, white)
				}
				if math.Hypot(float64(x)-cx, float64(y)-cy) < float64(size)*0.105 {
					canvas.Set(x, y, white)
				}
			}
		}
		if err := os.MkdirAll(filepath.Dir(target.path), 0o750); err != nil {
			log.Fatal(err)
		}
		file, err := os.Create(target.path)
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(file, canvas); err != nil {
			_ = file.Close()
			log.Fatal(err)
		}
		if err := file.Close(); err != nil {
			log.Fatal(err)
		}
	}
}
