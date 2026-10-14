package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

const (
	supersample = 4
	shapeRatio  = 0.805
	cornerRatio = 0.2237
)

var (
	topColor      = color.NRGBA{R: 0x2b, G: 0x32, B: 0x42, A: 0xff}
	bottomColor   = color.NRGBA{R: 0x14, G: 0x17, B: 0x1e, A: 0xff}
	playedColor   = color.NRGBA{R: 0x4c, G: 0x8b, B: 0xe0, A: 0xff}
	upcomingColor = color.NRGBA{R: 0xff, G: 0xc2, B: 0x5e, A: 0xff}
	cursorColor   = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xf2}
)

type iconsetFile struct {
	name string
	size int
}

func main() {
	iconset := flag.String("iconset", "", "directory to write a macOS .iconset into")
	pngPath := flag.String("png", "", "file to write a single PNG into")
	size := flag.Int("size", 256, "edge length of the single PNG")
	flag.Parse()

	if *iconset == "" && *pngPath == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "\nset -iconset, -png or both")
		os.Exit(2)
	}

	if *iconset != "" {
		if err := writeIconset(*iconset); err != nil {
			fail(err)
		}
	}
	if *pngPath != "" {
		if err := writePNG(*pngPath, *size); err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "icon:", err)
	os.Exit(1)
}

func writeIconset(dir string) error {
	files := []iconsetFile{
		{"icon_16x16.png", 16},
		{"icon_16x16@2x.png", 32},
		{"icon_32x32.png", 32},
		{"icon_32x32@2x.png", 64},
		{"icon_128x128.png", 128},
		{"icon_128x128@2x.png", 256},
		{"icon_256x256.png", 256},
		{"icon_256x256@2x.png", 512},
		{"icon_512x512.png", 512},
		{"icon_512x512@2x.png", 1024},
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, file := range files {
		path := filepath.Join(dir, file.name)
		if err := writePNG(path, file.size); err != nil {
			return err
		}
		fmt.Println("wrote", path)
	}
	return nil
}

func writePNG(path string, size int) error {
	if size < 8 || size > 4096 {
		return fmt.Errorf("unsupported size %d", size)
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := png.Encode(file, Render(size)); err != nil {
		return err
	}
	return file.Close()
}

func Render(size int) *image.NRGBA {
	work := size * supersample
	canvas := image.NewNRGBA(image.Rect(0, 0, work, work))

	shape := float64(work) * shapeRatio
	margin := (float64(work) - shape) / 2
	radius := shape * cornerRatio

	for y := 0; y < work; y++ {
		blend := float64(y) / float64(work-1)
		background := mix(topColor, bottomColor, blend)
		for x := 0; x < work; x++ {
			if insideRounded(float64(x)+0.5, float64(y)+0.5, margin, margin, margin+shape, margin+shape, radius) {
				canvas.SetNRGBA(x, y, background)
			}
		}
	}

	barCount := 5
	stripTop := margin + shape*0.28
	stripHeight := shape * 0.44
	barWidth := shape * 0.105
	gap := shape * 0.045
	totalWidth := float64(barCount)*barWidth + float64(barCount-1)*gap
	start := margin + (shape-totalWidth)/2
	heights := []float64{0.42, 0.74, 1, 0.68, 0.48}

	for i := 0; i < barCount; i++ {
		height := stripHeight * heights[i]
		top := stripTop + (stripHeight-height)/2
		left := start + float64(i)*(barWidth+gap)
		shade := upcomingColor
		if i < 2 {
			shade = playedColor
		}
		fillRounded(canvas, left, top, left+barWidth, top+height, barWidth/2, shade)
	}

	cursorWidth := shape * 0.022
	cursorX := start + 2*barWidth + 1.5*gap
	fillRect(canvas, cursorX, stripTop-shape*0.055, cursorX+cursorWidth, stripTop+stripHeight+shape*0.055, cursorColor)

	return downsample(canvas, size)
}

func insideRounded(px, py, x0, y0, x1, y1, radius float64) bool {
	cx := math.Min(math.Max(px, x0+radius), x1-radius)
	cy := math.Min(math.Max(py, y0+radius), y1-radius)
	dx := px - cx
	dy := py - cy
	return dx*dx+dy*dy <= radius*radius
}

func fillRounded(dst *image.NRGBA, x0, y0, x1, y1, radius float64, shade color.NRGBA) {
	inside := func(px, py float64) bool {
		return insideRounded(px, py, x0, y0, x1, y1, radius)
	}
	fill(dst, x0, y0, x1, y1, shade, inside)
}

func fillRect(dst *image.NRGBA, x0, y0, x1, y1 float64, shade color.NRGBA) {
	inside := func(px, py float64) bool {
		return px >= x0 && px <= x1 && py >= y0 && py <= y1
	}
	fill(dst, x0, y0, x1, y1, shade, inside)
}

func fill(dst *image.NRGBA, x0, y0, x1, y1 float64, shade color.NRGBA, inside func(float64, float64) bool) {
	bounds := dst.Bounds()
	from := image.Pt(int(math.Floor(x0))-1, int(math.Floor(y0))-1)
	to := image.Pt(int(math.Ceil(x1))+1, int(math.Ceil(y1))+1)
	for y := from.Y; y <= to.Y; y++ {
		for x := from.X; x <= to.X; x++ {
			if !image.Pt(x, y).In(bounds) {
				continue
			}
			if inside(float64(x)+0.5, float64(y)+0.5) {
				dst.SetNRGBA(x, y, shade)
			}
		}
	}
}

func mix(from, to color.NRGBA, blend float64) color.NRGBA {
	channel := func(a, b uint8) uint8 {
		return uint8(math.Round(float64(a) + (float64(b)-float64(a))*blend))
	}
	return color.NRGBA{R: channel(from.R, to.R), G: channel(from.G, to.G), B: channel(from.B, to.B), A: channel(from.A, to.A)}
}

func downsample(src *image.NRGBA, size int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	area := supersample * supersample
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a uint32
			for sy := 0; sy < supersample; sy++ {
				for sx := 0; sx < supersample; sx++ {
					pixel := src.NRGBAAt(x*supersample+sx, y*supersample+sy)
					r += uint32(pixel.R) * uint32(pixel.A)
					g += uint32(pixel.G) * uint32(pixel.A)
					b += uint32(pixel.B) * uint32(pixel.A)
					a += uint32(pixel.A)
				}
			}
			out := color.NRGBA{}
			if a > 0 {
				out = color.NRGBA{
					R: uint8(r / a),
					G: uint8(g / a),
					B: uint8(b / a),
					A: uint8(a / uint32(area)),
				}
			}
			dst.SetNRGBA(x, y, out)
		}
	}
	return dst
}
