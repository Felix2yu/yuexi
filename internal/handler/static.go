package handler

import (
	"embed"
	"image"
	"image/color"
	"image/png"
	"net/http"
)

//go:embed template/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

func ServeManifest(w http.ResponseWriter, r *http.Request) {
	data, _ := staticFS.ReadFile("static/manifest.json")
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Write(data)
}

func ServeSW(w http.ResponseWriter, r *http.Request) {
	data, _ := staticFS.ReadFile("static/sw.js")
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

func ServeIcon(w http.ResponseWriter, r *http.Request, size int) {
	img := generateIcon(size)
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	png.Encode(w, img)
}

func generateIcon(size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Fill with pink background
	bgColor := color.RGBA{236, 72, 153, 255} // pink-500
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, bgColor)
		}
	}

	// Draw a simple rounded rectangle border effect
	borderColor := color.RGBA{219, 39, 119, 255} // pink-600
	radius := size / 5
	thickness := size / 16
	if thickness < 1 {
		thickness = 1
	}

	// Top and bottom edges
	for x := radius; x < size-radius; x++ {
		for t := 0; t < thickness; t++ {
			img.Set(x, t, borderColor)
			img.Set(x, size-1-t, borderColor)
		}
	}
	// Left and right edges
	for y := radius; y < size-radius; y++ {
		for t := 0; t < thickness; t++ {
			img.Set(t, y, borderColor)
			img.Set(size-1-t, y, borderColor)
		}
	}
	// Corner arcs (simplified)
	for dy := 0; dy < radius; dy++ {
		for dx := 0; dx < radius; dx++ {
			if (dx-radius)*(dx-radius)+(dy-radius)*(dy-radius) <= radius*radius {
				for t := 0; t < thickness; t++ {
					// Top-left
					img.Set(dx, dy, borderColor)
					// Top-right
					img.Set(size-1-dx, dy, borderColor)
					// Bottom-left
					img.Set(dx, size-1-dy, borderColor)
					// Bottom-right
					img.Set(size-1-dx, size-1-dy, borderColor)
				}
			}
		}
	}

	// Draw a wave/crescent shape in the center (simplified moon icon)
	centerX, centerY := size/2, size/2
	waveColor := color.RGBA{255, 255, 255, 255}
	waveR := size / 4

	for y := -waveR; y <= waveR; y++ {
		for x := -waveR; x <= waveR; x++ {
			dist := x*x + y*y
			if dist <= waveR*waveR {
				// Create crescent by subtracting a shifted circle
				shiftX := x - waveR/3
				shiftDist := shiftX*shiftX + y*y
				if shiftDist > (waveR-2)*(waveR-2) || shiftX < 0 {
					px, py := centerX+x, centerY+y
					if px >= 0 && px < size && py >= 0 && py < size {
						img.Set(px, py, waveColor)
					}
				}
			}
		}
	}

	return img
}
