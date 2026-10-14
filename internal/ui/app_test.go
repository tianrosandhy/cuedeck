package ui

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"cuedeck/internal/model"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	fyneApp := test.NewApp()
	t.Cleanup(fyneApp.Quit)
	return New(fyneApp, nil, "")
}

func TestWindowRendersWithCues(t *testing.T) {
	a := newTestApp(t)
	a.proj.Name = "soundcheck"
	p := a.proj
	p.Add("Intro", 0)
	p.Add("Verse", 16)
	p.Add("Chorus", 32)
	p.Add("Bridge", 62.5)
	for i := 0; i < 8; i++ {
		p.Add(fmt.Sprintf("part %d", i+5), 80+float64(i)*16)
	}
	a.buildRows()
	a.syncTimeline()
	a.refresh()
	a.section = 1
	a.highlightRows()
	a.selectCue(4)

	if len(a.rows) != 12 {
		t.Fatalf("expected twelve cue rows, got %d", len(a.rows))
	}
	if a.cueTitle.Text != "Cues (12)" {
		t.Fatalf("cue title reads %q", a.cueTitle.Text)
	}

	canvas, ok := a.Window().Canvas().(test.WindowlessCanvas)
	if !ok {
		t.Fatalf("unexpected canvas type %T", a.Window().Canvas())
	}
	canvas.Resize(fyne.NewSize(1200, 800))
	image := canvas.Capture()
	if image.Bounds().Dx() != 1200 || image.Bounds().Dy() != 800 {
		t.Fatalf("captured %v", image.Bounds())
	}
	if width := a.clock.Size().Width; width < 120 {
		t.Fatalf("the clock was given only %v, text will be clipped", a.clock.Size())
	}
	if width := a.beatLabel.Size().Width; width < 60 {
		t.Fatalf("the bar readout was given only %v, text will be clipped", a.beatLabel.Size())
	}
	if width := a.durLabel.Size().Width; width < 60 {
		t.Fatalf("the length readout was given only %v", a.durLabel.Size())
	}

	picked, playing := false, false
	for y := 0; y < image.Bounds().Dy(); y++ {
		for x := 0; x < image.Bounds().Dx(); x++ {
			switch pixel := pixelColor(image, x, y); {
			case near(pixel, colorRowPicked, 6):
				picked = true
			case near(pixel, colorBarPlaying, 6):
				playing = true
			}
		}
	}
	if !picked {
		t.Fatal("the picked cue row was not painted")
	}
	if !playing {
		t.Fatal("the blue edge of the part playing now was not painted")
	}

	if out := os.Getenv("CUEDECK_UI_SNAPSHOT"); out != "" {
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, image); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCuesNeedAudio(t *testing.T) {
	a := newTestApp(t)
	a.addCueAtPlayhead()
	if len(a.proj.Markers) != 0 {
		t.Fatal("a cue should not be added without audio loaded")
	}
}

func TestTempoChangeKeepsMarkersOnTheirBeat(t *testing.T) {
	a := newTestApp(t)
	a.proj.Tempo = model.Tempo{BPM: 120, SigNum: 4, SigDen: 4}
	a.proj.Add("cue", 8)
	at120 := a.proj.Tempo.BeatToSeconds(8)
	a.bpm.SetText("60")
	at60 := a.proj.Tempo.BeatToSeconds(8)
	if a.proj.Markers[0].Beat != 8 {
		t.Fatalf("the marker moved off its beat: %v", a.proj.Markers[0].Beat)
	}
	if !(at60 > at120) {
		t.Fatalf("halving the tempo should move the cue later: %v then %v", at120, at60)
	}
}

func TestSnapRoundsCueBeats(t *testing.T) {
	a := newTestApp(t)
	a.snap.SetChecked(true)
	if got := a.snapBeat(3.34); got != 3.25 {
		t.Fatalf("snapped to %v, want 3.25", got)
	}
	a.snap.SetChecked(false)
	if got := a.snapBeat(3.34); got != 3.34 {
		t.Fatalf("snapping should be off, got %v", got)
	}
}

func TestTransportAndCueActionsSurviveNoAudio(t *testing.T) {
	a := newTestApp(t)
	a.togglePlay()
	a.stop()
	a.jumpTo(0)
	a.trigger(3)
	a.nudgeBeats(4)
	a.nudgeBars(2)
	a.alignBeatZero()
	a.editCue(0)
	a.deleteCue(0)
}

func TestTapTempo(t *testing.T) {
	a := newTestApp(t)
	a.tapTempo()
	time.Sleep(200 * time.Millisecond)
	a.tapTempo()
	if bpm := a.proj.Tempo.BPM; bpm < 200 || bpm > 400 {
		t.Fatalf("two taps 200ms apart produced %v bpm, expected roughly 300", bpm)
	}
	if got := parseFloat(a.bpm.Text, 0); math.Abs(got-a.proj.Tempo.BPM) > 0.01 {
		t.Fatalf("bpm field reads %q but the project tempo is %v", a.bpm.Text, a.proj.Tempo.BPM)
	}
}

func TestTapTempoIgnoresDoubleTaps(t *testing.T) {
	a := newTestApp(t)
	a.tapTempo()
	before := a.proj.Tempo.BPM
	a.tapTempo()
	if a.proj.Tempo.BPM != before {
		t.Fatalf("two taps within 100ms produced %v bpm, expected the tempo to stay at %v", a.proj.Tempo.BPM, before)
	}
}

func TestDigitHotkeysFireCues(t *testing.T) {
	a := newTestApp(t)
	a.proj.Add("one", 0)
	a.proj.Add("two", 32)
	a.proj.Add("three", 64)
	a.buildRows()
	a.syncTimeline()

	test.TypeOnCanvas(a.Window().Canvas(), "3")
	if a.section != 2 {
		t.Fatalf("typing 3 selected cue %d, expected the third", a.section)
	}
	if !a.rows[2].playing {
		t.Fatal("the fired cue row is not highlighted")
	}

	test.TypeOnCanvas(a.Window().Canvas(), "1")
	if a.section != 0 {
		t.Fatalf("typing 1 selected cue %d, expected the first", a.section)
	}

	test.TypeOnCanvas(a.Window().Canvas(), "9")
	if a.section != 0 {
		t.Fatal("a hotkey past the last cue should do nothing")
	}
}

func TestFocusInTheBpmFieldDoesNotFireCues(t *testing.T) {
	a := newTestApp(t)
	a.proj.Add("one", 0)
	a.proj.Add("two", 32)
	a.buildRows()
	a.syncTimeline()
	a.section = -1

	a.Window().Canvas().Focus(a.bpm)
	test.Type(a.bpm, "2")
	if a.section != -1 {
		t.Fatal("typing in the bpm field should not fire a cue")
	}
	if a.bpm.Text == "" {
		t.Fatal("the bpm field did not receive the keystroke")
	}

	a.refresh()
	if !a.editing {
		t.Fatal("the app should notice that a text field is holding the keys")
	}
	if !strings.Contains(a.statusLabel.Text, "bpm") {
		t.Fatalf("the status bar does not warn about the focused field: %q", a.statusLabel.Text)
	}

	a.Window().Canvas().Unfocus()
	a.refresh()
	if a.editing || strings.Contains(a.statusLabel.Text, "hands the keys back") {
		t.Fatalf("the warning stayed after the field was released: %q", a.statusLabel.Text)
	}
}

func pressKey(t *testing.T, a *App, name fyne.KeyName) {
	t.Helper()
	handler := a.Window().Canvas().OnTypedKey()
	if handler == nil {
		t.Fatal("the window has no key handler")
	}
	handler(&fyne.KeyEvent{Name: name})
}

func twelveCueApp(t *testing.T) *App {
	t.Helper()
	a := newTestApp(t)
	for i := 0; i < 12; i++ {
		a.proj.Add(fmt.Sprintf("part %d", i+1), float64(i)*16)
	}
	a.buildRows()
	a.syncTimeline()
	canvas, _ := a.Window().Canvas().(test.WindowlessCanvas)
	canvas.Resize(fyne.NewSize(1200, 800))
	return a
}

func TestArrowKeysPickWithoutPlaying(t *testing.T) {
	a := twelveCueApp(t)
	a.section = 0

	pressKey(t, a, fyne.KeyDown)
	if a.selected != 1 {
		t.Fatalf("down picked cue %d, expected the one after the part playing", a.selected)
	}
	if a.section != 0 {
		t.Fatalf("picking must not start playing, but the playing part moved to %d", a.section)
	}
	if !a.rows[1].selected {
		t.Fatal("the picked cue row is not highlighted")
	}
	if a.rows[1].bg.FillColor != colorRowPicked {
		t.Fatalf("the picked row uses %v, expected the picked colour", a.rows[1].bg.FillColor)
	}

	pressKey(t, a, fyne.KeyDown)
	pressKey(t, a, fyne.KeyDown)
	if a.selected != 3 {
		t.Fatalf("three downs landed on %d, expected 3", a.selected)
	}
	if a.section != 0 {
		t.Fatal("arrowing through cues must not play anything")
	}

	pressKey(t, a, fyne.KeyUp)
	if a.selected != 2 {
		t.Fatalf("up landed on %d, expected 2", a.selected)
	}
	if a.rows[1].selected {
		t.Fatal("the previously picked row is still highlighted")
	}
}

func TestArrowKeysReachCuesBeyondHotkeyNine(t *testing.T) {
	a := twelveCueApp(t)
	a.section = -1

	for i := 0; i < 12; i++ {
		pressKey(t, a, fyne.KeyDown)
	}
	if a.selected != 11 {
		t.Fatalf("twelve downs landed on cue %d, expected the twelfth", a.selected)
	}

	pressKey(t, a, fyne.KeyReturn)
	if a.section != 11 {
		t.Fatalf("enter played from cue %d, expected the twelfth", a.section)
	}
	if a.playhead.selected != 11 {
		t.Fatalf("the strip marks cue %d as picked, expected the twelfth", a.playhead.selected)
	}
}

func TestPickingClampsAtBothEnds(t *testing.T) {
	a := twelveCueApp(t)

	for i := 0; i < 4; i++ {
		pressKey(t, a, fyne.KeyUp)
	}
	if a.selected != 0 {
		t.Fatalf("upping past the start left the pick at %d", a.selected)
	}

	for i := 0; i < 40; i++ {
		pressKey(t, a, fyne.KeyDown)
	}
	if a.selected != 11 {
		t.Fatalf("downing past the end left the pick at %d", a.selected)
	}
}

func TestEnterPlaysThePickedCueFromAnEmptySelection(t *testing.T) {
	a := twelveCueApp(t)
	a.section = 3

	pressKey(t, a, fyne.KeyEnter)
	if a.selected != 3 {
		t.Fatalf("enter with nothing picked chose cue %d, expected the part playing", a.selected)
	}
	if a.section != 3 {
		t.Fatalf("enter moved the playing part to %d", a.section)
	}
}

func TestClickingARowPicksIt(t *testing.T) {
	a := twelveCueApp(t)
	test.Tap(a.rows[4])
	if a.selected != 4 {
		t.Fatalf("clicking the fifth row picked %d", a.selected)
	}
	if !a.rows[4].selected || a.rows[0].selected {
		t.Fatal("the wrong row is highlighted")
	}
}

func TestHoveringARowChangesItsColour(t *testing.T) {
	a := twelveCueApp(t)
	row := a.rows[2]
	idle := row.bg.FillColor

	row.MouseIn(&desktop.MouseEvent{})
	if row.bg.FillColor == idle {
		t.Fatal("hovering a cue row did not change its colour")
	}
	hovered := row.bg.FillColor

	row.MouseOut()
	if row.bg.FillColor != idle {
		t.Fatalf("leaving the row left it at %v instead of %v", row.bg.FillColor, idle)
	}

	a.selectCue(2)
	row.MouseIn(&desktop.MouseEvent{})
	if row.bg.FillColor == hovered {
		t.Fatal("a picked row should keep the picked colour while hovered")
	}
	if row.bg.FillColor != colorRowPicked {
		t.Fatalf("a picked row uses %v", row.bg.FillColor)
	}
}

func TestPlayingPartAndPickedPartAreShownSeparately(t *testing.T) {
	a := twelveCueApp(t)
	a.section = 1
	a.selectCue(4)

	if !a.rows[1].playing || a.rows[1].bar.FillColor != colorBarPlaying {
		t.Fatal("the part playing now should be edged in blue")
	}
	if a.rows[1].selected {
		t.Fatal("the part playing now should not be the picked one")
	}
	if !a.rows[4].selected || a.rows[4].bg.FillColor != colorRowPicked {
		t.Fatal("the picked part is not highlighted")
	}
	if a.rows[4].bar.FillColor == colorBarPlaying {
		t.Fatal("the picked part should not look like it is playing")
	}
}

func TestCueListScrollsToThePickedCue(t *testing.T) {
	a := twelveCueApp(t)
	if a.cueScroll.Offset.Y != 0 {
		t.Fatalf("the list started scrolled to %v", a.cueScroll.Offset.Y)
	}

	a.selectCue(11)
	if a.cueScroll.Offset.Y <= 0 {
		t.Fatal("picking an off-screen cue did not scroll the list")
	}

	a.selectCue(0)
	if a.cueScroll.Offset.Y != 0 {
		t.Fatalf("picking the first cue left the list scrolled to %v", a.cueScroll.Offset.Y)
	}
}

func TestBeatFormatting(t *testing.T) {
	tempo := model.DefaultTempo()
	if got := tempo.FormatBeat(0); got != "1.1.000" {
		t.Fatalf("beat 0 formatted as %q", got)
	}
	if got := tempo.FormatBeat(-1); got != "pre-roll" {
		t.Fatalf("negative beat formatted as %q", got)
	}
	if got := formatClock(83.456); got != "1:23.456" {
		t.Fatalf("clock formatted as %q", got)
	}
	if got := formatBeat(3.500); got != "3.5" {
		t.Fatalf("beat formatted as %q", got)
	}
}
