//go:build linux

package fisher

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	x11 "github.com/go-vgo/robotgo/x11"
	"github.com/godbus/dbus/v5"
	"github.com/kbinani/screenshot"
)

// Screen is the Linux capture + input backend. Capture prefers KWin's
// ScreenShot2 (KDE Wayland, region, fast) and falls back to kbinani's portal
// path (whole desktop, slow; all-black XGetImage under Wayland is avoided).
type Screen struct {
	kwin *kwinCapture
	path string
}

// displayIndex selects which monitor to capture; -1 means the primary.
var displayIndex = -1

func OpenScreen(display int) (*Screen, error) {
	displayIndex = display
	if screenshot.NumActiveDisplays() <= 0 {
		return nil, errors.New("no active displays (is DISPLAY set?)")
	}
	if targetRect().Empty() {
		return nil, fmt.Errorf("no target display for index %d", display)
	}
	s := &Screen{}
	if k, err := newKWinCapture(); err == nil {
		s.kwin = k
		s.path = "kwin screenshots2 (region, fast)"
	} else {
		s.path = "kbinani portal (slow)"
	}
	return s, nil
}

func (s *Screen) Name() string { return s.path }

func (s *Screen) Bounds() (image.Rectangle, error) {
	tr := targetRect()
	if tr.Empty() {
		return image.Rectangle{}, errors.New("no target display")
	}
	return image.Rect(0, 0, tr.Dx(), tr.Dy()), nil
}

func (s *Screen) Grab() (*image.RGBA, error) {
	tr := targetRect()
	if tr.Empty() {
		return nil, errors.New("no target display")
	}
	if s.kwin != nil {
		ox, oy := globalOffset()
		if img, _, err := s.kwin.capture(ox, oy, uint32(tr.Dx()), uint32(tr.Dy())); err == nil {
			return img, nil
		}
	}
	ox, oy := captureOffset()
	img, err := screenshot.Capture(ox, oy, tr.Dx(), tr.Dy())
	if err != nil {
		return nil, fmt.Errorf("capture: %w", err)
	}
	return img, nil
}

func (s *Screen) GrabUnion(spots []Spot) (*image.RGBA, error) {
	u := spotsUnion(spots)
	if u.Empty() {
		return s.Grab()
	}
	if s.kwin != nil {
		ox, oy := globalOffset()
		if img, _, err := s.kwin.capture(u.X+ox, u.Y+oy, uint32(u.W), uint32(u.H)); err == nil {
			return shiftBounds(img, u.X, u.Y), nil
		}
	}
	full, err := s.Grab()
	if err != nil {
		return nil, err
	}
	return shiftBounds(crop(full, u.Bounds()), u.X, u.Y), nil
}

func (s *Screen) TapKey(key string) error { return x11.KeyTap(key) }
func (s *Screen) ActiveTitle() string     { return x11.GetTitle() }
func (s *Screen) CursorPos() (int, int) {
	rx, ry := x11.GetMousePos()
	u := unionRect().Min
	t := targetRect().Min
	return rx + u.X - t.X, ry + u.Y - t.Y
}
func (s *Screen) DisplayCount() int { return screenshot.NumActiveDisplays() }

// Display describes one monitor in Xinerama coordinates.
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
		out = append(out, Display{
			Index: i, X: r.Min.X, Y: r.Min.Y, W: r.Dx(), H: r.Dy(),
			Primary: image.Pt(0, 0).In(r),
		})
	}
	return out
}

// --- geometry ---------------------------------------------------------------

func unionRect() image.Rectangle {
	n := screenshot.NumActiveDisplays()
	u := screenshot.GetDisplayBounds(0)
	for i := 1; i < n; i++ {
		u = u.Union(screenshot.GetDisplayBounds(i))
	}
	return u
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

func globalOffset() (int, int) {
	t := targetRect().Min
	u := unionRect().Min
	return t.X - u.X, t.Y - u.Y
}

func captureOffset() (int, int) {
	t := targetRect().Min
	if os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		u := unionRect().Min
		return t.X - u.X, t.Y - u.Y
	}
	return t.X, t.Y
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

// --- KWin ScreenShot2 -------------------------------------------------------

type kwinCapture struct {
	conn  *dbus.Conn
	scale float64
}

const (
	kwinDest  = "org.kde.KWin"
	kwinIface = "org.kde.KWin.ScreenShot2"
)

func kwinPath() dbus.ObjectPath { return dbus.ObjectPath("/org/kde/KWin/ScreenShot2") }

func newKWinCapture() (*kwinCapture, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	k := &kwinCapture{conn: conn, scale: 1}
	if _, _, err := k.capture(0, 0, 16, 16); err != nil {
		conn.Close()
		return nil, err
	}
	return k, nil
}

func (k *kwinCapture) Close() error { return k.conn.Close() }

func (k *kwinCapture) capture(x, y int, w, h uint32) (*image.RGBA, map[string]dbus.Variant, error) {
	if w == 0 || h == 0 {
		return nil, nil, errors.New("kwin: empty region")
	}
	obj := k.conn.Object(kwinDest, kwinPath())
	opts := map[string]dbus.Variant{"include-cursor": dbus.MakeVariant(false)}
	r, wr, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()
	call := obj.Call(kwinIface+".CaptureArea", 0, x, y, w, h, opts, dbus.UnixFD(wr.Fd()))
	wr.Close()
	if call.Err != nil {
		return nil, nil, call.Err
	}
	var results map[string]dbus.Variant
	if err := call.Store(&results); err != nil {
		return nil, nil, err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	iw := int(results["width"].Value().(uint32))
	ih := int(results["height"].Value().(uint32))
	stride := int(results["stride"].Value().(uint32))
	if sc, ok := results["scale"]; ok {
		if d, ok := sc.Value().(float64); ok {
			k.scale = d
		}
	}
	format := uint32(0)
	if f, ok := results["format"]; ok {
		format, _ = f.Value().(uint32)
	}
	if format != 4 && format != 5 && format != 6 {
		return nil, results, fmt.Errorf("kwin: unsupported QImage format %d", format)
	}
	img := image.NewRGBA(image.Rect(0, 0, iw, ih))
	for yy := 0; yy < ih; yy++ {
		row := yy * stride
		for xx := 0; xx < iw; xx++ {
			o := row + xx*4
			if o+2 >= len(data) {
				break
			}
			img.SetRGBA(xx, yy, color.RGBA{data[o+2], data[o+1], data[o], 255})
		}
	}
	return img, results, nil
}

// SetupKWin writes the .desktop that authorizes this binary for KWin
// ScreenShot2 and rebuilds the KDE service cache. Must be run from the installed
// binary path, not `go run`.
func SetupKWin() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".local", "share", "applications")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "wami-auto-fisher.desktop")
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=WAMI Auto Fisher
Comment=Authorizes KWin ScreenShot2 for fast capture
Exec=%s
X-KDE-DBUS-Restricted-Interfaces=org.kde.KWin.ScreenShot2
`, exe)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	if cacheDir, err := os.UserCacheDir(); err == nil {
		if matches, _ := filepath.Glob(filepath.Join(cacheDir, "ksycoca6*")); len(matches) > 0 {
			for _, m := range matches {
				_ = os.Remove(m)
			}
		}
	}
	for _, tool := range []string{"kbuildsycoca6", "kbuildsycoca5"} {
		if _, err := exec.LookPath(tool); err == nil {
			cmd := exec.Command(tool, "--noincremental")
			cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
			_ = cmd.Run()
			break
		}
	}
	time.Sleep(1500 * time.Millisecond)
	return path, nil
}
