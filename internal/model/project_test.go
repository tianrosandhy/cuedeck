package model

import (
	"math"
	"testing"
)

func TestBeatTimeRoundTrip(t *testing.T) {
	tempo := Tempo{BPM: 128, BeatZero: 0.25, SigNum: 4, SigDen: 4}
	for _, beat := range []float64{0, 1, 3.75, 64, 127.5, 1000} {
		seconds := tempo.BeatToSeconds(beat)
		if got := tempo.SecondsToBeat(seconds); math.Abs(got-beat) > 1e-9 {
			t.Fatalf("beat %v round trip gave %v", beat, got)
		}
	}
}

func TestBarBeatTickFourFour(t *testing.T) {
	tempo := DefaultTempo()
	cases := []struct {
		beat  float64
		bar   int
		inBar int
		tick  int
	}{
		{0, 1, 1, 0},
		{3, 1, 4, 0},
		{4, 2, 1, 0},
		{7.5, 2, 4, 480},
		{16, 5, 1, 0},
	}
	for _, c := range cases {
		bar, inBar, tick := tempo.BarBeatTick(c.beat)
		if bar != c.bar || inBar != c.inBar || tick != c.tick {
			t.Fatalf("beat %v gave %d.%d.%d, want %d.%d.%d", c.beat, bar, inBar, tick, c.bar, c.inBar, c.tick)
		}
	}
}

func TestBarBeatTickSixEight(t *testing.T) {
	tempo := Tempo{BPM: 120, SigNum: 6, SigDen: 8}
	if got := tempo.BeatsPerBar(); math.Abs(got-3) > 1e-9 {
		t.Fatalf("6/8 should be three quarter beats per bar, got %v", got)
	}
	bar, inBar, _ := tempo.BarBeatTick(4)
	if bar != 2 || inBar != 2 {
		t.Fatalf("beat 4 in 6/8 gave %d.%d, want 2.2", bar, inBar)
	}
}

func TestSnap(t *testing.T) {
	if got := Snap(3.4, 0.25); math.Abs(got-3.5) > 1e-9 {
		t.Fatalf("snap gave %v", got)
	}
	if got := Snap(3.4, 0); got != 3.4 {
		t.Fatalf("snap with no grid changed the value to %v", got)
	}
}

func TestProjectOrderingAndSections(t *testing.T) {
	p := NewProject()
	p.Add("second", 8)
	p.Add("first", 4)
	p.Add("intro", 0)

	labels := []string{p.Markers[0].Label, p.Markers[1].Label, p.Markers[2].Label}
	want := []string{"intro", "first", "second"}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("markers sorted as %v, want %v", labels, want)
		}
	}

	if got := p.SectionAt(-1); got != -1 {
		t.Fatalf("before the first cue should be no section, got %d", got)
	}
	if got := p.SectionAt(3.999); got != 0 {
		t.Fatalf("beat 3.999 should be section 0, got %d", got)
	}
	if got := p.SectionAt(4); got != 1 {
		t.Fatalf("beat 4 should be section 1, got %d", got)
	}
	if got := p.SectionAt(9999); got != 2 {
		t.Fatalf("late beat should be the last section, got %d", got)
	}

	if !p.Remove(p.Markers[1].ID) {
		t.Fatal("remove reported failure")
	}
	if len(p.Markers) != 2 {
		t.Fatalf("expected two markers left, got %d", len(p.Markers))
	}
}

func TestUpdateKeepsSortedAndUniqueIDs(t *testing.T) {
	p := NewProject()
	p.Add("a", 0)
	p.Add("b", 4)
	if p.Markers[0].ID == p.Markers[1].ID {
		t.Fatal("marker ids collided")
	}
	if !p.Update(p.Markers[0].ID, "moved", 32) {
		t.Fatal("update reported failure")
	}
	if p.Markers[1].Label != "moved" {
		t.Fatalf("marker did not move to the end, order is %v", p.Markers)
	}
}

func TestAutoLabelSkipsUsedNames(t *testing.T) {
	p := NewProject()
	p.Add("M1", 0)
	if got := p.AutoLabel(); got != "M2" {
		t.Fatalf("auto label gave %v, want M2", got)
	}
}
