package audio

import (
	"testing"
	"time"
)

func levelClip(rate int, seconds int, value float32) *Clip {
	frames := rate * seconds
	data := make([]float32, frames*2)
	for i := range data {
		data[i] = value
	}
	return &Clip{Data: data, Rate: rate, Frames: int64(frames), Kind: "TEST"}
}

func TestEngineLevelMeters(t *testing.T) {
	engine, err := StartOn("", 128)
	if err != nil {
		t.Skipf("cannot start the audio engine: %v", err)
	}
	defer engine.Close()

	engine.Load(levelClip(engine.Rate(), 2, 0.5))
	engine.Play()
	time.Sleep(250 * time.Millisecond)

	if peak := engine.TakePeak(); peak < 0.45 || peak > 0.55 {
		t.Fatalf("the peak meter reads %.3f for a half-scale signal", peak)
	}
	if full := engine.FullScaleSamples(); full != 0 {
		t.Fatalf("%d samples were flagged as hitting full scale on a half-scale signal", full)
	}
	if again := engine.TakePeak(); again != 0 {
		t.Fatalf("the peak meter was not reset by reading it: %v", again)
	}

	engine.Load(levelClip(engine.Rate(), 2, 1.2))
	engine.Play()
	time.Sleep(250 * time.Millisecond)

	if peak := engine.TakePeak(); peak < 1.1 || peak > 1.3 {
		t.Fatalf("the peak meter reads %.3f for a signal above full scale", peak)
	}
	if engine.FullScaleSamples() == 0 {
		t.Fatal("a signal above full scale was not flagged")
	}
}

func TestEngineReportsLateCallbacks(t *testing.T) {
	engine, err := StartOn("", 128)
	if err != nil {
		t.Skipf("cannot start the audio engine: %v", err)
	}
	defer engine.Close()

	engine.Load(levelClip(engine.Rate(), 5, 0.5))
	engine.Play()
	engine.ResetStats()
	if engine.LateBlocks() != 0 || engine.WorstLateBlock() != 0 {
		t.Fatalf("resetting the stats left %d late blocks and a worst case of %v", engine.LateBlocks(), engine.WorstLateBlock())
	}

	time.Sleep(600 * time.Millisecond)

	late, blocks := engine.LateBlocks(), engine.Blocks()
	if blocks == 0 {
		t.Fatal("no audio callbacks ran")
	}
	switch {
	case late > 0 && engine.WorstLateBlock() <= 0:
		t.Fatal("late blocks were counted without recording how late")
	case late == 0 && engine.WorstLateBlock() != 0:
		t.Fatalf("a worst case of %v was recorded with no late blocks", engine.WorstLateBlock())
	}
	if float64(late) > float64(blocks)*0.1 {
		t.Fatalf("%d of %d callbacks were late, which would make the meter useless", late, blocks)
	}
	t.Logf("%d of %d callbacks arrived late in this test process (worst %v)", late, blocks, engine.WorstLateBlock())
}
