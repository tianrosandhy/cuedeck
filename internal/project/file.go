package project

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cuedeck/internal/model"
)

const (
	Ext     = ".cuedeck"
	magic   = "CUEDECK\x01"
	Version = 1
	maxHead = 8 << 20
)

var ErrNotProject = errors.New("not a cuedeck project file")

type header struct {
	Version int             `json:"version"`
	Name    string          `json:"name"`
	Tempo   model.Tempo     `json:"tempo"`
	Markers []model.Marker  `json:"markers"`
	Audio   model.AudioInfo `json:"audio"`
	SavedAt time.Time       `json:"savedAt"`
}

func IsProjectFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), Ext)
}

func Save(path string, p *model.Project, audio []byte) error {
	if path == "" {
		return errors.New("empty project path")
	}
	p.Sort()
	p.Audio.Bytes = int64(len(audio))
	h := header{
		Version: Version,
		Name:    p.Name,
		Tempo:   p.Tempo,
		Markers: p.Markers,
		Audio:   p.Audio,
		SavedAt: time.Now().UTC(),
	}
	meta, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".cuedeck-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	w := bufio.NewWriterSize(tmp, 1<<20)
	if _, err := w.WriteString(magic); err != nil {
		tmp.Close()
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(meta))); err != nil {
		tmp.Close()
		return err
	}
	if _, err := w.Write(meta); err != nil {
		tmp.Close()
		return err
	}
	if _, err := w.Write(audio); err != nil {
		tmp.Close()
		return err
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func Load(path string) (*model.Project, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 1<<20)
	got := make([]byte, len(magic))
	if _, err := io.ReadFull(r, got); err != nil {
		return nil, nil, ErrNotProject
	}
	if string(got) != magic {
		return nil, nil, fmt.Errorf("%w: %s", ErrNotProject, filepath.Base(path))
	}
	var metaLen uint32
	if err := binary.Read(r, binary.LittleEndian, &metaLen); err != nil {
		return nil, nil, ErrNotProject
	}
	if metaLen == 0 || metaLen > maxHead {
		return nil, nil, ErrNotProject
	}
	meta := make([]byte, metaLen)
	if _, err := io.ReadFull(r, meta); err != nil {
		return nil, nil, ErrNotProject
	}
	var h header
	if err := json.Unmarshal(meta, &h); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrNotProject, err)
	}
	if h.Version > Version {
		return nil, nil, fmt.Errorf("project version %d is newer than supported %d", h.Version, Version)
	}
	audio, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	p := &model.Project{
		Name:    h.Name,
		Tempo:   h.Tempo,
		Markers: h.Markers,
		Audio:   h.Audio,
	}
	if p.Name == "" {
		p.Name = strings.TrimSuffix(filepath.Base(path), Ext)
	}
	if len(p.Markers) == 0 {
		p.Markers = nil
	}
	p.Sort()
	return p, audio, nil
}
