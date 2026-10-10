package handler

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"math"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed template/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// swVersion 由内嵌静态资源内容求得：任何前端资产（SW/样式/清单）一变，版本即变，
// activate 阶段据此淘汰旧缓存。取代过去写死的 `yuexi-v3`。
var swVersion = func() string {
	h := sha256.New()
	_ = fs.WalkDir(staticFS, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, rerr := staticFS.ReadFile(path)
		if rerr != nil {
			return nil
		}
		h.Write([]byte(path))
		h.Write(b)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))[:12]
}()

func ServeManifest(w http.ResponseWriter, r *http.Request) {
	data, _ := staticFS.ReadFile("static/manifest.json")
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

// ServeSW 下发 Service Worker，并把 __BUILD_VERSION__ 替换为内容版本号。
func ServeSW(w http.ResponseWriter, r *http.Request) {
	data, _ := staticFS.ReadFile("static/sw.js")
	body := bytes.ReplaceAll(data, []byte("__BUILD_VERSION__"), []byte(swVersion))
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(body)
}

// ServeStatic serves embedded build assets (CSS/JS) under /static/*. This keeps
// third-party front-end libraries (Tailwind, Alpine, Chart.js) served from the
// same origin instead of an external CDN, removing the render-blocking network
// dependency and improving cache locality behind nginx.
func ServeStatic(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/static/")
	if name == "" || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	data, err := staticFS.ReadFile("static/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

// iconCache holds pre-rendered PNG bytes keyed by "s<size>"(普通) / "m<size>"(maskable)，
// 避免每次请求都跑一遍逐像素生成。
var iconCache sync.Map // string -> []byte

func cachedIcon(key string, gen func() image.Image) []byte {
	if v, ok := iconCache.Load(key); ok {
		return v.([]byte)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, gen()); err != nil {
		return nil // fall back to on-the-fly encoding
	}
	b := buf.Bytes()
	iconCache.Store(key, b)
	return b
}

func writeIcon(w http.ResponseWriter, key string, gen func() image.Image) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if b := cachedIcon(key, gen); b != nil {
		w.Write(b)
		return
	}
	png.Encode(w, gen())
}

func ServeIcon(w http.ResponseWriter, r *http.Request, size int) {
	writeIcon(w, fmt.Sprintf("s%d", size), func() image.Image { return generateIcon(size) })
}

// ServeMaskableIcon 供 manifest 的 purpose=maskable 使用：满幅背景 + 主体收进安全圆。
func ServeMaskableIcon(w http.ResponseWriter, r *http.Request, size int) {
	writeIcon(w, fmt.Sprintf("m%d", size), func() image.Image { return generateMaskableIcon(size) })
}

func ServeFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "no-cache")
	if b := cachedIcon("s32", func() image.Image { return generateIcon(32) }); b != nil {
		w.Write(b)
		return
	}
	png.Encode(w, generateIcon(32))
}

// ServeFaviconSVG 下发矢量 favicon（静态文件，几何参数与 renderIcon 保持一致）。
func ServeFaviconSVG(w http.ResponseWriter, r *http.Request) {
	data, err := staticFS.ReadFile("static/favicon.svg")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

func generateIcon(size int) image.Image { return renderIcon(size, false) }

func generateMaskableIcon(size int) image.Image { return renderIcon(size, true) }

// blendWhite 把白色按 alpha 混合到已有像素上（而不是直接覆盖）。
// 直接 Set 会让抗锯齿边缘变成"半透明白"，在 maskable 下破坏满幅不透明，
// 在普通图标下也会丢掉月亮/波浪下方的渐变底色。
func blendWhite(img *image.RGBA, x, y int, alpha float64) {
	if alpha <= 0 || x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
		return
	}
	if alpha > 1 {
		alpha = 1
	}
	old := img.RGBAAt(x, y)
	mix := func(c uint8) uint8 {
		return uint8(float64(c)*(1-alpha) + 255*alpha + 0.5)
	}
	img.SetRGBA(x, y, color.RGBA{mix(old.R), mix(old.G), mix(old.B), mix(old.A)})
}

// renderIcon 绘制月汐图标。
//
// maskable 模式下的两点差异（见 PWA规范.md 第一节）：
//   - 背景铺满整幅、不留透明角，满足 Android 自适应图标要求；
//   - 月亮与波浪整体缩到中心 80% 直径的安全圆内，避免被系统裁切成残缺。
func renderIcon(size int, maskable bool) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cx, cy := float64(size)/2, float64(size)/2
	r := float64(size) / 2

	// Clean gradient background - soft pink to rose
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			dist := math.Sqrt(dx*dx + dy*dy)
			if !maskable && dist > r {
				continue
			}
			// Smooth radial gradient from center
			t := dist / r
			if t > 1 {
				t = 1
			}
			// Center: soft pink, Edge: deeper rose
			cr := uint8(244 - t*30) // 244 -> 214
			cg := uint8(114 - t*50) // 114 -> 64
			cb := uint8(158 - t*40) // 158 -> 118
			img.Set(x, y, color.RGBA{cr, cg, cb, 255})
		}
	}

	// maskable：主体缩到安全区，绘制范围也收进安全圆
	scale := 1.0
	clipR := r - 1
	if maskable {
		scale = 0.72
		clipR = r * 0.8 // 安全圆直径 = 80%
	}

	// Draw crescent moon - larger, centered upper area
	moonCx := cx - float64(size)*0.05*scale
	moonCy := cy - float64(size)*0.15*scale
	moonR := float64(size) * 0.22 * scale
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
					blendWhite(img, x, y, alpha)
				}
			}
		}
	}

	// Draw 2 minimal wave lines - clean and modern
	waveThickness := float64(size) * 0.035 * scale
	if waveThickness < 2 {
		waveThickness = 2
	}

	waves := []struct {
		amp, freq, phase, yRel, alpha float64
	}{
		{float64(size) * 0.04, 0.028, 0.5, 0.08, 0.5},
		{float64(size) * 0.05, 0.032, 1.8, 0.20, 0.75},
	}

	for _, w := range waves {
		yBase := cy + float64(size)*w.yRel*scale
		amp := w.amp * scale
		for x := 0; x < size; x++ {
			waveY := yBase + amp*math.Sin(w.freq*float64(x)+w.phase)
			for dy := -waveThickness / 2; dy <= waveThickness/2; dy++ {
				px, py := x, int(waveY+dy)
				if py >= 0 && py < size {
					ddx, ddy := float64(px)-cx, float64(py)-cy
					if math.Sqrt(ddx*ddx+ddy*ddy) <= clipR {
						blendWhite(img, px, py, w.alpha)
					}
				}
			}
		}
	}

	return img
}
