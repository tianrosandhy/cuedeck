package audio

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/wav"
)

func tonePower(samples []float32, left bool, rate int, freq float64) float64 {
	offset := 0
	if !left {
		offset = 1
	}
	from := len(samples) / 4
	to := len(samples) * 3 / 4
	omega := 2 * math.Pi * freq / float64(rate)

	coeff := 2 * math.Cos(omega)
	var s1, s2 float64
	for i := from + offset; i < to; i += 2 {
		s0 := float64(samples[i]) + coeff*s1 - s2
		s2, s1 = s1, s0
	}
	return s1*s1 + s2*s2 - coeff*s1*s2
}

func writeStereoWAV(t *testing.T, rate, seconds int, leftFreq, rightFreq float64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stereo.wav")
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
			now := float64(position) / float64(rate)
			samples[i][0] = 0.5 * math.Sin(2*math.Pi*leftFreq*now)
			samples[i][1] = 0.5 * math.Sin(2*math.Pi*rightFreq*now)
			position++
		}
		return len(samples), true
	})
	format := beep.Format{SampleRate: beep.SampleRate(rate), NumChannels: 2, Precision: 2}
	if err := wav.Encode(file, gen, format); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func channelRMS(samples []float32, left bool) float64 {
	offset := 0
	if !left {
		offset = 1
	}
	var sum float64
	count := 0
	for i := offset; i < len(samples); i += 2 {
		sum += float64(samples[i]) * float64(samples[i])
		count++
	}
	if count == 0 {
		return 0
	}
	return math.Sqrt(sum / float64(count))
}

func channelDifference(samples []float32) float64 {
	var sum float64
	count := 0
	for i := 0; i+1 < len(samples); i += 2 {
		sum += math.Abs(float64(samples[i] - samples[i+1]))
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func TestDecodeKeepsStereoChannelsSeparate(t *testing.T) {
	path := writeStereoWAV(t, 44100, 2, 440, 220)
	clip, err := DecodeFile(path, 48000)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if clip.Rate != 48000 {
		t.Fatalf("expected the clip to be resampled to 48000, got %d", clip.Rate)
	}
	if clip.Frames != int64(len(clip.Data)/2) {
		t.Fatalf("frame count %d does not match %d interleaved samples", clip.Frames, len(clip.Data))
	}

	leftRMS := channelRMS(clip.Data, true)
	rightRMS := channelRMS(clip.Data, false)
	if leftRMS < 0.2 || rightRMS < 0.2 {
		t.Fatalf("a channel came out weak: left %.4f right %.4f", leftRMS, rightRMS)
	}
	if difference := channelDifference(clip.Data); difference < 0.05 {
		t.Fatalf("the two channels are nearly identical (mean difference %.4f), stereo was collapsed", difference)
	}

	leftAt440 := tonePower(clip.Data, true, clip.Rate, 440)
	leftAt220 := tonePower(clip.Data, true, clip.Rate, 220)
	if leftAt440 < leftAt220*20 {
		t.Fatalf("the left channel does not hold the 440 Hz tone (440: %.3g, 220: %.3g)", leftAt440, leftAt220)
	}

	rightAt220 := tonePower(clip.Data, false, clip.Rate, 220)
	rightAt440 := tonePower(clip.Data, false, clip.Rate, 440)
	if rightAt220 < rightAt440*20 {
		t.Fatalf("the right channel does not hold the 220 Hz tone (220: %.3g, 440: %.3g)", rightAt220, rightAt440)
	}
}

func TestDecodeExampleMP3KeepsBothChannels(t *testing.T) {
	if _, err := os.Stat(exampleMP3); err != nil {
		t.Skip("example track is not present")
	}
	clip, err := DecodeFile(exampleMP3, 48000)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	leftRMS := channelRMS(clip.Data, true)
	rightRMS := channelRMS(clip.Data, false)
	if leftRMS < 0.01 || rightRMS < 0.01 {
		t.Fatalf("a channel came out silent: left %.4f right %.4f", leftRMS, rightRMS)
	}
	difference := channelDifference(clip.Data)
	t.Logf("stereo example: left rms %.4f, right rms %.4f, mean channel difference %.4f", leftRMS, rightRMS, difference)
	if difference <= 0 {
		t.Fatal("the two channels are identical, stereo information was lost")
	}
}
