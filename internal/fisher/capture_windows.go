//go:build windows

package fisher

import (
	"errors"
	"image"
	"image/draw"

	win "github.com/go-vgo/robotgo/win"
	"github.com/kbinani/screenshot"
)

// Screen is the Windows capture + input backend. Capture uses kbinani/screenshot
// (GDI BitBlt); input uses robotgo/win SendInput.
type Screen struct {
	path string
}

var displayIndex = -1

func OpenScreen(display int) (*Screen, error) {
	displayIndex = display
	if screenshot.NumActiveDisplays() <= 0 {
		return nil, errors.New("no active displays")
	}
	return &Screen{path: "kbinani/screenshot (GDI) + robotgo/win SendInput"}, nil
}

func (s *Screen) Name() string { return s.path }

func (s *Screen) Bounds() (image.Rectangle, error) {
	r := targetRect()
	if r.Empty() {
		return image.Rectangle{}, errors.New("no target display")
	}
	return image.Rect(0, 0, r.Dx(), r.Dy()), nil
}

func (s *Screen) Grab() (*image.RGBA, error) {
	tr := targetRect()
	if tr.Empty() {
		return nil, errors.New("no target display")
	}
	// kbinani CaptureRect returns an image whose origin is the display's.
	src, err := screenshot.CaptureRect(tr)
	if err != nil {
		return nil, err
	}
	dst := image.NewRGBA(image.Rect(0, 0, tr.Dx(), tr.Dy()))
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst, nil
}

func (s *Screen) GrabUnion(spots []Spot) (*image.RGBA, error) {
	u := spotsUnion(spots)
	if u.Empty() {
		return s.Grab()
	}
	full, err := s.Grab()
	if err != nil {
		return nil, err
	}
	return shiftBounds(crop(full, u.Bounds()), u.X, u.Y), nil
}

func (s *Screen) TapKey(key string) error { return win.KeyTap(key) }
func (s *Screen) ActiveTitle() string     { return win.GetTitle() }
func (s *Screen) CursorPos() (int, int)   { return win.GetMousePos() }
func (s *Screen) DisplayCount() int       { return screenshot.NumActiveDisplays() }

// Display describes one monitor.
type Display struct {
	Index   int  `json:"index"`
	X       int  `json:"x"`
	Y       int  `json:"y"`
	W       int  `json:"w"`
	H       int  `json:"h"`
	Primary bool `json:"primary"`
}

func (s *Screen) Displays() []Display {
	n := screenshot.NumActiveDisplays()
	out := make([]Display, 0, n)
	for i := 0; i < n; i++ {
		r := screenshot.GetDisplayBounds(i)
		out = append(out, Display{Index: i, X: r.Min.X, Y: r.Min.Y, W: r.Dx(), H: r.Dy(), Primary: i == 0})
	}
	return out
}

func targetIndex() int {
	n := screenshot.NumActiveDisplays()
	if displayIndex >= 0 && displayIndex < n {
		return displayIndex
	}
	return 0
}

func targetRect() image.Rectangle {
	if screenshot.NumActiveDisplays() == 0 {
		return image.Rectangle{}
	}
	return screenshot.GetDisplayBounds(targetIndex())
}

func spotsUnion(spots []Spot) Rect {
	if len(spots) == 0 {
		return Rect{}
	}
	r := spots[0].ROI.Bounds()
	for _, s := range spots[1:] {
		r = r.Union(s.ROI.Bounds())
	}
	return Rect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}
}

func shiftBounds(src *image.RGBA, minX, minY int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(minX, minY, minX+src.Bounds().Dx(), minY+src.Bounds().Dy()))
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst
}

func crop(src *image.RGBA, r image.Rectangle) *image.RGBA {
	r = r.Intersect(src.Bounds())
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), src, r.Min, draw.Src)
	return dst
}
