package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func writeWAV24(t *testing.T, rate int, frames int, gen func(frame int) (float64, float64)) []byte {
	t.Helper()
	var pcm bytes.Buffer
	put := func(value float64) {
		sample := int32(math.Round(value * 8388607)) // 2^23 - 1
		pcm.Write([]byte{byte(sample), byte(sample >> 8), byte(sample >> 16)})
	}
	for i := 0; i < frames; i++ {
		left, right := gen(i)
		put(left)
		put(right)
	}

	var out bytes.Buffer
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(36+pcm.Len()))
	out.WriteString("WAVE")
	out.WriteString("fmt ")
	_ = binary.Write(&out, binary.LittleEndian, uint32(16))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(2))
	_ = binary.Write(&out, binary.LittleEndian, uint32(rate))
	_ = binary.Write(&out, binary.LittleEndian, uint32(rate*2*3))
	_ = binary.Write(&out, binary.LittleEndian, uint16(6))
	_ = binary.Write(&out, binary.LittleEndian, uint16(24))
	out.WriteString("data")
	_ = binary.Write(&out, binary.LittleEndian, uint32(pcm.Len()))
	out.Write(pcm.Bytes())
	return out.Bytes()
}

func TestDecode24BitStereoWAV(t *testing.T) {
	const rate = 48000
	values := []float64{0, 0.25, -0.25, 0.5, -0.5, 1, -1}
	raw := writeWAV24(t, rate, len(values), func(frame int) (float64, float64) {
		return values[frame], -values[frame]
	})

	clip, err := Decode("test24.wav", bytes.NewReader(raw), rate)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if clip.Frames != int64(len(values)) {
		t.Fatalf("decoded %d frames, expected %d", clip.Frames, len(values))
	}
	if clip.SourceRate != rate {
		t.Fatalf("source rate came out as %d", clip.SourceRate)
	}

	const tolerance = 1.0 / 8388607 * 1.5
	for i, want := range values {
		gotLeft := float64(clip.Data[i*2])
		gotRight := float64(clip.Data[i*2+1])
		if math.Abs(gotLeft-want) > tolerance {
			t.Fatalf("sample %d left = %.6f, expected %.6f", i, gotLeft, want)
		}
		if math.Abs(gotRight+want) > tolerance {
			t.Fatalf("sample %d right = %.6f, expected %.6f", i, gotRight, -want)
		}
	}
}

func TestDecode24BitWAVKeepsBothChannelsSeparate(t *testing.T) {
	const rate = 48000
	const frames = 4800
	raw := writeWAV24(t, rate, frames, func(frame int) (float64, float64) {
		phase := 2 * math.Pi * 440 * float64(frame) / rate
		return 0.8 * math.Sin(phase), 0.2 * math.Sin(phase)
	})

	clip, err := Decode("test24.wav", bytes.NewReader(raw), rate)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	left := channelRMS(clip.Data, true)
	right := channelRMS(clip.Data, false)
	if math.Abs(left-0.8/math.Sqrt2) > 0.02 || math.Abs(right-0.2/math.Sqrt2) > 0.02 {
		t.Fatalf("channel levels are wrong: left %.3f right %.3f", left, right)
	}
	if left <= right*3 {
		t.Fatalf("the channels are not independent: left %.3f right %.3f", left, right)
	}
}
