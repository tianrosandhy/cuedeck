package audio

import (
	"bytes"
	"errors"
	"math"
	"os"
	"testing"
)

const exampleMP3 = "../../mp3example.mp3"

func TestDecodeExampleMP3(t *testing.T) {
	if _, err := os.Stat(exampleMP3); err != nil {
		t.Skip("example track is not present")
	}
	clip, err := DecodeFile(exampleMP3, 48000)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if clip.Rate != 48000 {
		t.Fatalf("rate is %d, want 48000", clip.Rate)
	}
	if clip.SourceRate != 44100 {
		t.Fatalf("source rate is %d, want 44100", clip.SourceRate)
	}
	if clip.Frames != int64(len(clip.Data)/2) {
		t.Fatalf("frame count %d does not match %d samples", clip.Frames, len(clip.Data))
	}
	if seconds := clip.Seconds(); seconds < 140 || seconds > 160 {
		t.Fatalf("length came out as %.2f s", seconds)
	}
	peak := 0.0
	for _, sample := range clip.Data {
		if value := math.Abs(float64(sample)); value > peak {
			peak = value
		}
	}
	if peak < 0.05 {
		t.Fatalf("decoded audio is silent, peak is %v", peak)
	}
	if peak > 1.2 {
		t.Fatalf("decoded audio is out of range, peak is %v", peak)
	}
}

func TestDecodeWithoutResampling(t *testing.T) {
	if _, err := os.Stat(exampleMP3); err != nil {
		t.Skip("example track is not present")
	}
	clip, err := DecodeFile(exampleMP3, 44100)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if clip.Rate != 44100 || clip.SourceRate != 44100 {
		t.Fatalf("expected an untouched 44100 clip, got %+v", clip)
	}
}

func TestKindOf(t *testing.T) {
	cases := []struct {
		name string
		head []byte
		want string
	}{
		{"a.mp3", nil, "mp3"},
		{"a.WAV", nil, "wav"},
		{"a.flac", nil, "flac"},
		{"a", []byte("ID3\x04"), "mp3"},
		{"a", []byte{0xff, 0xfb, 0x90}, "mp3"},
		{"a", append([]byte("RIFF\x00\x00\x00\x00"), []byte("WAVE")...), "wav"},
		{"a", []byte("fLaC\x00"), "flac"},
		{"a", []byte("OggS\x00"), "ogg"},
		{"a.bin", []byte("nothing"), ""},
	}
	for _, c := range cases {
		if got := KindOf(c.name, c.head); got != c.want {
			t.Fatalf("KindOf(%q) gave %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDecodeRejectsUnknownData(t *testing.T) {
	_, err := Decode("mystery.bin", bytes.NewReader([]byte("not audio at all")), 48000)
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestNilEngineIsSafe(t *testing.T) {
	var engine *Engine
	if engine.HasClip() || engine.IsPlaying() || engine.Rate() != 0 {
		t.Fatal("a nil engine should report an idle state")
	}
	if engine.Duration() != 0 || engine.Position() != 0 {
		t.Fatal("a nil engine should report no position")
	}
	engine.Load(&Clip{})
	engine.Play()
	engine.SeekSeconds(1)
	engine.SetVolume(2)
	engine.Stop()
	engine.Close()
}
