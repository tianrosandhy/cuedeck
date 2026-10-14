package ui

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func pixelColor(img image.Image, x, y int) color.NRGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func near(got color.NRGBA, want color.NRGBA, tolerance int) bool {
	diff := func(a, b uint8) int {
		if a > b {
			return int(a - b)
		}
		return int(b - a)
	}
	return diff(got.R, want.R) <= tolerance && diff(got.G, want.G) <= tolerance && diff(got.B, want.B) <= tolerance
}

func TestPlayheadDrawsCueMap(t *testing.T) {
	fyneApp := test.NewApp()
	defer fyneApp.Quit()

	playhead := newPlayhead(nil, nil)
	playhead.SetDuration(180)
	playhead.SetMarks([]float64{45, 90, 135})
	playhead.SetPosition(20)
	playhead.SetSection(0)

	window := fyneApp.NewWindow("playhead")
	window.SetContent(playhead)
	canvas, _ := window.Canvas().(test.WindowlessCanvas)
	canvas.Resize(fyne.NewSize(900, 60))
	image := canvas.Capture()
	if out := os.Getenv("CUEDECK_UI_SNAPSHOT"); out != "" {
		file, err := os.Create(out + ".playhead.png")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, image); err != nil {
			t.Fatal(err)
		}
	}

	width := float64(playhead.Size().Width)
	origin := float64(playhead.Position().X)
	at := func(seconds float64) int { return int(origin + seconds/180*width) }

	played := pixelColor(image, at(6), 30)
	if played.B <= played.R+40 {
		t.Fatalf("the played region should be blue, got %+v", played)
	}
	base := pixelColor(image, at(30), 30)
	if !near(base, colorStrip, 10) {
		t.Fatalf("material before the first cue should be plain, got %+v", base)
	}
	firstSection := pixelColor(image, at(60), 30)
	secondSection := pixelColor(image, at(100), 30)
	if firstSection == secondSection {
		t.Fatalf("cue sections should alternate, both read %+v", firstSection)
	}
	if firstSection == base {
		t.Fatalf("the first section is not drawn, got the plain colour %+v", firstSection)
	}
	for _, mark := range []float64{45, 90, 135} {
		x := at(mark)
		found := false
		for offset := -2; offset <= 2; offset++ {
			got := pixelColor(image, x+offset, 30)
			if got.R > 170 && got.G > 120 && got.R > got.B+40 {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected a cue tick at %.0fs (x=%d), got %+v", mark, x, pixelColor(image, x, 30))
		}
	}
	cursorX := at(20)
	found := false
	for offset := -2; offset <= 2; offset++ {
		if got := pixelColor(image, cursorX+offset, 45); got.R > 230 && got.G > 230 && got.B > 230 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a playhead line near 20s (x=%d), got %+v", cursorX, pixelColor(image, cursorX, 45))
	}
}

func TestPlayheadScrubMapsToSeconds(t *testing.T) {
	var got float64 = -1
	playhead := newPlayhead(func(seconds float64) { got = seconds }, nil)
	playhead.SetDuration(100)
	window := test.NewWindow(playhead)
	window.Resize(fyne.NewSize(400, 40))
	defer window.Close()

	playhead.Tapped(&fyne.PointEvent{Position: fyne.NewPos(200, 20)})
	if expected := 200 / float64(playhead.Size().Width) * 100; math.Abs(got-expected) > 0.5 {
		t.Fatalf("tapping the middle asked to seek to %v, expected %v", got, expected)
	}

	playhead.Tapped(&fyne.PointEvent{Position: fyne.NewPos(-40, 20)})
	if got != 0 {
		t.Fatalf("tapping before the strip asked to seek to %v", got)
	}

	playhead.Tapped(&fyne.PointEvent{Position: fyne.NewPos(900, 20)})
	if got != 100 {
		t.Fatalf("tapping past the strip asked to seek to %v", got)
	}
}

func TestPlayheadScrollingNudges(t *testing.T) {
	var delta float64
	playhead := newPlayhead(nil, func(d float64) { delta = d })
	playhead.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -1)})
	if delta != -1 {
		t.Fatalf("scrolling up reported %v", delta)
	}
	playhead.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, 2)})
	if delta != 2 {
		t.Fatalf("scrolling down reported %v", delta)
	}
}

func TestPlayheadReceivesCanvasGestures(t *testing.T) {
	fyneApp := test.NewApp()
	defer fyneApp.Quit()

	var scrubbed float64 = -1
	var scrolled float64
	playhead := newPlayhead(func(seconds float64) { scrubbed = seconds }, func(d float64) { scrolled = d })
	playhead.SetDuration(120)

	window := fyneApp.NewWindow("gestures")
	window.SetContent(playhead)
	window.Resize(fyne.NewSize(400, 40))
	canvas := window.Canvas()

	centre := fyne.NewPos(playhead.Position().X+playhead.Size().Width/4, playhead.Position().Y+playhead.Size().Height/2)
	test.Scroll(canvas, centre, 0, -1)
	if scrolled != -1 {
		t.Fatalf("the canvas did not deliver a scroll to the strip: %v", scrolled)
	}

	test.TapCanvas(canvas, centre)
	if scrubbed < 0 || scrubbed > 120 {
		t.Fatalf("the canvas did not deliver a tap to the strip: %v", scrubbed)
	}

	scrubbed = -1
	test.Drag(canvas, centre, 40, 0)
	if scrubbed < 0 || scrubbed > 120 {
		t.Fatalf("the canvas did not deliver a drag to the strip: %v", scrubbed)
	}
}
