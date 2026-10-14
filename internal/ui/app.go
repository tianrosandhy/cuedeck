package ui

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"cuedeck/internal/audio"
	"cuedeck/internal/model"
	"cuedeck/internal/project"
)

type tapTracker struct {
	taps []time.Time
}

func (t *tapTracker) Tap() (float64, bool) {
	now := time.Now()
	if n := len(t.taps); n > 0 {
		gap := now.Sub(t.taps[n-1])
		if gap < 100*time.Millisecond {
			return 0, false
		}
		if gap > 2*time.Second {
			t.taps = nil
		}
	}
	t.taps = append(t.taps, now)
	if len(t.taps) > 9 {
		t.taps = t.taps[len(t.taps)-9:]
	}
	if len(t.taps) < 2 {
		return 0, false
	}
	span := t.taps[len(t.taps)-1].Sub(t.taps[0])
	per := span.Seconds() / float64(len(t.taps)-1)
	if per <= 0 {
		return 0, false
	}
	return 60 / per, true
}

func (t *tapTracker) Reset() { t.taps = nil }

type Options struct {
	Paths        []string
	MemoryBudget int64
}

type App struct {
	fyneApp fyne.App
	win     fyne.Window
	engine  *audio.Engine
	proj    *model.Project
	path    string
	raw     []byte
	tap     *tapTracker
	busy    dialog.Dialog

	projects     []*loadedProject
	active       int
	uses         int
	memoryBudget int64
	decoding     bool
	resume       resumeState

	setList     *fyne.Container
	setScroll   *container.Scroll
	setTitle    *widget.Label
	projectRows []*cueRow

	deviceSelect *widget.Select
	deviceLabels []string
	deviceIDs    []string

	blockSelect *widget.Select

	fileLabel   *widget.Label
	titleLabel  *widget.Label
	clock       *valueText
	beatLabel   *valueText
	durLabel    *widget.Label
	statusLabel *widget.Label
	toolbarBox  *fyne.Container
	readoutBox  *fyne.Container
	footerBox   *fyne.Container
	playhead    *playhead
	playBtn     *widget.Button
	bpm         *widget.Entry
	sig         *widget.Select
	snap        *widget.Check
	cueList     *fyne.Container
	cueScroll   *container.Scroll
	cueTitle    *widget.Label
	rows        []*cueRow

	section  int
	selected int
	dirty    bool
	playing  bool
	editing  bool
	syncing  bool
}

func New(fyneApp fyne.App, engine *audio.Engine, options Options) *App {
	a := &App{
		fyneApp:      fyneApp,
		engine:       engine,
		proj:         model.NewProject(),
		tap:          &tapTracker{},
		section:      -2,
		selected:     -1,
		active:       -1,
		memoryBudget: options.MemoryBudget,
	}
	a.win = fyneApp.NewWindow("CueDeck")
	fyneApp.SetIcon(appIcon())
	a.win.SetContent(a.buildUI())
	a.win.Resize(fyne.NewSize(1200, 800))
	a.attachKeys()
	a.win.SetOnDropped(a.openDropped)
	a.buildRows()
	a.syncTempoControls()
	a.syncTimeline()
	a.updateTitle()
	a.buildProjectRows()
	a.refreshDevices()

	if len(options.Paths) > 0 {
		a.openPaths(options.Paths)
	}
	return a
}

func (a *App) Run() {
	a.startTicker()
	a.win.ShowAndRun()
}

func (a *App) Window() fyne.Window { return a.win }

func (a *App) Project() *model.Project { return a.proj }

func (a *App) buildUI() fyne.CanvasObject {
	cuePanel := a.buildCuePanel()
	setPanel := a.buildSetPanel()
	top := container.NewVBox(
		a.buildToolbar(),
		widget.NewSeparator(),
		a.buildTempoBar(),
		a.buildReadout(),
		a.buildPlayhead(),
	)
	left := container.NewBorder(nil, nil, a.buildTransport(), nil, setPanel)
	return container.NewBorder(
		top,
		a.buildFooter(),
		left,
		nil,
		cuePanel,
	)
}

func (a *App) buildPlayhead() fyne.CanvasObject {
	a.playhead = newPlayhead(a.seekTo, func(delta float64) { a.nudgeBeats(delta * 0.5) })
	return container.NewPadded(a.playhead)
}

func (a *App) buildToolbar() fyne.CanvasObject {
	openBtn := widget.NewButtonWithIcon("Open", theme.FolderOpenIcon(), a.openFile)
	saveBtn := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() { a.save(false) })
	saveAsBtn := widget.NewButton("Save as…", func() { a.save(true) })
	helpBtn := widget.NewButtonWithIcon("", theme.HelpIcon(), a.showHelp)

	a.titleLabel = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	a.fileLabel = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	a.fileLabel.SizeName = theme.SizeNameCaptionText

	info := container.NewHBox(a.titleLabel, a.fileLabel)
	left := container.NewHBox(openBtn, saveBtn, saveAsBtn, widget.NewSeparator(), info)

	a.deviceSelect = widget.NewSelect(nil, nil)
	a.deviceSelect.PlaceHolder = "output device"
	a.deviceSelect.OnChanged = func(label string) {
		a.switchDevice(a.deviceIDFor(label))
	}
	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), a.refreshDevices)
	output := container.NewHBox(
		widget.NewLabelWithStyle("Output", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWrap(fyne.NewSize(300, a.deviceSelect.MinSize().Height+6), a.deviceSelect),
		refreshBtn,
	)

	a.blockSelect = widget.NewSelect(blockOptions(), nil)
	a.blockSelect.OnChanged = func(label string) {
		a.setBlock(blockFrames(label))
	}
	a.syncBlockSelect()
	block := container.NewHBox(
		widget.NewLabelWithStyle("Block", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWrap(fyne.NewSize(120, a.blockSelect.MinSize().Height+6), a.blockSelect),
	)

	a.toolbarBox = container.NewBorder(nil, nil, nil, container.NewHBox(output, block, widget.NewSeparator(), helpBtn), left)
	return a.toolbarBox
}

func blockOptions() []string {
	return []string{"64", "128", "256", "512", "1024"}
}

func blockFrames(label string) int {
	return int(parseFloat(label, 0))
}

func (a *App) syncBlockSelect() {
	if a.blockSelect == nil {
		return
	}
	period := audio.DefaultPeriodFrames
	if a.engine != nil {
		period = a.engine.ConfiguredPeriod()
	}
	label := fmt.Sprintf("%d", period)
	if a.blockSelect.Selected != label {
		a.blockSelect.SetSelected(label)
	}
}

func (a *App) buildReadout() fyne.CanvasObject {
	a.clock = newValueText(38, true)
	a.beatLabel = newValueText(22, true)
	a.beatLabel.SetColor(colorAccent)
	a.durLabel = widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{Monospace: true})
	a.durLabel.SizeName = theme.SizeNameSubHeadingText

	right := container.NewVBox(a.beatLabel, a.durLabel)
	a.readoutBox = container.NewBorder(nil, nil, a.clock, right)
	return container.NewPadded(a.readoutBox)
}

func (a *App) buildTempoBar() fyne.CanvasObject {
	a.bpm = widget.NewEntry()
	a.bpm.SetPlaceHolder("120")
	a.bpm.TextStyle = fyne.TextStyle{Monospace: true}
	a.bpm.Validator = func(s string) error {
		v := parseFloat(s, 0)
		if s != "" && (v <= 0 || v > 1000) {
			return fmt.Errorf("bpm must be between 0 and 1000")
		}
		return nil
	}
	a.bpm.OnChanged = func(s string) {
		if a.syncing {
			return
		}
		v := parseFloat(s, 0)
		if v <= 0 || v > 1000 {
			return
		}
		a.proj.Tempo.BPM = v
		a.onTempoChanged()
	}
	a.bpm.OnSubmitted = func(string) { a.win.Canvas().Unfocus() }

	tapBtn := widget.NewButtonWithIcon("Tap", theme.MediaRecordIcon(), a.tapTempo)
	a.sig = widget.NewSelect([]string{"4/4", "3/4", "2/4", "5/4", "6/8", "7/8", "12/8"}, nil)
	a.sig.SetSelected("4/4")
	a.sig.OnChanged = func(s string) {
		if a.syncing {
			return
		}
		num, den := parseSig(s)
		a.proj.Tempo.SigNum = num
		a.proj.Tempo.SigDen = den
		a.onTempoChanged()
	}
	a.snap = widget.NewCheck("snap", func(bool) {})
	a.snap.SetChecked(true)
	zeroBtn := widget.NewButton("Beat 1 = playhead", a.alignBeatZero)

	return container.NewPadded(container.NewHBox(
		widget.NewLabelWithStyle("BPM", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWrap(fyne.NewSize(80, a.bpm.MinSize().Height+8), a.bpm),
		tapBtn,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Signature", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWrap(fyne.NewSize(90, a.sig.MinSize().Height), a.sig),
		zeroBtn,
		widget.NewSeparator(),
		a.snap,
	))
}

func (a *App) buildFooter() fyne.CanvasObject {
	addBtn := widget.NewButtonWithIcon("Add cue at playhead", theme.ContentAddIcon(), a.addCueAtPlayhead)
	beatAddBtn := widget.NewButton("Add cue at beat…", a.addCueAtBeat)
	a.statusLabel = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	a.statusLabel.SizeName = theme.SizeNameCaptionText

	a.footerBox = container.NewVBox(
		widget.NewSeparator(),
		container.NewPadded(container.NewHBox(addBtn, beatAddBtn)),
		container.NewPadded(a.statusLabel),
	)
	return a.footerBox
}

func (a *App) attachKeys() {
	canvas := a.win.Canvas()
	canvas.SetOnTypedRune(func(r rune) {
		if r >= '1' && r <= '9' {
			a.trigger(int(r - '1'))
			return
		}
		switch r {
		case 'm', 'M':
			a.addCueAtPlayhead()
		case 's', 'S':
			a.stop()
		case '[':
			a.nudgeBars(-1)
		case ']':
			a.nudgeBars(1)
		case ',':
			a.nudgeBeats(-0.25)
		case '.':
			a.nudgeBeats(0.25)
		}
	})
	canvas.SetOnTypedKey(func(ev *fyne.KeyEvent) {
		switch ev.Name {
		case fyne.KeySpace:
			a.togglePlay()
		case fyne.KeyEscape:
			a.stop()
		case fyne.KeyLeft:
			a.nudgeBeats(-1)
		case fyne.KeyRight:
			a.nudgeBeats(1)
		case fyne.KeyUp:
			a.moveSelection(-1)
		case fyne.KeyDown:
			a.moveSelection(1)
		case fyne.KeyReturn, fyne.KeyEnter:
			a.playSelection()
		case fyne.KeyPageUp:
			a.chooseProject(a.active - 1)
		case fyne.KeyPageDown:
			a.chooseProject(a.active + 1)
		}
	})
}

func (a *App) startTicker() {
	go func() {
		ticker := time.NewTicker(40 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			fyne.Do(a.refresh)
		}
	}()
}

func (a *App) refresh() {
	pos := a.engine.Position()
	duration := a.engine.Duration()

	_, editing := a.win.Canvas().Focused().(*widget.Entry)
	a.editing = editing

	clock := formatClock(pos)
	beat := a.proj.Tempo.FormatBeat(a.proj.Tempo.SecondsToBeat(pos))
	length := formatClock(duration)
	relayout := needsRelayout(a.clock.text.Text, clock) ||
		needsRelayout(a.beatLabel.text.Text, beat) ||
		needsRelayout(a.durLabel.Text, length)

	a.clock.SetText(clock)
	a.beatLabel.SetText(beat)
	if a.durLabel.Text != length {
		a.durLabel.SetText(length)
	}
	if relayout && a.readoutBox != nil {
		a.readoutBox.Refresh()
	}

	a.playhead.SetDuration(duration)
	a.playhead.SetPosition(pos)

	section := -1
	if a.engine.HasClip() {
		section = a.proj.SectionAt(a.proj.Tempo.SecondsToBeat(pos))
	}
	if section != a.section {
		a.section = section
		a.playhead.SetSection(section)
		a.highlightRows()
	}

	if playing := a.engine.IsPlaying(); playing != a.playing {
		a.playing = playing
		if playing {
			a.playBtn.SetIcon(theme.MediaPauseIcon())
			a.playBtn.SetText("Pause")
		} else {
			a.playBtn.SetIcon(theme.MediaPlayIcon())
			a.playBtn.SetText("Play")
		}
	}

	if status := a.statusText(); a.statusLabel.Text != status {
		relayout := len(a.statusLabel.Text) != len(status)
		a.statusLabel.SetText(status)
		if relayout && a.footerBox != nil {
			a.footerBox.Refresh()
		}
	}
}

func needsRelayout(current, next string) bool {
	return current != next && len(current) != len(next)
}

func (a *App) statusText() string {
	parts := make([]string, 0, 6)
	if a.engine == nil {
		parts = append(parts, "no audio engine")
	} else {
		info := a.engine.Device()
		parts = append(parts, fmt.Sprintf("%s · %d Hz", info.Name, a.engine.Rate()))
		if block := a.engine.MeasuredBlock(); block > 0 {
			frames := float64(block) * float64(a.engine.Rate()) / float64(time.Second)
			parts = append(parts, fmt.Sprintf("block %.2f ms (%.0f f)", float64(block.Microseconds())/1000, frames))
		} else {
			parts = append(parts, "measuring block…")
		}
		if clip := a.engine.Clip(); clip != nil {
			parts = append(parts, fmt.Sprintf("%s %.1f kHz → %.1f kHz · %s", clip.Kind, float64(clip.SourceRate)/1000, float64(a.engine.Rate())/1000, formatSeconds(clip.Seconds())))
		} else {
			parts = append(parts, "no audio loaded")
		}
		if cue := a.engine.CueLatency(); cue > 0 {
			parts = append(parts, fmt.Sprintf("last cue applied %.2f ms after trigger", float64(cue.Microseconds())/1000))
		}
		if peak := a.engine.TakePeak(); peak > 0 {
			parts = append(parts, "out "+dbText(peak))
		}
		if volume := a.engine.Volume(); volume > 0 && volume < 1 {
			parts = append(parts, "master "+dbText(float32(volume)))
		}
		if late := a.engine.LateBlocks(); late > 0 {
			parts = append(parts, fmt.Sprintf("LATE %d blocks (worst %s) — raise the block size",
				late, a.engine.WorstLateBlock().Round(10*time.Microsecond)))
		}
	}
	if len(a.projects) > 0 {
		parts = append(parts, fmt.Sprintf("set %d/%d songs", a.active+1, len(a.projects)))
		parts = append(parts, fmt.Sprintf("%d decoded", a.decodedCount()))
		parts = append(parts, fmt.Sprintf("PCM %s", memoryText(a.cachedBytes(), a.memoryBudget)))
	}
	if a.editing {
		parts = append(parts, "typing the bpm — enter hands the keys back to the transport")
	}
	return strings.Join(parts, "  ·  ")
}

func memoryText(used, budget int64) string {
	text := fmt.Sprintf("%d MB", used>>20)
	if budget > 0 {
		text += fmt.Sprintf(" of %d MB", budget>>20)
	}
	return text
}

func dbText(linear float32) string {
	if linear <= 0 {
		return "-inf dB"
	}
	return fmt.Sprintf("%.1f dB", 20*math.Log10(float64(linear)))
}

func (a *App) syncTimeline() {
	marks := make([]float64, 0, len(a.proj.Markers))
	for _, m := range a.proj.Markers {
		marks = append(marks, a.proj.Tempo.BeatToSeconds(m.Beat))
	}
	a.playhead.SetMarks(marks)
	a.playhead.SetDuration(a.engine.Duration())
	a.playhead.SetSelected(a.selected)
	a.section = -2
	a.highlightRows()
}

func (a *App) syncTempoControls() {
	a.bpm.SetText(formatBeat(a.proj.Tempo.BPM))
	a.sig.SetSelected(fmt.Sprintf("%d/%d", a.proj.Tempo.SigNum, a.proj.Tempo.SigDen))
}

func (a *App) onTempoChanged() {
	a.buildRows()
	a.syncTimeline()
	a.markDirty()
}

func (a *App) markDirty() {
	a.dirty = true
	a.updateTitle()
}

func (a *App) updateTitle() {
	mark := ""
	if a.dirty {
		mark = " •"
	}
	title := a.proj.Name
	position := ""
	if a.active < 0 {
		title = "no project loaded"
	} else if len(a.projects) > 1 {
		position = fmt.Sprintf(" (%d/%d)", a.active+1, len(a.projects))
	}
	a.win.SetTitle(fmt.Sprintf("CueDeck — %s%s%s", title, position, mark))

	shown := title + position + mark
	name := "unsaved"
	if a.path != "" {
		name = filepath.Base(a.path)
	}
	file := fmt.Sprintf("  (%s)", name)
	relayout := needsRelayout(a.titleLabel.Text, shown) || needsRelayout(a.fileLabel.Text, file)
	if a.titleLabel.Text != shown {
		a.titleLabel.SetText(shown)
	}
	if a.fileLabel.Text != file {
		a.fileLabel.SetText(file)
	}
	if relayout && a.toolbarBox != nil {
		a.toolbarBox.Refresh()
	}
}

func (a *App) togglePlay() {
	if !a.engine.HasClip() {
		return
	}
	a.engine.Toggle()
}

func (a *App) stop() {
	a.engine.Stop()
}

func (a *App) seekTo(seconds float64) {
	a.engine.SeekSeconds(seconds)
}

func (a *App) nudgeBeats(delta float64) {
	if !a.engine.HasClip() {
		return
	}
	beat := a.proj.Tempo.SecondsToBeat(a.engine.Position()) + delta
	a.engine.SeekSeconds(a.proj.Tempo.BeatToSeconds(beat))
}

func (a *App) nudgeBars(bars float64) {
	a.nudgeBeats(bars * a.proj.Tempo.BeatsPerBar())
}

func (a *App) trigger(index int) {
	if index < 0 || index >= len(a.proj.Markers) {
		return
	}
	a.jumpTo(index)
}

func (a *App) jumpTo(index int) {
	if index < 0 || index >= len(a.proj.Markers) {
		return
	}
	marker := a.proj.Markers[index]
	a.engine.SeekSeconds(a.proj.Tempo.BeatToSeconds(marker.Beat))
	a.section = index
	a.playhead.SetSection(index)
	a.highlightRows()
}

func (a *App) tapTempo() {
	bpm, ok := a.tap.Tap()
	if !ok {
		return
	}
	bpm = math.Round(bpm*100) / 100
	a.proj.Tempo.BPM = bpm
	if a.bpm.Text != formatBeat(bpm) {
		a.bpm.SetText(formatBeat(bpm))
	}
	a.onTempoChanged()
}

func (a *App) alignBeatZero() {
	if !a.engine.HasClip() {
		return
	}
	a.proj.Tempo.BeatZero = a.engine.Position()
	a.buildRows()
	a.syncTimeline()
	a.markDirty()
}

func (a *App) openFile() {
	d := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			a.fail("Cannot open file", err)
			return
		}
		if reader == nil {
			return
		}
		path := reader.URI().Path()
		reader.Close()
		a.openPaths([]string{path})
	}, a.win)
	d.SetFilter(storage.NewExtensionFileFilter(append([]string{project.Ext}, audio.SupportedExtensions()...)))
	d.SetTitleText("Open a project or an audio file (adds to the set)")
	d.Resize(fyne.NewSize(940, 640))
	d.Show()
}

func (a *App) openDropped(_ fyne.Position, uris []fyne.URI) {
	paths := make([]string, 0, len(uris))
	for _, uri := range uris {
		if uri == nil {
			continue
		}
		if path := uri.Path(); path != "" {
			paths = append(paths, path)
		}
	}
	if len(paths) > 0 {
		a.openPaths(paths)
	}
}

func (a *App) save(forceDialog bool) {
	if a.activeEntry() == nil {
		a.fail("Nothing to save", fmt.Errorf("open a project first"))
		return
	}
	if a.path == "" || forceDialog {
		d := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil {
				a.fail("Cannot save project", err)
				return
			}
			if writer == nil {
				return
			}
			path := writer.URI().Path()
			writer.Close()
			a.writeProject(path)
		}, a.win)
		d.SetFileName(a.proj.Name + project.Ext)
		d.SetFilter(storage.NewExtensionFileFilter([]string{project.Ext}))
		d.Resize(fyne.NewSize(940, 640))
		d.Show()
		return
	}
	a.writeProject(a.path)
}

func (a *App) writeProject(path string) {
	if !project.IsProjectFile(path) {
		path += project.Ext
	}
	if err := project.Save(path, a.proj, a.raw); err != nil {
		a.fail("Cannot save project", err)
		return
	}
	a.path = path
	if entry := a.activeEntry(); entry != nil {
		entry.path = path
	}
	a.dirty = false
	a.updateTitle()
}

func (a *App) showBusy(message string) {
	if a.busy != nil {
		return
	}
	bar := widget.NewProgressBarInfinite()
	bar.Start()
	d := dialog.NewCustomWithoutButtons("Working", container.NewVBox(widget.NewLabel(message), bar), a.win)
	a.busy = d
	d.Show()
}

func (a *App) hideBusy() {
	if a.busy == nil {
		return
	}
	a.busy.Hide()
	a.busy = nil
}

func (a *App) fail(title string, err error) {
	dialog.ShowError(fmt.Errorf("%s: %w", title, err), a.win)
}

func (a *App) showHelp() {
	text := strings.Join([]string{
		"space        play / pause",
		"esc          stop (back to beat 1)",
		"1 … 9        fire cue 1-9 instantly",
		"↑ / ↓        pick the next part to play",
		"enter        play from the picked cue",
		"← / →        nudge one beat",
		", / .        nudge 1/4 beat",
		"[ / ]        nudge one bar",
		"m            drop a cue at the playhead",
		"page up/down load the previous / next song in the set",
		"scroll       nudge the strip by 1/2 beat",
		"click / drag the strip to scrub",
		"",
		"The picked cue is amber in the list and on the",
		"strip; the part playing now has a blue edge.",
		"Cues are stored as beat positions, so they stay",
		"locked to the music if you change the BPM.",
		"",
		"The set on the left holds every project you have",
		"opened. Clicking one loads it paused — press",
		"space when you want it to start, and nothing",
		"starts by itself when a song ends.",
	}, "\n")
	label := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	d := dialog.NewCustom("Keys", "Close", container.NewPadded(label), a.win)
	d.Resize(fyne.NewSize(520, 560))
	d.Show()
}

func parseSig(s string) (int, int) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 4, 4
	}
	num := int(parseFloat(parts[0], 4))
	den := int(parseFloat(parts[1], 4))
	if num <= 0 {
		num = 4
	}
	if den <= 0 {
		den = 4
	}
	return num, den
}
