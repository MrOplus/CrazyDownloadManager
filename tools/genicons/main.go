// genicons draws the Crazy Download Manager logo and writes the app icon
// (build/appicon.png), the browser extension icons and the tray icons
// (internal/tray: a Windows .ico and a monochrome macOS template image). Run from the repo root:
//
//	go run ./tools/genicons
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
)

// coverage functions return true when (x, y) in unit space [0,1]² is inside.
func roundRect(x, y, x0, y0, x1, y1, r float64) bool {
	cx := math.Max(x0+r, math.Min(x, x1-r))
	cy := math.Max(y0+r, math.Min(y, y1-r))
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r && x >= x0 && x <= x1 && y >= y0 && y <= y1
}

func triangle(x, y, ax, ay, bx, by, cx, cy float64) bool {
	s := func(px, py, qx, qy, rx, ry float64) float64 { return (px-rx)*(qy-ry) - (qx-rx)*(py-ry) }
	d1, d2, d3 := s(x, y, ax, ay, bx, by), s(x, y, bx, by, cx, cy), s(x, y, cx, cy, ax, ay)
	neg := d1 < 0 || d2 < 0 || d3 < 0
	pos := d1 > 0 || d2 > 0 || d3 > 0
	return !(neg && pos)
}

func lerp(a, b uint8, t float64) float64 { return float64(a) + (float64(b)-float64(a))*t }

// sample returns the colour at a point (premultiplied-free RGBA, alpha 0..1).
func sample(x, y float64) (r, g, b, a float64) {
	if !roundRect(x, y, 0.04, 0.04, 0.96, 0.96, 0.22) {
		return 0, 0, 0, 0
	}
	// background: diagonal blue → violet gradient
	t := (x + y) / 2
	r, g, b = lerp(0x2f, 0x6d, t), lerp(0x8c, 0x3b, t), lerp(0xf0, 0xd6, t)
	a = 1
	// orange "speed" streaks on the left
	for i, yy := range []float64{0.34, 0.47, 0.60} {
		w := 0.20 - float64(i)*0.035
		if roundRect(x, y, 0.13, yy-0.025, 0.13+w, yy+0.025, 0.025) {
			return 0xff, 0xb0, 0x2e, 1
		}
	}
	// white download arrow
	if roundRect(x, y, 0.47, 0.18, 0.63, 0.52, 0.03) || triangle(x, y, 0.33, 0.48, 0.77, 0.48, 0.55, 0.72) {
		return 255, 255, 255, 1
	}
	// tray
	if roundRect(x, y, 0.27, 0.77, 0.83, 0.84, 0.035) {
		return 255, 255, 255, 1
	}
	return
}

// sampleMono is the tray silhouette for the macOS menu bar: a download arrow
// over a tray, black on transparent (the system tints template images).
func sampleMono(x, y float64) (r, g, b, a float64) {
	if roundRect(x, y, 0.41, 0.06, 0.59, 0.50, 0.04) || triangle(x, y, 0.20, 0.44, 0.80, 0.44, 0.50, 0.74) {
		return 0, 0, 0, 1
	}
	if roundRect(x, y, 0.10, 0.84, 0.90, 0.95, 0.05) {
		return 0, 0, 0, 1
	}
	return
}

func render(size int) *image.NRGBA { return renderWith(size, sample) }

func renderWith(size int, sample func(x, y float64) (r, g, b, a float64)) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var R, G, B, A float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) / float64(size)
					y := (float64(py) + (float64(sy)+0.5)/ss) / float64(size)
					r, g, b, a := sample(x, y)
					R += r * a
					G += g * a
					B += b * a
					A += a
				}
			}
			n := float64(ss * ss)
			if A > 0 {
				img.SetNRGBA(px, py, color.NRGBA{uint8(R / A), uint8(G / A), uint8(B / A), uint8(255 * A / n)})
			}
		}
	}
	return img
}

func write(path string, img image.Image) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}

// writeICO writes a Windows icon with uncompressed 32-bit DIB entries, which
// every LoadImage version understands (PNG entries need Vista+ and are only
// common for 256 px).
func writeICO(path string, sizes []int) {
	var dir, data bytes.Buffer
	le := binary.LittleEndian
	_ = binary.Write(&dir, le, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for _, n := range sizes {
		img := render(n)
		var e bytes.Buffer
		// BITMAPINFOHEADER: height is doubled for the XOR + AND masks.
		_ = binary.Write(&e, le, struct {
			Size                  uint32
			Width, Height         int32
			Planes, BitCount      uint16
			Compression, SizeImg  uint32
			XPPM, YPPM            int32
			ClrUsed, ClrImportant uint32
		}{40, int32(n), int32(2 * n), 1, 32, 0, 0, 0, 0, 0, 0})
		for y := n - 1; y >= 0; y-- { // bottom-up BGRA
			for x := 0; x < n; x++ {
				c := img.NRGBAAt(x, y)
				e.Write([]byte{c.B, c.G, c.R, c.A})
			}
		}
		stride := ((n + 31) / 32) * 4 // AND mask: all zero, alpha does the work
		e.Write(make([]byte, stride*n))
		b := uint8(n)
		if n >= 256 {
			b = 0
		}
		_ = binary.Write(&dir, le, struct {
			W, H, Colors, Reserved uint8
			Planes, BitCount       uint16
			Bytes, Offset          uint32
		}{b, b, 0, 0, 1, 32, uint32(e.Len()), uint32(offset)})
		offset += e.Len()
		data.Write(e.Bytes())
	}
	if err := os.WriteFile(path, append(dir.Bytes(), data.Bytes()...), 0o644); err != nil {
		log.Fatal(err)
	}
}

func main() {
	write("build/appicon.png", render(1024))
	for _, s := range []int{16, 32, 48, 128} {
		write(filepath.Join("extension", "chromium", "icons", strconv.Itoa(s)+".png"), render(s))
	}
	writeICO(filepath.Join("internal", "tray", "icon.ico"), []int{16, 20, 24, 32, 48})
	// 18 pt menu bar image at @2x; the tray code sets its size to 18x18.
	write(filepath.Join("internal", "tray", "icon_template.png"), renderWith(36, sampleMono))
	log.Println("icons written")
}
