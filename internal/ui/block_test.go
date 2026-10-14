package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"cuedeck/internal/audio"
)

func TestBlockSizeCanBeChangedWhileRunning(t *testing.T) {
	engine := testEngine(t)
	fyneApp := test.NewApp()
	t.Cleanup(fyneApp.Quit)
	a := New(fyneApp, engine, testOptions())

	want := fmt.Sprintf("%d", audio.DefaultPeriodFrames)
	if a.blockSelect.Selected != want {
		t.Fatalf("the picker starts on %q, expected %q", a.blockSelect.Selected, want)
	}

	a.setBlock(256)
	if got := a.engine.ConfiguredPeriod(); got != 256 {
		t.Fatalf("the engine still runs %d frame blocks", got)
	}
	if a.blockSelect.Selected != "256" {
		t.Fatalf("the picker shows %q after the change", a.blockSelect.Selected)
	}
	if got := RememberedBlock(fyneApp, 0); got != 256 {
		t.Fatalf("the block size was remembered as %d", got)
	}

	before := a.engine
	a.setBlock(256)
	if a.engine != before {
		t.Fatal("picking the block size already in use should not reopen the engine")
	}
}

func TestChangingTheBlockKeepsTheSongAndItsPosition(t *testing.T) {
	engine := testEngine(t)
	fyneApp := test.NewApp()
	t.Cleanup(fyneApp.Quit)
	a := New(fyneApp, engine, testOptions())

	dir := t.TempDir()
	path, _ := writeWAV(t, dir, "song.wav", 3, 48000)
	a.openPaths([]string{path})
	waitForDecoded(t, a, 5*time.Second)
	clip := a.projects[0].clip
	if clip == nil {
		t.Fatal("the project did not decode")
	}

	a.engine.SeekSeconds(1)
	a.engine.Play()
	time.Sleep(120 * time.Millisecond)

	a.setBlock(512)
	time.Sleep(150 * time.Millisecond)

	if a.engine == engine {
		t.Fatal("the engine was not replaced")
	}
	if a.engine.Clip() != clip {
		t.Fatal("the decoded audio was dropped even though the rate did not change")
	}
	if !a.engine.IsPlaying() {
		t.Fatal("changing the block size stopped playback")
	}
	if position := a.engine.Position(); position < 1.0 || position > 1.5 {
		t.Fatalf("playback resumed at %v, expected the position it had", position)
	}
	if a.engine.ConfiguredPeriod() != 512 {
		t.Fatalf("the new engine runs %d frame blocks", a.engine.ConfiguredPeriod())
	}
	a.engine.Pause()
}

func TestStatusReportsOutputLevelAndGain(t *testing.T) {
	engine := testEngine(t)
	fyneApp := test.NewApp()
	t.Cleanup(fyneApp.Quit)
	a := New(fyneApp, engine, testOptions())

	dir := t.TempDir()
	path, _ := writeWAV(t, dir, "song.wav", 2, 48000)
	a.openPaths([]string{path})
	waitForDecoded(t, a, 5*time.Second)

	a.engine.SetVolume(0.5)
	a.engine.Play()
	time.Sleep(200 * time.Millisecond)
	a.refresh()

	status := a.statusLabel.Text
	if !strings.Contains(status, "out ") {
		t.Fatalf("the status bar does not report the output level: %q", status)
	}
	if !strings.Contains(status, "master -6.0 dB") {
		t.Fatalf("the status bar does not report the master gain in dB: %q", status)
	}
	if !strings.Contains(status, "block ") {
		t.Fatalf("the status bar does not report the block: %q", status)
	}
	a.engine.Pause()
}

func TestLevelTextInDecibels(t *testing.T) {
	cases := []struct {
		linear float32
		want   string
	}{
		{1, "0.0 dB"},
		{0.5, "-6.0 dB"},
		{0.25, "-12.0 dB"},
		{0, "-inf dB"},
	}
	for _, c := range cases {
		if got := dbText(c.linear); got != c.want {
			t.Fatalf("dbText(%v) reads %q, expected %q", c.linear, got, c.want)
		}
	}
}

func TestBlockOptionsCoverTheUsefulRange(t *testing.T) {
	options := blockOptions()
	if len(options) < 3 {
		t.Fatalf("only %d block sizes are offered", len(options))
	}
	for _, label := range options {
		if frames := blockFrames(label); frames < 64 || frames > 2048 {
			t.Fatalf("block size %q parses to %d frames", label, frames)
		}
	}
	if blockFrames("banana") != 0 {
		t.Fatal("a nonsense block label should parse to zero")
	}
}

func TestTheToolbarKeepsTheOutputControls(t *testing.T) {
	a := newTestApp(t)
	canvas, _ := a.Window().Canvas().(test.WindowlessCanvas)
	canvas.Resize(fyne.NewSize(1200, 800))

	if a.blockSelect == nil || a.deviceSelect == nil {
		t.Fatal("the toolbar lost its output controls")
	}
	if width := a.blockSelect.Size().Width; width <= 0 {
		t.Fatalf("the block picker was given %v", a.blockSelect.Size())
	}
	if width := a.deviceSelect.Size().Width; width < 200 {
		t.Fatalf("the device picker was given %v", a.deviceSelect.Size())
	}
}
