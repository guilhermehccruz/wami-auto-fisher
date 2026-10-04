//go:build linux

package fisher

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const launcherName = "wami-auto-fisher"

// WriteLauncher installs a proper launcher entry and icon for the current
// binary under ~/.local/share, and (on KDE) refreshes the service cache. It
// doubles as the KWin ScreenShot2 authorization: the .desktop carries
// X-KDE-DBUS-Restricted-Interfaces and an absolute Exec matching /proc/<pid>/exe.
//
// It is skipped for transient executables (go run / go-build) so a dev run does
// not install a broken launcher.
func WriteLauncher(icon []byte) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	if strings.Contains(exe, "go-build") || strings.HasPrefix(exe, os.TempDir()) {
		return "", fmt.Errorf("skipping launcher install for transient executable %s", exe)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	if len(icon) > 0 {
		iconDir := filepath.Join(home, ".local", "share", "icons", "hicolor", "256x256", "apps")
		if err := os.MkdirAll(iconDir, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(iconDir, launcherName+".png"), icon, 0o644); err != nil {
			return "", err
		}
	}

	appDir := filepath.Join(home, ".local", "share", "applications")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", err
	}
	desktopPath := filepath.Join(appDir, launcherName+".desktop")
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=WAMI Auto Fisher
Comment=Automates the WAMI active fishing minigame
Exec=%s
Icon=%s
Terminal=false
Categories=Utility;Game;
Keywords=fishing;automation;game;
X-KDE-DBUS-Restricted-Interfaces=org.kde.KWin.ScreenShot2
`, exe, launcherName)
	if err := os.WriteFile(desktopPath, []byte(content), 0o644); err != nil {
		return "", err
	}

	refreshCaches(appDir)
	return desktopPath, nil
}

// SetupKWin installs the launcher entry (which authorizes KWin ScreenShot2) and
// rebuilds the KDE service cache.
func SetupKWin(icon []byte) (string, error) {
	path, err := WriteLauncher(icon)
	if err != nil {
		return "", err
	}
	time.Sleep(1500 * time.Millisecond)
	return path, nil
}

func refreshCaches(appDir string) {
	if cacheDir, err := os.UserCacheDir(); err == nil {
		if matches, _ := filepath.Glob(filepath.Join(cacheDir, "ksycoca6*")); len(matches) > 0 {
			for _, m := range matches {
				_ = os.Remove(m)
			}
		}
	}
	runQuiet("update-desktop-database", appDir)
	for _, tool := range []string{"kbuildsycoca6", "kbuildsycoca5"} {
		if _, err := exec.LookPath(tool); err == nil {
			runQuiet(tool, "--noincremental")
			break
		}
	}
}

func runQuiet(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	_ = cmd.Run()
}
