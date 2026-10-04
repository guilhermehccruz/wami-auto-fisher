//go:build linux

package main

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// kwinCapture talks to org.kde.KWin.ScreenShot2, which captures an arbitrary
// region at native speed (no PNG round-trip). It is used when the session is
// KDE Plasma Wayland and the process is authorized (see setupKwinCmd).
//
// Coordinates are KWin's global layout coordinates (the same space as
// kdotool), i.e. target-local + the virtual origin.
type kwinCapture struct {
	conn  *dbus.Conn
	scale float64
}

const (
	kwinDest  = "org.kde.KWin"
	kwinIface = "org.kde.KWin.ScreenShot2"
)

func kwinPath() dbus.ObjectPath { return dbus.ObjectPath("/org/kde/KWin/ScreenShot2") }

// newKWinCapture connects and probes a 2x2 capture, which both verifies the
// interface exists and that we are authorized (it returns
// "The process is not authorized to take a screenshot" otherwise).
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

// capture grabs a wxh region at global (x,y) and returns it as RGBA.
func (k *kwinCapture) capture(x, y int, w, h uint32) (*image.RGBA, map[string]dbus.Variant, error) {
	if w == 0 || h == 0 {
		return nil, nil, fmt.Errorf("kwin: empty region")
	}
	obj := k.conn.Object(kwinDest, kwinPath())
	opts := map[string]dbus.Variant{"include-cursor": dbus.MakeVariant(false)}

	r, wr, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()

	start := time.Now()
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
	_ = start

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
	if format != 4 && format != 5 && format != 6 { // RGB32 / ARGB32 / ARGB32_Premultiplied
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

// setupKwinCmd writes the .desktop file KWin needs to authorize this binary for
// ScreenShot2, then refreshes the KDE service cache.
func setupKwinCmd(args []string) {
	exe, err := os.Executable()
	if err != nil {
		fatal(err)
	}
	exe, _ = filepath.EvalSymlinks(exe)

	home, err := os.UserHomeDir()
	if err != nil {
		fatal(err)
	}
	dir := filepath.Join(home, ".local", "share", "applications")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fatal(err)
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
		fatal(err)
	}
	fmt.Println("wrote", path, "with Exec="+exe)

	// A stale sycoca cache keeps KWin from seeing a changed Exec, so clear it.
	if cacheDir, err := os.UserCacheDir(); err == nil {
		if matches, _ := filepath.Glob(filepath.Join(cacheDir, "ksycoca6*")); len(matches) > 0 {
			for _, m := range matches {
				_ = os.Remove(m)
			}
			fmt.Printf("cleared %d sycoca cache file(s)\n", len(matches))
		}
	}

	for _, tool := range []string{"kbuildsycoca6", "kbuildsycoca5"} {
		if _, err := exec.LookPath(tool); err == nil {
			cmd := exec.Command(tool, "--noincremental")
			cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
			_ = cmd.Run()
			fmt.Println("rebuilt service cache with", tool, "--noincremental")
			break
		}
	}
	time.Sleep(1500 * time.Millisecond)
	fmt.Println("\nNow run the same binary path:")
	fmt.Println("  " + exe + " probe   # capture path should say 'kwin screenshots2'")
	fmt.Println("Note: if you rebuild to a different path, run setup-kwin again.")
}

func kwinAvailable() bool {
	k, err := newKWinCapture()
	if k != nil {
		k.Close()
	}
	return err == nil
}

func kwinError() string {
	k, err := newKWinCapture()
	if k != nil {
		k.Close()
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func shorten(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
