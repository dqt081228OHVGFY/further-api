package replicate

import (
	"fmt"
	"strconv"
	"strings"
)

func mapOpenAISizeToFlux(size string) (aspect string, width int, height int, ok bool) {
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return "", 0, 0, false
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return "", 0, 0, false
	}

	switch {
	case w == h:
		return "1:1", 0, 0, true
	case w == 1792 && h == 1024:
		return "16:9", 0, 0, true
	case w == 1024 && h == 1792:
		return "9:16", 0, 0, true
	case w == 1536 && h == 1024:
		return "3:2", 0, 0, true
	case w == 1024 && h == 1536:
		return "2:3", 0, 0, true
	}

	rw, rh := reduceRatio(w, h)
	ratioStr := fmt.Sprintf("%d:%d", rw, rh)
	switch ratioStr {
	case "1:1", "16:9", "9:16", "3:2", "2:3", "4:5", "5:4", "3:4", "4:3":
		return ratioStr, 0, 0, true
	}

	width = normalizeFluxDimension(w)
	height = normalizeFluxDimension(h)
	return "custom", width, height, true
}

func reduceRatio(w, h int) (int, int) {
	g := gcd(w, h)
	if g == 0 {
		return w, h
	}
	return w / g, h / g
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func normalizeFluxDimension(value int) int {
	const (
		minDim = 256
		maxDim = 1440
		step   = 32
	)
	if value < minDim {
		value = minDim
	}
	if value > maxDim {
		value = maxDim
	}
	remainder := value % step
	if remainder != 0 {
		if remainder >= step/2 {
			value += step - remainder
		} else {
			value -= remainder
		}
	}
	if value < minDim {
		value = minDim
	}
	if value > maxDim {
		value = maxDim
	}
	return value
}
