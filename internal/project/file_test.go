package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"cuedeck/internal/model"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	payload := []byte("pretend this is an mp3")
	p := model.NewProject()
	p.Name = "soundcheck"
	p.Tempo = model.Tempo{BPM: 132.5, BeatZero: 1.25, SigNum: 4, SigDen: 4}
	p.Audio = model.AudioInfo{OriginalName: "track.mp3", Format: "MP3", SourceRate: 44100}
	p.Add("intro", 0)
	p.Add("chorus", 32)

	path := filepath.Join(t.TempDir(), "set"+Ext)
	if err := Save(path, p, payload); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, audio, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Name != "soundcheck" {
		t.Fatalf("name round trip gave %q", got.Name)
	}
	if got.Tempo.BPM != 132.5 || got.Tempo.BeatZero != 1.25 {
		t.Fatalf("tempo round trip gave %+v", got.Tempo)
	}
	if got.Audio.OriginalName != "track.mp3" || got.Audio.SourceRate != 44100 {
		t.Fatalf("audio info round trip gave %+v", got.Audio)
	}
	if got.Audio.Bytes != int64(len(payload)) {
		t.Fatalf("audio size recorded as %d, want %d", got.Audio.Bytes, len(payload))
	}
	if len(got.Markers) != 2 || got.Markers[0].Label != "intro" || got.Markers[1].Beat != 32 {
		t.Fatalf("markers round trip gave %+v", got.Markers)
	}
	if string(audio) != string(payload) {
		t.Fatalf("embedded audio changed: %q", audio)
	}
}

func TestSaveIsAtomicAndReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "set"+Ext)
	p := model.NewProject()
	if err := Save(path, p, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, p, []byte("second")); err != nil {
		t.Fatal(err)
	}
	_, audio, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != "second" {
		t.Fatalf("second save left %q", audio)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary files were left behind: %v", entries)
	}
}

func TestLoadRejectsForeignFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(path, []byte("ID3\x04\x00\x00\x00\x00\x00\x00fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(path); !errors.Is(err, ErrNotProject) {
		t.Fatalf("expected ErrNotProject, got %v", err)
	}
}

func TestProjectFileDetection(t *testing.T) {
	if !IsProjectFile("/music/set1.CUEDECK") {
		t.Fatal("extension match should be case insensitive")
	}
	if IsProjectFile("/music/set1.mp3") {
		t.Fatal("mp3 should not be treated as a project")
	}
}
