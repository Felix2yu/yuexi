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

	// Deep pink-to-rose gradient background circle
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist <= r {
				t := float64(y) / float64(size)
				cr := uint8(220 + t*36)  // 220 -> 256 (clamped)
				cg := uint8(40 + t*32)   // 40 -> 72
				cb := uint8(100 + t*53)  // 100 -> 153
				if cr > 236 {
					cr = 236
				}
				img.Set(x, y, color.RGBA{cr, cg, cb, 255})
			}
		}
	}

	// Draw 3 flowing wave curves (BitTorrent style)
	waveThickness := float64(size) * 0.045
	if waveThickness < 2 {
		waveThickness = 2
	}

	// Wave parameters: amplitude, frequency, phase, vertical position
	waves := []struct {
		amp, freq, phase, yBase, alpha float64
	}{
		{float64(size) * 0.06, 0.035, 0.0, float64(size) * 0.48, 0.55},
		{float64(size) * 0.07, 0.040, 1.2, float64(size) * 0.56, 0.70},
		{float64(size) * 0.08, 0.045, 2.4, float64(size) * 0.65, 0.90},
	}

	for _, w := range waves {
		for x := 0; x < size; x++ {
			waveY := w.yBase + w.amp*math.Sin(w.freq*float64(x)+w.phase)
			for dy := -waveThickness / 2; dy <= waveThickness/2; dy++ {
				px, py := x, int(waveY+dy)
				if py >= 0 && py < size {
					// Check if inside circle
					ddx, ddy := float64(px)-cx, float64(py)-cy
					if math.Sqrt(ddx*ddx+ddy*ddy) <= r-1 {
						a := uint8(255 * w.alpha)
						img.Set(px, py, color.RGBA{255, 255, 255, a})
					}
				}
			}
		}
	}

	// Draw crescent moon in upper-right area
	moonCx := cx + float64(size)*0.12
	moonCy := cy - float64(size)*0.18
	moonR := float64(size) * 0.16
	shiftX := moonR * 0.35

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)-moonCx, float64(y)-moonCy
			distMain := math.Sqrt(dx*dx + dy*dy)
			// Shifted circle for subtraction
			dx2 := float64(x) - (moonCx + shiftX)
			distShift := math.Sqrt(dx2*dx2 + dy*dy)

			if distMain <= moonR && distShift > moonR*0.82 {
				// Anti-aliasing at the edge
				alpha := 1.0
				if distMain > moonR-1.5 {
					alpha = (moonR - distMain + 1.5) / 1.5
				}
				if distShift < moonR*0.82+1.5 {
					a2 := (distShift - moonR*0.82 + 1.5) / 1.5
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

	return img
}
