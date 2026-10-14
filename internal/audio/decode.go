package audio

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/flac"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/vorbis"
	"github.com/gopxl/beep/v2/wav"
)

const resampleQuality = 6

var ErrUnsupportedFormat = errors.New("unsupported audio format")

type Clip struct {
	Data       []float32
	Rate       int
	Frames     int64
	Kind       string
	SourceRate int
}

func (c *Clip) Seconds() float64 {
	if c == nil || c.Rate <= 0 {
		return 0
	}
	return float64(c.Frames) / float64(c.Rate)
}

type readSeekCloser struct {
	io.ReadSeeker
}

func (readSeekCloser) Close() error { return nil }

func KindOf(name string, head []byte) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp3":
		return "mp3"
	case ".wav", ".wave":
		return "wav"
	case ".flac":
		return "flac"
	case ".ogg", ".oga":
		return "ogg"
	}
	switch {
	case len(head) >= 3 && string(head[:3]) == "ID3":
		return "mp3"
	case len(head) >= 2 && head[0] == 0xff && head[1]&0xe0 == 0xe0:
		return "mp3"
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WAVE":
		return "wav"
	case len(head) >= 4 && string(head[:4]) == "fLaC":
		return "flac"
	case len(head) >= 4 && string(head[:4]) == "OggS":
		return "ogg"
	}
	return ""
}

func SupportedExtensions() []string {
	return []string{".mp3", ".wav", ".flac", ".ogg"}
}

func DecodeFile(path string, rate int) (*Clip, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Decode(filepath.Base(path), f, rate)
}

func Decode(name string, rs io.ReadSeeker, rate int) (*Clip, error) {
	if rate <= 0 {
		return nil, fmt.Errorf("invalid target sample rate %d", rate)
	}
	head := make([]byte, 16)
	n, _ := io.ReadFull(rs, head)
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	head = head[:n]
	kind := KindOf(name, head)
	if kind == "" {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, filepath.Base(name))
	}

	var stream beep.StreamSeekCloser
	var format beep.Format
	var err error
	switch kind {
	case "mp3":
		stream, format, err = mp3.Decode(readSeekCloser{rs})
	case "wav":
		stream, format, err = wav.Decode(rs)
	case "flac":
		stream, format, err = flac.Decode(readSeekCloser{rs})
	case "ogg":
		stream, format, err = vorbis.Decode(readSeekCloser{rs})
	}
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	sourceRate := int(format.SampleRate)
	if sourceRate <= 0 {
		return nil, fmt.Errorf("%s: invalid sample rate", kind)
	}
	sourceFrames := int64(stream.Len())
	frames := sourceFrames
	var src beep.Streamer = stream
	if sourceRate != rate {
		src = beep.Resample(resampleQuality, format.SampleRate, beep.SampleRate(rate), stream)
		frames = int64(math.Ceil(float64(sourceFrames) * float64(rate) / float64(sourceRate)))
	}
	if frames <= 0 {
		frames = 1 << 16
	}
	data := make([]float32, 0, frames*2)
	buf := make([][2]float64, 4096)
	for {
		got, ok := src.Stream(buf)
		for i := 0; i < got; i++ {
			data = append(data, float32(buf[i][0]), float32(buf[i][1]))
		}
		if !ok || got == 0 {
			break
		}
	}
	if e, ok := src.(interface{ Err() error }); ok {
		if err := e.Err(); err != nil {
			return nil, err
		}
	}
	if len(data) < 2 {
		return nil, fmt.Errorf("%s: no audio decoded", filepath.Base(name))
	}
	return &Clip{
		Data:       data,
		Rate:       rate,
		Frames:     int64(len(data) / 2),
		Kind:       strings.ToUpper(kind),
		SourceRate: sourceRate,
	}, nil
}
