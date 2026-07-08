package handler

import (
	"embed"
	"image"
	"image/color"
	"image/png"
	"math"
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

func ServeFavicon(w http.ResponseWriter, r *http.Request) {
	img := generateIcon(32)
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "no-cache")
	png.Encode(w, img)
}

func generateIcon(size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cx, cy := float64(size)/2, float64(size)/2
	r := float64(size) / 2

	// Clean gradient background - soft pink to rose
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist <= r {
				// Smooth radial gradient from center
				t := dist / r
				// Center: soft pink, Edge: deeper rose
				cr := uint8(244 - t*30)  // 244 -> 214
				cg := uint8(114 - t*50)  // 114 -> 64
				cb := uint8(158 - t*40)  // 158 -> 118
				img.Set(x, y, color.RGBA{cr, cg, cb, 255})
			}
		}
	}

	// Draw crescent moon - larger, centered upper area
	moonCx := cx - float64(size)*0.05
	moonCy := cy - float64(size)*0.15
	moonR := float64(size) * 0.22
	shiftX := moonR * 0.4
	shiftY := -moonR * 0.1

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)-moonCx, float64(y)-moonCy
			distMain := math.Sqrt(dx*dx + dy*dy)
			dx2 := float64(x) - (moonCx + shiftX)
			dy2 := float64(y) - (moonCy + shiftY)
			distShift := math.Sqrt(dx2*dx2 + dy2*dy2)

			if distMain <= moonR && distShift > moonR*0.75 {
				alpha := 1.0
				// Anti-alias outer edge
				if distMain > moonR-1.2 {
					alpha = (moonR - distMain + 1.2) / 1.2
				}
				// Anti-alias inner edge
				if distShift < moonR*0.75+1.2 {
					a2 := (distShift - moonR*0.75 + 1.2) / 1.2
					if a2 < alpha {
						alpha = a2
					}
				}
				if alpha > 0 {
					a := uint8(255 * alpha)
					img.Set(x, y, color.RGBA{255, 255, 255, a})
				}
			}
		}
	}

	// Draw 2 minimal wave lines - clean and modern
	waveThickness := float64(size) * 0.035
	if waveThickness < 2 {
		waveThickness = 2
	}

	waves := []struct {
		amp, freq, phase, yBase, alpha float64
	}{
		{float64(size) * 0.04, 0.028, 0.5, float64(size) * 0.58, 0.5},
		{float64(size) * 0.05, 0.032, 1.8, float64(size) * 0.70, 0.75},
	}

	for _, w := range waves {
		for x := 0; x < size; x++ {
			waveY := w.yBase + w.amp*math.Sin(w.freq*float64(x)+w.phase)
			for dy := -waveThickness / 2; dy <= waveThickness/2; dy++ {
				px, py := x, int(waveY+dy)
				if py >= 0 && py < size {
					ddx, ddy := float64(px)-cx, float64(py)-cy
					if math.Sqrt(ddx*ddx+ddy*ddy) <= r-1 {
						a := uint8(255 * w.alpha)
						img.Set(px, py, color.RGBA{255, 255, 255, a})
					}
				}
			}
		}
	}

	return img
}
