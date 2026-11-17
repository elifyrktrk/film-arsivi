// Command genicons draws the app icons (PNG) used by the web manifest and iOS
// "Add to Home Screen". Run from apps/web: go run ./cmd/genicons
// Same motif as static/icon.svg: a poster outline with a marquee light, on the accent color.
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

var (
	accent  = color.RGBA{0x3b, 0x2f, 0xb3, 0xff} // --color-accent (light)
	white   = color.RGBA{0xff, 0xff, 0xff, 0xff}
	marquee = color.RGBA{0xe9, 0xb3, 0x5c, 0xff} // --color-marquee (dark)
)

func main() {
	for _, size := range []int{180, 192, 512} {
		name := "icon-" + itoa(size) + ".png"
		if size == 180 {
			name = "apple-touch-icon.png"
		}
		write(filepath.Join("internal", "web", "static", name), draw(size))
	}
}

// draw renders on a 32-unit grid scaled to size. iOS rounds corners itself, so the
// background is a full square (no transparency).
func draw(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	u := float64(size) / 32
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := (float64(x)+0.5)/u, (float64(y)+0.5)/u
			c := accent
			// poster outline: rect 9..23 x 6..26, stroke 2 (outer 8..24, 5..27)
			outer := fx >= 8 && fx <= 24 && fy >= 5 && fy <= 27
			inner := fx > 10 && fx < 22 && fy > 7 && fy < 25
			if outer && !inner {
				c = white
			}
			if dx, dy := fx-16, fy-13; dx*dx+dy*dy <= 2.5*2.5 {
				c = marquee
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func write(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	log.Println("wrote", path)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
