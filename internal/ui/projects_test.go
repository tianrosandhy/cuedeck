package ui

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/wav"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"

	"cuedeck/internal/audio"
	"cuedeck/internal/model"
	"cuedeck/internal/project"
)

func writeCueProject(t *testing.T, dir, name string, audio []byte, cues ...float64) string {
	t.Helper()
	p := model.NewProject()
	p.Name = name
	for i, beat := range cues {
		p.Add(name+" cue "+string(rune('A'+i)), beat)
	}
	path := filepath.Join(dir, name+project.Ext)
	if err := project.Save(path, p, audio); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeWAV(t *testing.T, dir, name string, seconds int, rate int) (string, []byte) {
	t.Helper()
	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	total := rate * seconds
	position := 0
	gen := beep.StreamerFunc(func(samples [][2]float64) (int, bool) {
		for i := range samples {
			if position >= total {
				return i, false
			}
			value := 0.1 * math.Sin(2*math.Pi*440*float64(position)/float64(rate))
			samples[i][0] = value
			samples[i][1] = value
			position++
		}
		return len(samples), true
	})
	if err := wav.Encode(file, gen, beep.Format{SampleRate: beep.SampleRate(rate), NumChannels: 2, Precision: 2}); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, raw
}

func testEngine(t *testing.T) *audio.Engine {
	t.Helper()
	engine, err := audio.StartOn("", audio.DefaultPeriodFrames)
	if err != nil {
		t.Skipf("no audio device available: %v", err)
	}
	t.Cleanup(engine.Close)
	return engine
}

func waitForDecoded(t *testing.T, a *App, limit time.Duration) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if len(a.projects) > 0 && a.decodedCount() == len(a.projects) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%d of %d projects finished decoding", a.decodedCount(), len(a.projects))
}

func TestOpeningSeveralProjectsFillsTheSet(t *testing.T) {
	a := newTestApp(t)
	dir := t.TempDir()
	first := writeCueProject(t, dir, "one", []byte("audio one"), 0, 16)
	second := writeCueProject(t, dir, "two", []byte("audio two"), 0)
	third := writeCueProject(t, dir, "three", []byte("audio three"), 0, 32, 64)

	a.openPaths([]string{first, second, third})

	if len(a.projects) != 3 {
		t.Fatalf("the set holds %d projects, expected 3", len(a.projects))
	}
	if a.active != 0 {
		t.Fatalf("the first project opened should be active, got %d", a.active)
	}
	if a.proj.Name != "one" {
		t.Fatalf("the active project is %q", a.proj.Name)
	}
	if a.path != first {
		t.Fatalf("the active path is %q, expected %q", a.path, first)
	}
	if len(a.projectRows) != 3 {
		t.Fatalf("the set panel shows %d rows", len(a.projectRows))
	}
	if !strings.Contains(a.setTitle.Text, "3") {
		t.Fatalf("the set header reads %q", a.setTitle.Text)
	}
	if !strings.Contains(a.titleLabel.Text, "(1/3)") {
		t.Fatalf("the title does not show the position in the set: %q", a.titleLabel.Text)
	}
	if len(a.rows) != 2 {
		t.Fatalf("the cue list shows %d cues for the first project, expected 2", len(a.rows))
	}
	if a.engine.IsPlaying() {
		t.Fatal("opening projects must not start playback")
	}
	if a.decodedCount() != 0 {
		t.Fatalf("nothing should be decoded without an audio device, %d are", a.decodedCount())
	}
	for _, entry := range a.projects {
		if entry.state != decodePending {
			t.Fatalf("project %q is in state %d, expected pending", entry.proj.Name, entry.state)
		}
	}
}

func TestChoosingAProjectSwapsEverythingWithoutPlaying(t *testing.T) {
	a := newTestApp(t)
	dir := t.TempDir()
	first := writeCueProject(t, dir, "one", []byte("audio one"), 0, 16)
	second := writeCueProject(t, dir, "two", []byte("audio two"), 0, 32, 64)
	a.openPaths([]string{first, second})

	a.chooseProject(1)

	if a.active != 1 {
		t.Fatalf("choosing the second project left active=%d", a.active)
	}
	if a.proj.Name != "two" {
		t.Fatalf("the active project is %q", a.proj.Name)
	}
	if a.path != second || string(a.raw) != "audio two" {
		t.Fatalf("the active path/audio did not follow the choice")
	}
	if len(a.rows) != 3 {
		t.Fatalf("the cue list shows %d cues, expected the second project's 3", len(a.rows))
	}
	if a.titleLabel.Text != "two (2/2)" {
		t.Fatalf("the title reads %q", a.titleLabel.Text)
	}
	if a.engine.IsPlaying() {
		t.Fatal("choosing a project must not start playback")
	}
	if a.selected != -1 {
		t.Fatal("choosing a project should clear the picked cue")
	}
	if !a.projectRows[1].live || a.projectRows[0].live {
		t.Fatal("the set panel does not mark the chosen project as the live one")
	}

	a.chooseProject(1)
	if a.active != 1 {
		t.Fatal("choosing the project already active should be a no-op")
	}
}

func TestClosingProjectsKeepsTheSetUsable(t *testing.T) {
	a := newTestApp(t)
	dir := t.TempDir()
	first := writeCueProject(t, dir, "one", []byte("audio one"), 0)
	second := writeCueProject(t, dir, "two", []byte("audio two"), 0)
	third := writeCueProject(t, dir, "three", []byte("audio three"), 0)
	a.openPaths([]string{first, second, third})

	a.closeProject(0)
	if len(a.projects) != 2 || a.proj.Name != "two" {
		t.Fatalf("closing the active project left %d projects with %q active", len(a.projects), a.proj.Name)
	}

	a.closeProject(0)
	if len(a.projects) != 1 || a.proj.Name != "three" {
		t.Fatalf("closing again left %d projects with %q active", len(a.projects), a.proj.Name)
	}
	if a.projectRows[0].live == false {
		t.Fatal("the remaining project is not marked live")
	}

	a.closeProject(0)
	if len(a.projects) != 0 {
		t.Fatalf("the set still holds %d projects", len(a.projects))
	}
	if a.active != -1 {
		t.Fatalf("active is %d with an empty set", a.active)
	}
	if !strings.Contains(a.titleLabel.Text, "no project") {
		t.Fatalf("the title reads %q with an empty set", a.titleLabel.Text)
	}
	if len(a.rows) != 0 {
		t.Fatalf("the cue list still shows %d cues", len(a.rows))
	}
	if a.engine.IsPlaying() {
		t.Fatal("closing everything should not leave playback running")
	}
}

func TestClosingAnEarlierProjectKeepsTheActiveOne(t *testing.T) {
	a := newTestApp(t)
	dir := t.TempDir()
	first := writeCueProject(t, dir, "one", []byte("audio one"), 0)
	second := writeCueProject(t, dir, "two", []byte("audio two"), 0)
	third := writeCueProject(t, dir, "three", []byte("audio three"), 0)
	a.openPaths([]string{first, second, third})
	a.chooseProject(2)

	a.closeProject(0)

	if a.active != 1 || a.proj.Name != "three" {
		t.Fatalf("after closing an earlier project active=%d with %q", a.active, a.proj.Name)
	}
	if len(a.projects) != 2 {
		t.Fatalf("the set holds %d projects", len(a.projects))
	}
}

func TestMemoryBudgetEvictsTheOldestClip(t *testing.T) {
	a := newTestApp(t)
	a.memoryBudget = 30 << 20

	clipOf := func(frames int) *audio.Clip {
		return &audio.Clip{Data: make([]float32, frames*2), Rate: 48000, Frames: int64(frames)}
	}
	a.projects = []*loadedProject{
		{proj: model.NewProject(), clip: clipOf(48000 * 30), state: decodeReady, used: 1},
		{proj: model.NewProject(), clip: clipOf(48000 * 30), state: decodeReady, used: 2},
		{proj: model.NewProject(), clip: clipOf(48000 * 30), state: decodeReady, used: 3},
	}
	a.active = 2

	if used := a.cachedBytes() >> 20; used != 32 {
		t.Fatalf("three 30 second clips should measure 32 MiB, got %d", used)
	}

	a.evictClips(a.projects[2])

	if a.projects[0].clip != nil {
		t.Fatal("the least recently used clip was not evicted")
	}
	if a.projects[0].state != decodePending {
		t.Fatalf("an evicted project should be pending again, it is %d", a.projects[0].state)
	}
	if a.projects[1].clip == nil {
		t.Fatal("too much was evicted")
	}
	if a.projects[2].clip == nil {
		t.Fatal("the active project must never be evicted")
	}
	if used := a.cachedBytes() >> 20; used != 21 {
		t.Fatalf("cache holds %d MiB after eviction, expected 21", used)
	}
}

func TestDroppedFilesJoinTheSet(t *testing.T) {
	a := newTestApp(t)
	dir := t.TempDir()
	path := writeCueProject(t, dir, "dropped", []byte("audio"), 0, 8)

	a.openDropped(fyne.Position{}, []fyne.URI{storage.NewFileURI(path)})

	if len(a.projects) != 1 || a.proj.Name != "dropped" {
		t.Fatalf("the dropped project did not join the set: %d projects, active %q", len(a.projects), a.proj.Name)
	}
}

func TestOpeningADecodedProjectLoadsItWithoutPlaying(t *testing.T) {
	engine := testEngine(t)
	fyneApp := test.NewApp()
	t.Cleanup(fyneApp.Quit)
	a := New(fyneApp, engine, testOptions())

	dir := t.TempDir()
	path, _ := writeWAV(t, dir, "song.wav", 1, 44100)
	second, _ := writeWAV(t, dir, "other.wav", 1, 44100)

	a.openPaths([]string{path})
	waitForDecoded(t, a, 5*time.Second)

	entry := a.projects[0]
	if entry.clip == nil {
		t.Fatal("the opened project was not decoded")
	}
	if entry.clip.Seconds() < 0.9 || entry.clip.Seconds() > 1.2 {
		t.Fatalf("the decoded clip is %v seconds long", entry.clip.Seconds())
	}
	if engine.Clip() != entry.clip {
		t.Fatal("the decoded project was not handed to the engine")
	}
	if engine.IsPlaying() {
		t.Fatal("opening a project must leave the transport stopped")
	}
	if entry.clip.Rate != engine.Rate() {
		t.Fatalf("the clip runs at %d Hz but the engine at %d Hz", entry.clip.Rate, engine.Rate())
	}

	a.openPaths([]string{second})
	waitForDecoded(t, a, 5*time.Second)
	if len(a.projects) != 2 {
		t.Fatalf("the set holds %d projects", len(a.projects))
	}
	if a.active != 1 {
		t.Fatalf("the newly opened project should be active, got %d", a.active)
	}
	if engine.Clip() != a.projects[1].clip {
		t.Fatal("the engine is not on the newly opened project")
	}
	if engine.IsPlaying() {
		t.Fatal("opening a second project must not start playback")
	}

	a.chooseProject(0)
	if engine.Clip() != a.projects[0].clip {
		t.Fatal("choosing back did not put the first clip back in the engine")
	}
	if engine.IsPlaying() {
		t.Fatal("choosing a project must not start playback")
	}
}

func TestSongEndDoesNotAdvanceTheSet(t *testing.T) {
	engine := testEngine(t)
	fyneApp := test.NewApp()
	t.Cleanup(fyneApp.Quit)
	a := New(fyneApp, engine, testOptions())

	dir := t.TempDir()
	short, _ := writeWAV(t, dir, "short.wav", 1, 48000)
	other, _ := writeWAV(t, dir, "other.wav", 3, 48000)
	a.openPaths([]string{short, other})
	waitForDecoded(t, a, 5*time.Second)

	a.chooseProject(0)
	engine.Play()
	if !engine.IsPlaying() {
		t.Fatal("playback did not start")
	}

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) && engine.IsPlaying() {
		time.Sleep(20 * time.Millisecond)
	}
	if engine.IsPlaying() {
		t.Fatal("the short project never reached its end")
	}
	time.Sleep(100 * time.Millisecond)

	if a.active != 0 {
		t.Fatalf("the set advanced to project %d on its own", a.active)
	}
	if a.proj.Name != "short" {
		t.Fatalf("the active project changed to %q", a.proj.Name)
	}
	if engine.Clip() != a.projects[0].clip {
		t.Fatal("the engine was handed another project on its own")
	}
	if !engine.Ended() {
		t.Fatal("the engine does not report the end of the song")
	}
	if a.decodedCount() != 2 {
		t.Fatalf("only %d of 2 projects are decoded", a.decodedCount())
	}
}
