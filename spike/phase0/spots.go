package main

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"strings"
)

// Rect is a region in virtual-desktop physical pixels.
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Bounds converts the rect to an image.Rectangle.
func (r Rect) Bounds() image.Rectangle {
	return image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H)
}

func (r Rect) String() string {
	return fmt.Sprintf("(%d,%d %dx%d)", r.X, r.Y, r.W, r.H)
}

// Spot is one prompt location and the key it commands.
type Spot struct {
	ID  string `json:"id"`
	Key string `json:"key"`
	ROI Rect   `json:"roi"`
}

const (
	// defaultROIW and defaultROIH are the observed prompt-box size at 1080p.
	defaultROIW = 180
	defaultROIH = 56
)

// defaultSpots are the approximate prompt centers read from the 1080p
// screenshots in DESIGN.md Appendix A, expanded to ROI boxes. They are a
// starting point for a maximized game window, not shipped truth; calibrate.
func defaultSpots() []Spot {
	mk := func(id, key string, cx, cy int) Spot {
		return Spot{
			ID:  id,
			Key: key,
			ROI: Rect{cx - defaultROIW/2, cy - defaultROIH/2, defaultROIW, defaultROIH},
		}
	}
	return []Spot{
		mk("s_a", "a", 1116, 698),
		mk("s_w", "w", 1253, 604),
		mk("s_d", "d", 1373, 683),
		mk("s_s", "s", 1242, 800),
	}
}

// loadSpots returns the spots from a JSON file, or the built-in defaults when
// path is empty. Every spot needs a non-empty id and key, and a positive ROI.
func loadSpots(path string) ([]Spot, error) {
	if strings.TrimSpace(path) == "" {
		return defaultSpots(), nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spots %s: %w", path, err)
	}
	var spots []Spot
	if err := json.Unmarshal(raw, &spots); err != nil {
		return nil, fmt.Errorf("parse spots %s: %w", path, err)
	}
	if len(spots) == 0 {
		return nil, fmt.Errorf("spots %s: empty", path)
	}
	for i, s := range spots {
		if s.ID == "" || s.Key == "" {
			return nil, fmt.Errorf("spots %s: entry %d needs id and key", path, i)
		}
		if s.ROI.W <= 0 || s.ROI.H <= 0 {
			return nil, fmt.Errorf("spots %s: entry %s needs a positive roi", path, s.ID)
		}
	}
	return spots, nil
}

func spotIDs(spots []Spot) string {
	ids := make([]string, len(spots))
	for i, s := range spots {
		ids[i] = s.ID + "=" + s.Key
	}
	return strings.Join(ids, " ")
}
