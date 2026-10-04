package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"wami-auto-fisher/internal/fisher"
)

// appIconPNG is embedded so the app can install its own launcher icon without
// shipping the asset alongside the binary.
//
//go:embed build/appicon.png
var appIconPNG []byte

// Service is the Go surface bound to the frontend.
type Service struct {
	app *application.App

	mu        sync.Mutex
	screen    *fisher.Screen
	runner    *fisher.Runner
	cfg       fisher.Config
	path      string
	screenErr string
}

type Diagnostics struct {
	CapturePath string           `json:"capturePath"`
	ActiveTitle string           `json:"activeTitle"`
	Displays    []fisher.Display `json:"displays"`
	ConfigPath  string           `json:"configPath"`
	Platform    string           `json:"platform"`
	Version     string           `json:"version"`
	Error       string           `json:"error,omitempty"`
}

type configFile struct {
	Config fisher.Config `json:"config"`
}

func NewService() *Service { return &Service{} }

func (s *Service) SetApp(app *application.App) { s.app = app }

// fixedSpots are the coordinates supplied by the operator, converted to the
// target monitor's local space (Linux global x minus the 1920 layout offset;
// Windows uses the same values with the game window at the monitor's top-left).
// The app does not calibrate: these are used as-is on both platforms.
func fixedSpots() []fisher.Spot {
	return []fisher.Spot{
		{ID: "s_a", Key: "a", ROI: fisher.Rect{X: 969, Y: 581, W: 109, H: 42}},
		{ID: "s_w", Key: "w", ROI: fisher.Rect{X: 1102, Y: 506, W: 111, H: 43}},
		{ID: "s_s", Key: "s", ROI: fisher.Rect{X: 1102, Y: 655, W: 109, H: 41}},
		{ID: "s_d", Key: "d", ROI: fisher.Rect{X: 1235, Y: 580, W: 109, H: 43}},
	}
}

// Startup loads config, opens the capture backend and builds the runner.
func (s *Service) Startup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	base := filepath.Join(dir, "wami-auto-fisher")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	s.path = filepath.Join(base, "config.json")
	s.cfg = fisher.DefaultConfig()

	if b, err := os.ReadFile(s.path); err == nil {
		// Pre-fill with defaults so a config written before a field existed
		// (e.g. idleTimeoutMs) keeps that field's default instead of 0.
		cf := configFile{Config: fisher.DefaultConfig()}
		if err := json.Unmarshal(b, &cf); err == nil {
			s.cfg = cf.Config
		}
	}

	screen, err := fisher.OpenScreen(-1)
	if err != nil {
		s.screenErr = err.Error()
	} else {
		s.screen = screen
		spots := fixedSpots()
		s.runner = fisher.NewRunner(screen, screen, spots, s.cfg, func(st fisher.Status) {
			if s.app != nil {
				s.app.Event.Emit("status", st)
			}
		})
	}
	// Keep the desktop launcher entry (and its icon) current. On Linux this also
	// authorizes KWin ScreenShot2; elsewhere it is a no-op.
	_, _ = fisher.WriteLauncher(appIconPNG)
	return s.saveLocked()
}

func (s *Service) saveLocked() error {
	b, err := json.MarshalIndent(configFile{Config: s.cfg}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(s.path, b, 0o644)
}

func (s *Service) GetSpots() []fisher.Spot { return fixedSpots() }

func (s *Service) GetConfig() fisher.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Service) SetConfig(cfg fisher.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
	if s.runner != nil {
		s.runner.SetSpots(fixedSpots(), cfg)
	}
	return s.saveLocked()
}

// --- runner -----------------------------------------------------------------

func (s *Service) Start() error {
	s.mu.Lock()
	r := s.runner
	errMsg := s.screenErr
	s.mu.Unlock()
	if r == nil {
		return errors.New("capture backend unavailable: " + errMsg)
	}
	r.Start()
	return nil
}

func (s *Service) Stop() {
	s.mu.Lock()
	r := s.runner
	s.mu.Unlock()
	if r != nil {
		r.Stop()
	}
}

func (s *Service) Status() fisher.Status {
	s.mu.Lock()
	r := s.runner
	errMsg := s.screenErr
	s.mu.Unlock()
	if r == nil {
		return fisher.Status{Error: errMsg}
	}
	return r.Snapshot()
}

// --- diagnostics ------------------------------------------------------------

func (s *Service) Diagnostics() Diagnostics {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := Diagnostics{ConfigPath: s.path, Error: s.screenErr, Platform: runtime.GOOS, Version: version}
	if s.screen != nil {
		d.CapturePath = s.screen.Name()
		d.ActiveTitle = s.screen.ActiveTitle()
		d.Displays = s.screen.Displays()
	}
	return d
}

// SetupKWin authorizes this binary for fast KWin ScreenShot2 capture (Linux KDE).
func (s *Service) SetupKWin() (string, error) {
	path, err := fisher.SetupKWin(appIconPNG)
	if err != nil {
		return "", err
	}
	if screen, err := fisher.OpenScreen(-1); err == nil {
		s.mu.Lock()
		s.screen = screen
		s.screenErr = ""
		s.runner = fisher.NewRunner(screen, screen, fixedSpots(), s.cfg, func(st fisher.Status) {
			if s.app != nil {
				s.app.Event.Emit("status", st)
			}
		})
		s.mu.Unlock()
	}
	return path, nil
}
