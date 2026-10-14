package audio

import (
	"testing"
	"time"
)

func startTestEngine(t *testing.T) (*Engine, *Clip) {
	t.Helper()
	engine, err := StartOn("", 128)
	if err != nil {
		t.Skipf("cannot start the audio engine: %v", err)
	}
	t.Cleanup(engine.Close)

	clip := &Clip{
		Data:   make([]float32, engine.Rate()*8*2),
		Rate:   engine.Rate(),
		Frames: int64(engine.Rate() * 8),
		Kind:   "TEST",
	}
	engine.Load(clip)
	time.Sleep(120 * time.Millisecond)
	if block := engine.MeasuredBlock(); block <= 0 {
		t.Fatal("the audio callback never ran")
	}
	return engine, clip
}

func TestEngineJumpIsAppliedInTheNextBlock(t *testing.T) {
	engine, _ := startTestEngine(t)

	engine.Play()
	time.Sleep(200 * time.Millisecond)
	if !engine.IsPlaying() {
		t.Fatal("playback did not start")
	}
	if position := engine.Position(); position <= 0 || position > 0.3 {
		t.Fatalf("playback position ran to %v after 200ms", position)
	}

	engine.SeekSeconds(6)
	time.Sleep(60 * time.Millisecond)
	position := engine.Position()
	if position < 5.9 || position > 6.2 {
		t.Fatalf("after jumping to 6s the player reports %v", position)
	}
	if !engine.IsPlaying() {
		t.Fatal("jumping should not stop playback")
	}
	latency := engine.CueLatency()
	if latency <= 0 {
		t.Fatal("the jump latency was not measured")
	}
	if limit := engine.MeasuredBlock() * 3 / 2; latency > limit {
		t.Fatalf("the jump took %v to be applied, more than one block (%v)", latency, engine.MeasuredBlock())
	}
}

func TestEngineClampsSeeksAndVolume(t *testing.T) {
	engine, clip := startTestEngine(t)

	engine.SeekSeconds(-5)
	time.Sleep(40 * time.Millisecond)
	if position := engine.Position(); position != 0 {
		t.Fatalf("seeking before the start left the player at %v", position)
	}

	engine.SeekSeconds(1e6)
	time.Sleep(40 * time.Millisecond)
	if position := engine.Position(); position < clip.Seconds()-0.05 {
		t.Fatalf("seeking past the end left the player at %v", position)
	}

	engine.SetVolume(4)
	if engine.Volume() != 1 {
		t.Fatalf("volume was not clamped, got %v", engine.Volume())
	}
	engine.SetVolume(-1)
	if engine.Volume() != 0 {
		t.Fatalf("volume was not clamped, got %v", engine.Volume())
	}
	engine.SetVolume(0.5)
	if engine.Volume() != 0.5 {
		t.Fatalf("volume round trip gave %v", engine.Volume())
	}
}

func TestEngineStopsAtTheEnd(t *testing.T) {
	engine, _ := startTestEngine(t)

	engine.SeekSeconds(7.5)
	engine.Play()
	time.Sleep(900 * time.Millisecond)
	if engine.IsPlaying() {
		t.Fatal("playback should have stopped at the end of the clip")
	}
	if !engine.Ended() {
		t.Fatal("the ended flag was not set")
	}

	engine.Play()
	time.Sleep(100 * time.Millisecond)
	if !engine.IsPlaying() {
		t.Fatal("pressing play at the end should start again from the top")
	}
	if position := engine.Position(); position > 0.3 {
		t.Fatalf("play at the end restarted at %v instead of the beginning", position)
	}
	engine.Pause()
}

func TestEngineReportsCallbackStability(t *testing.T) {
	engine, _ := startTestEngine(t)

	engine.ResetStats()
	start := time.Now()
	time.Sleep(400 * time.Millisecond)
	elapsed := time.Since(start)

	blocks := engine.Blocks()
	expected := elapsed.Seconds() * float64(engine.Rate()) / float64(engine.BlockFrames())
	if float64(blocks) < expected*0.9 {
		t.Fatalf("only %d of an expected %.0f audio callbacks arrived", blocks, expected)
	}
	low, high := engine.BlockSpread()
	if low <= 0 || high <= 0 {
		t.Fatalf("callback timing was not measured (%v .. %v)", low, high)
	}
	if high > engine.MeasuredBlock()*3 {
		t.Fatalf("callback timing jittered between %v and %v", low, high)
	}
}
