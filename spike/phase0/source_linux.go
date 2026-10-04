//go:build linux

package main

import (
	"fmt"
	"image"
	"image/draw"
	"os"

	x11 "github.com/go-vgo/robotgo/x11"
	"github.com/kbinani/screenshot"
)

// screen is the Linux capture + input backend.
//
// Everything is expressed in the TARGET monitor's own coordinate space, with
// (0,0) at its top-left. On KDE Plasma Wayland, capture uses KWin's
// ScreenShot2 D-Bus API (fast, region-based); otherwise it falls back to
// kbinani/screenshot, whose Wayland path goes through the desktop portal and
// returns the whole desktop (~2 fps).
type screen struct {
	kwin       *kwinCapture
	kwinReason string
}

// displayIndex selects which monitor to capture; -1 means the primary.
var displayIndex = -1

// capturePathName is the active capture backend, for diagnostics.
var capturePathName = "unknown"

func openScreen() (*screen, error) {
	if screenshot.NumActiveDisplays() <= 0 {
		return nil, fmt.Errorf("no active displays; is DISPLAY set?")
	}
	if targetRect().Empty() {
		return nil, fmt.Errorf("no target display found (index %d)", displayIndex)
	}
	s := &screen{}
	if k, err := newKWinCapture(); err == nil {
		s.kwin = k
		capturePathName = "kwin screenshots2 (region, fast)"
	} else {
		s.kwinReason = err.Error()
		capturePathName = "kbinani portal (whole desktop, slow)"
	}
	return s, nil
}

func (s *screen) Name() string {
	return fmt.Sprintf("capture=%s; input=robotgo/x11 XTEST (display %d)", capturePathName, targetIndex())
}

// unionRect is the bounding box of every display, in Xinerama coordinates
// (relative to the primary).
func unionRect() image.Rectangle {
	n := screenshot.NumActiveDisplays()
	u := screenshot.GetDisplayBounds(0)
	for i := 1; i < n; i++ {
		u = u.Union(screenshot.GetDisplayBounds(i))
	}
	return u
}

func primaryRect() image.Rectangle {
	n := screenshot.NumActiveDisplays()
	for i := 0; i < n; i++ {
		if image.Pt(0, 0).In(screenshot.GetDisplayBounds(i)) {
			return screenshot.GetDisplayBounds(i)
		}
	}
	if n > 0 {
		return screenshot.GetDisplayBounds(0)
	}
	return image.Rectangle{}
}

func targetIndex() int {
	n := screenshot.NumActiveDisplays()
	if displayIndex >= 0 && displayIndex < n {
		return displayIndex
	}
	for i := 0; i < n; i++ {
		if image.Pt(0, 0).In(screenshot.GetDisplayBounds(i)) {
			return i
		}
	}
	return 0
}

func targetRect() image.Rectangle {
	if screenshot.NumActiveDisplays() == 0 {
		return image.Rectangle{}
	}
	return screenshot.GetDisplayBounds(targetIndex())
}

func (s *screen) Bounds() (image.Rectangle, error) {
	tr := targetRect()
	if tr.Empty() {
		return image.Rectangle{}, fmt.Errorf("no target display")
	}
	return image.Rect(0, 0, tr.Dx(), tr.Dy()), nil
}

// globalOffset maps target-local coordinates to KWin's global layout space
// (target origin minus the layout origin).
func globalOffset() (int, int) {
	t := targetRect().Min
	u := unionRect().Min
	return t.X - u.X, t.Y - u.Y
}

// captureOffset is where the target display starts inside the image returned by
// kbinani/screenshot, which differs by session type:
//
//   - X11 (captureXinerama): Capture(x,y) is relative to the primary, and
//     kbinani adds the primary's absolute origin internally.
//   - Wayland (captureDbus): the portal returns the WHOLE desktop and kbinani
//     crops at (x,y), so the offset is the target origin minus the virtual
//     origin.
func captureOffset() (int, int) {
	t := targetRect().Min
	if os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		u := unionRect().Min
		return t.X - u.X, t.Y - u.Y
	}
	return t.X, t.Y
}

func capturePath() string { return capturePathName }

// Grab captures the target monitor and returns an image with bounds (0,0).
func (s *screen) Grab() (*image.RGBA, error) {
	tr := targetRect()
	if tr.Empty() {
		return nil, fmt.Errorf("no target display")
	}
	if s.kwin != nil {
		ox, oy := globalOffset()
		img, _, err := s.kwin.capture(ox, oy, uint32(tr.Dx()), uint32(tr.Dy()))
		if err == nil {
			return img, nil
		}
		// fall through to kbinani on transient failure
	}
	ox, oy := captureOffset()
	img, err := screenshot.Capture(ox, oy, tr.Dx(), tr.Dy())
	if err != nil {
		return nil, fmt.Errorf("capture display %d: %w", targetIndex(), err)
	}
	return img, nil
}

// GrabRect captures the target monitor and crops one target-local region.
func (s *screen) GrabRect(r Rect) (*image.RGBA, error) {
	full, err := s.Grab()
	if err != nil {
		return nil, err
	}
	return crop(full, r.Bounds()), nil
}

// GrabUnion captures just the union of the given spots (target-local) and
// returns an image whose bounds are that union, so callers can index it with
// the spots' own coordinates. Used by the watch loop.
func (s *screen) GrabUnion(spots []Spot) (*image.RGBA, error) {
	u := spotsUnion(spots)
	if u.W <= 0 || u.H <= 0 {
		return s.Grab()
	}
	if s.kwin != nil {
		ox, oy := globalOffset()
		img, _, err := s.kwin.capture(u.X+ox, u.Y+oy, uint32(u.W), uint32(u.H))
		if err == nil {
			return shiftBounds(img, u.X, u.Y), nil
		}
	}
	full, err := s.Grab()
	if err != nil {
		return nil, err
	}
	return shiftBounds(crop(full, u.Bounds()), u.X, u.Y), nil
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

func tapKey(key string) error { return x11.KeyTap(key) }

func mousePos() (int, int) {
	rx, ry := x11.GetMousePos()
	u := unionRect().Min
	t := targetRect().Min
	return rx + u.X - t.X, ry + u.Y - t.Y
}

func moveMouse(x, y int) error {
	u := unionRect().Min
	t := targetRect().Min
	x11.Move(x-u.X+t.X, y-u.Y+t.Y)
	return nil
}

func activeTitle() string { return x11.GetTitle() }

func unionOrigin() (int, int) { return unionRect().Min.X, unionRect().Min.Y }

func displayBounds(i int) (Rect, bool) {
	if i < 0 || i >= screenshot.NumActiveDisplays() {
		return Rect{}, false
	}
	r := screenshot.GetDisplayBounds(i)
	return Rect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}, true
}

func screenSize() (int, int) {
	tr := targetRect()
	return tr.Dx(), tr.Dy()
}

func displayCount() int { return screenshot.NumActiveDisplays() }
