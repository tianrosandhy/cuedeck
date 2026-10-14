package ui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"cuedeck/internal/audio"
	"cuedeck/internal/model"
	"cuedeck/internal/project"
)

type decodeState int

const (
	decodePending decodeState = iota
	decodeRunning
	decodeReady
	decodeFailed
)

type resumeState struct {
	armed    bool
	position float64
	playing  bool
}

type loadedProject struct {
	path    string
	proj    *model.Project
	raw     []byte
	clip    *audio.Clip
	state   decodeState
	failure error
	used    int
}

func (p *loadedProject) audioName() string {
	if name := p.proj.Audio.OriginalName; name != "" {
		return name
	}
	return filepath.Base(p.path)
}

func (p *loadedProject) clipBytes() int64 {
	if p.clip == nil {
		return 0
	}
	return int64(len(p.clip.Data)) * 4
}

func (p *loadedProject) isPlaceholder() bool {
	return p == nil || p.proj == nil
}

func (a *App) activeEntry() *loadedProject {
	if a.active < 0 || a.active >= len(a.projects) {
		return nil
	}
	return a.projects[a.active]
}

func (a *App) cachedBytes() int64 {
	var total int64
	for _, entry := range a.projects {
		total += entry.clipBytes()
	}
	return total
}

func (a *App) decodedCount() int {
	count := 0
	for _, entry := range a.projects {
		if entry.state == decodeReady {
			count++
		}
	}
	return count
}

func (a *App) openPaths(paths []string) {
	added := -1
	for _, path := range paths {
		entry, err := a.addProject(path)
		if err != nil {
			a.fail("Cannot open "+filepath.Base(path), err)
			continue
		}
		if added < 0 {
			added = len(a.projects) - 1
		}
		_ = entry
	}
	if added >= 0 {
		a.activate(added)
	}
	a.scheduleDecode()
}

func (a *App) addProject(path string) (*loadedProject, error) {
	entry := &loadedProject{path: path}
	if project.IsProjectFile(path) {
		loaded, raw, err := project.Load(path)
		if err != nil {
			return nil, err
		}
		entry.proj = loaded
		entry.raw = raw
	} else {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		fresh := model.NewProject()
		fresh.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		fresh.Audio = model.AudioInfo{
			OriginalName: filepath.Base(path),
			Format:       strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), ".")),
			Bytes:        int64(len(raw)),
		}
		entry.proj = fresh
		entry.raw = raw
	}
	if len(entry.raw) == 0 {
		entry.state = decodeFailed
		entry.failure = fmt.Errorf("no audio inside")
	}
	a.projects = append(a.projects, entry)
	a.buildProjectRows()
	return entry, nil
}

func (a *App) activate(index int) {
	if index < 0 || index >= len(a.projects) || index == a.active {
		return
	}
	entry := a.projects[index]
	a.active = index
	a.uses++
	entry.used = a.uses

	a.proj = entry.proj
	a.raw = entry.raw
	a.path = entry.path
	a.dirty = false
	a.selected = -1
	a.section = -2
	a.tap.Reset()

	a.syncing = true
	a.syncTempoControls()
	a.syncing = false

	a.buildRows()
	a.syncTimeline()
	a.loadActiveClip()
	a.buildProjectRows()
	a.updateTitle()
}

func (a *App) chooseProject(index int) {
	a.activate(index)
}

func (a *App) closeProject(index int) {
	if index < 0 || index >= len(a.projects) {
		return
	}
	a.projects = append(a.projects[:index], a.projects[index+1:]...)

	switch {
	case len(a.projects) == 0:
		a.forgetProject()
	case index == a.active:
		next := index
		if next >= len(a.projects) {
			next = len(a.projects) - 1
		}
		a.active = -1
		a.activate(next)
	default:
		if index < a.active {
			a.active--
		}
		a.buildProjectRows()
		a.updateTitle()
	}
}

func (a *App) forgetProject() {
	a.active = -1
	a.proj = model.NewProject()
	a.raw = nil
	a.path = ""
	a.dirty = false
	a.selected = -1
	a.section = -2
	a.engine.Load(nil)

	a.syncing = true
	a.syncTempoControls()
	a.syncing = false

	a.buildRows()
	a.syncTimeline()
	a.buildProjectRows()
	a.updateTitle()
}

func (a *App) loadActiveClip() {
	entry := a.activeEntry()
	if entry == nil {
		a.engine.Load(nil)
		return
	}
	if entry.clip != nil {
		a.engine.Load(entry.clip)
		return
	}
	a.engine.Load(nil)
	a.scheduleDecode()
}

func (a *App) scheduleDecode() {
	if a.decoding || a.engine == nil || a.engine.Rate() <= 0 {
		return
	}
	index := -1
	for i, entry := range a.projects {
		if entry.state == decodePending && len(entry.raw) > 0 {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}

	entry := a.projects[index]
	entry.state = decodeRunning
	a.decoding = true
	a.updateProjectRows()

	name := entry.audioName()
	raw := entry.raw
	rate := a.engine.Rate()
	go func() {
		clip, err := audio.Decode(name, bytes.NewReader(raw), rate)
		fyne.Do(func() {
			a.decoding = false
			if err != nil {
				entry.state = decodeFailed
				entry.failure = err
			} else {
				entry.state = decodeReady
				entry.clip = clip
				a.evictClips(entry)
				a.adoptDecoded(entry)
			}
			a.updateProjectRows()
			a.scheduleDecode()
		})
	}()
}

func (a *App) adoptDecoded(entry *loadedProject) {
	if a.activeEntry() != entry || a.engine.Clip() == entry.clip {
		return
	}
	a.engine.Load(entry.clip)
	if a.resume.armed {
		a.engine.SeekSeconds(a.resume.position)
		if a.resume.playing {
			a.engine.Play()
		}
		a.resume.armed = false
	}
	a.syncTimeline()
}

func (a *App) evictClips(keep *loadedProject) {
	if a.memoryBudget <= 0 {
		return
	}
	for a.cachedBytes() > a.memoryBudget {
		victim := a.leastRecentlyUsedClip(keep)
		if victim == nil {
			return
		}
		victim.clip = nil
		victim.state = decodePending
	}
}

func (a *App) leastRecentlyUsedClip(keep *loadedProject) *loadedProject {
	var victim *loadedProject
	for _, entry := range a.projects {
		if entry == keep || entry == a.activeEntry() || entry.clip == nil {
			continue
		}
		if victim == nil || entry.used < victim.used {
			victim = entry
		}
	}
	return victim
}

func (a *App) invalidateClips() {
	for _, entry := range a.projects {
		entry.clip = nil
		if len(entry.raw) > 0 {
			entry.state = decodePending
		}
	}
}

func (a *App) buildSetPanel() fyne.CanvasObject {
	a.setList = container.NewVBox()
	a.setTitle = widget.NewLabelWithStyle("Set", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	hint := widget.NewLabelWithStyle("click a song to load it, then press play", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	hint.SizeName = theme.SizeNameCaptionText
	header := container.NewPadded(container.NewVBox(a.setTitle, hint))
	a.setScroll = container.NewVScroll(a.setList)
	return container.NewBorder(header, nil, nil, nil, a.setScroll)
}

func (a *App) buildProjectRows() {
	if a.setList == nil {
		return
	}
	objects := make([]fyne.CanvasObject, 0, len(a.projects))
	rows := make([]*cueRow, 0, len(a.projects))
	for i, entry := range a.projects {
		row := a.newProjectRow(i, entry)
		rows = append(rows, row)
		objects = append(objects, row)
	}
	a.projectRows = rows
	a.setList.Objects = objects
	a.setList.Refresh()

	if a.setTitle != nil {
		count := fmt.Sprintf("Set (%d)", len(a.projects))
		if a.setTitle.Text != count {
			a.setTitle.SetText(count)
		}
	}
	a.highlightProjectRows()
}

func (a *App) newProjectRow(index int, entry *loadedProject) *cueRow {
	name := widget.NewLabelWithStyle(entry.proj.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	summary := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	summary.SizeName = theme.SizeNameCaptionText

	closeBtn := widget.NewButtonWithIcon("", theme.ContentClearIcon(), func() { a.closeProject(index) })
	content := container.NewBorder(nil, nil, nil, container.NewHBox(closeBtn), container.NewVBox(name, summary))

	row := newCueRow(content, summary, func() { a.chooseProject(index) })
	a.fillProjectSummary(row, entry)
	return row
}

func (a *App) fillProjectSummary(row *cueRow, entry *loadedProject) {
	row.timing.SetText(projectSummary(entry))
}

func projectSummary(entry *loadedProject) string {
	cues := fmt.Sprintf("%d cues", len(entry.proj.Markers))
	switch entry.state {
	case decodeRunning:
		return "loading…  ·  " + cues
	case decodeFailed:
		return "cannot decode  ·  " + cues
	case decodePending:
		return "not loaded  ·  " + cues
	}
	return fmt.Sprintf("%s  ·  %s", formatSeconds(entry.clip.Seconds()), cues)
}

func (a *App) updateProjectRows() {
	for i, row := range a.projectRows {
		if i >= len(a.projects) {
			break
		}
		a.fillProjectSummary(row, a.projects[i])
	}
	a.highlightProjectRows()
}

func (a *App) highlightProjectRows() {
	for i, row := range a.projectRows {
		row.setLive(i == a.active)
	}
}
