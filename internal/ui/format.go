package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func formatClock(seconds float64) string {
	if math.IsNaN(seconds) || seconds < 0 {
		seconds = 0
	}
	total := int64(seconds)
	ms := int64(math.Round((seconds - float64(total)) * 1000))
	if ms >= 1000 {
		total++
		ms = 0
	}
	return fmt.Sprintf("%d:%02d.%03d", total/60, total%60, ms)
}

func formatSeconds(seconds float64) string {
	if math.IsNaN(seconds) || seconds < 0 {
		seconds = 0
	}
	total := int(seconds)
	rest := int(math.Round((seconds - float64(total)) * 10))
	if rest >= 10 {
		total++
		rest = 0
	}
	return fmt.Sprintf("%d:%02d.%d", total/60, total%60, rest)
}

func formatBeat(beat float64) string {
	if math.IsNaN(beat) || math.IsInf(beat, 0) {
		return "0"
	}
	s := strconv.FormatFloat(beat, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

func parseFloat(s string, fallback float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return fallback
	}
	return v
}
