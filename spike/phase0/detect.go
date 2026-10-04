package main

import "image"

// Metrics is the per-spot classifier output for one captured frame.
type Metrics struct {
	Spot      Spot
	WhiteFrac float64 // near-white, low-saturation pixel fraction (the prompt fill)
	BlueFrac  float64 // border-blue pixel fraction (the prompt outline)
	Score     float64 // whiteFrac + 0.25*blueFrac
}

// classifyRoi scores one region. A prompt is a near-white box with a blue
// border; the scene background is greenish/blue and much darker.
func classifyRoi(img *image.RGBA, r Rect) (white, blue float64) {
	b := r.Bounds().Intersect(img.Bounds())
	n := b.Dx() * b.Dy()
	if n <= 0 {
		return 0, 0
	}
	var wc, bc int
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			r8, g8, bl8 := int(c.R), int(c.G), int(c.B)
			mx := max(r8, g8, bl8)
			mn := min(r8, g8, bl8)
			if mn >= 200 && mx-mn <= 50 {
				wc++
			}
			if bl8 >= 140 && bl8-r8 >= 50 && bl8-g8 >= 20 {
				bc++
			}
		}
	}
	return float64(wc) / float64(n), float64(bc) / float64(n)
}

// scoreAll classifies every spot against a captured frame.
func scoreAll(img *image.RGBA, spots []Spot) []Metrics {
	out := make([]Metrics, len(spots))
	for i, s := range spots {
		w, b := classifyRoi(img, s.ROI)
		out[i] = Metrics{Spot: s, WhiteFrac: w, BlueFrac: b, Score: w + 0.25*b}
	}
	return out
}

// best returns the highest-scoring spot that clears threshold, provided it also
// beats the runner-up by margin. ok=false with ambiguous=true means two spots
// scored too close to choose safely (the runner ignores that poll).
func best(m []Metrics, threshold, margin float64) (idx int, ok, ambiguous bool) {
	if len(m) == 0 {
		return 0, false, false
	}
	first, second := -1, -1
	for i := range m {
		if first < 0 || m[i].Score > m[first].Score {
			second = first
			first = i
		} else if second < 0 || m[i].Score > m[second].Score {
			second = i
		}
	}
	if first < 0 || m[first].WhiteFrac < threshold {
		return 0, false, false
	}
	if second >= 0 && m[first].Score < m[second].Score*margin {
		return first, false, true
	}
	return first, true, false
}
