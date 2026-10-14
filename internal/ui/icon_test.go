package ui

import (
	"bytes"
	"image/png"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

type iconRecorder struct {
	fyne.App
	icon fyne.Resource
}

func (r *iconRecorder) SetIcon(icon fyne.Resource) {
	r.icon = icon
	r.App.SetIcon(icon)
}

func (r *iconRecorder) Icon() fyne.Resource {
	if r.icon != nil {
		return r.icon
	}
	return r.App.Icon()
}

func TestEmbeddedAppIconIsUsable(t *testing.T) {
	image, err := png.Decode(bytes.NewReader(iconPNG))
	if err != nil {
		t.Fatalf("the embedded icon is not a PNG: %v", err)
	}
	bounds := image.Bounds()
	if bounds.Dx() != bounds.Dy() {
		t.Fatalf("the icon is %v, expected a square", bounds)
	}
	if bounds.Dx() < 128 {
		t.Fatalf("the icon is only %d px wide, too small for a window icon", bounds.Dx())
	}

	painted := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, alpha := image.At(x, y).RGBA(); alpha > 0 {
				painted++
			}
		}
	}
	if painted < bounds.Dx()*bounds.Dy()/2 {
		t.Fatalf("most of the icon is transparent, only %d pixels are painted", painted)
	}
}

func TestTheWindowGetsTheEmbeddedIcon(t *testing.T) {
	recorder := &iconRecorder{App: test.NewApp()}
	t.Cleanup(recorder.App.Quit)

	New(recorder, nil, testOptions())

	icon := recorder.Icon()
	if icon == nil {
		t.Fatal("the app was never handed an icon")
	}
	if len(icon.Content()) != len(iconPNG) {
		t.Fatalf("the app icon is %d bytes, expected the embedded %d", len(icon.Content()), len(iconPNG))
	}
}
