package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"wami-auto-fisher/internal/fisher"
)

// Service is the Go surface bound to the frontend.
type Service struct {
	app *application.App

	mu        sync.Mutex
	screen    *fisher.Screen
	runner    *fisher.Runner
	spots     []fisher.Spot
	cfg       fisher.Config
	path      string
	screenErr string
}

type Diagnostics struct {
	CapturePath string           `json:"capturePath"`
	ActiveTitle string           `json:"activeTitle"`
	Displays    []fisher.Display `json:"displays"`
	ConfigPath  string           `json:"configPath"`
	Error       string           `json:"error,omitempty"`
}

type configFile struct {
	Spots  []fisher.Spot `json:"spots"`
	Config fisher.Config `json:"config"`
}

func NewService() *Service { return &Service{} }

func (s *Service) SetApp(app *application.App) { s.app = app }

func defaultSpots() []fisher.Spot {
	return []fisher.Spot{
		{ID: "s_a", Key: "a"},
		{ID: "s_w", Key: "w"},
		{ID: "s_d", Key: "d"},
		{ID: "s_s", Key: "s"},
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
	s.spots = defaultSpots()

	if b, err := os.ReadFile(s.path); err == nil {
		var cf configFile
		if json.Unmarshal(b, &cf) == nil {
			if len(cf.Spots) > 0 {
				s.spots = cf.Spots
			}
			if cf.Config.PollMs > 0 {
				s.cfg = cf.Config
			}
		}
	}

	screen, err := fisher.OpenScreen(-1)
	if err != nil {
		s.screenErr = err.Error()
	} else {
		s.screen = screen
		s.runner = fisher.NewRunner(screen, screen, s.spots, s.cfg, func(st fisher.Status) {
			if s.app != nil {
				s.app.Event.Emit("status", st)
			}
		})
	}
	return s.saveLocked()
}

func (s *Service) saveLocked() error {
	b, err := json.MarshalIndent(configFile{Spots: s.spots, Config: s.cfg}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(s.path, b, 0o644)
}

// --- config / spots ---------------------------------------------------------

func (s *Service) GetSpots() []fisher.Spot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]fisher.Spot(nil), s.spots...)
}

func (s *Service) SetSpots(spots []fisher.Spot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spots = spots
	if s.runner != nil {
		s.runner.SetSpots(spots, s.cfg)
	}
	return s.saveLocked()
}

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
		s.runner.SetSpots(s.spots, cfg)
	}
	return s.saveLocked()
}

// --- runner -----------------------------------------------------------------

func (s *Service) Start() error {
	s.mu.Lock()
	r := s.runner
	s.mu.Unlock()
	if r == nil {
		return errors.New("capture backend unavailable: " + s.screenErr)
	}
	if len(s.GetSpots()) == 0 {
		return errors.New("configure at least one spot first")
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
	s.mu.Unlock()
	if r == nil {
		return fisher.Status{Error: s.screenErr}
	}
	return r.Snapshot()
}

// --- diagnostics / calibration ---------------------------------------------

func (s *Service) Diagnostics() Diagnostics {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := Diagnostics{ConfigPath: s.path, Error: s.screenErr}
	if s.screen != nil {
		d.CapturePath = s.screen.Name()
		d.ActiveTitle = s.screen.ActiveTitle()
		d.Displays = s.screen.Displays()
	}
	return d
}

// SetupKWin authorizes this binary for fast KWin ScreenShot2 capture (Linux KDE).
func (s *Service) SetupKWin() (string, error) {
	path, err := fisher.SetupKWin()
	if err != nil {
		return "", err
	}
	// Re-open the screen so the new capture path is picked up.
	if screen, err := fisher.OpenScreen(-1); err == nil {
		s.mu.Lock()
		s.screen = screen
		if s.runner != nil {
			s.runner = fisher.NewRunner(screen, screen, s.spots, s.cfg, func(st fisher.Status) {
				if s.app != nil {
					s.app.Event.Emit("status", st)
				}
			})
		}
		s.mu.Unlock()
	}
	return path, nil
}

// Preview is a calibration screenshot plus its pixel size.
type Preview struct {
	PNG string `json:"png"`
	W   int    `json:"w"`
	H   int    `json:"h"`
}

// CapturePreview returns a base64 PNG of the target monitor, for calibration.
func (s *Service) CapturePreview() (Preview, error) {
	s.mu.Lock()
	sc := s.screen
	s.mu.Unlock()
	if sc == nil {
		return Preview{}, errors.New("capture backend unavailable: " + s.screenErr)
	}
	img, err := sc.Grab()
	if err != nil {
		return Preview{}, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return Preview{}, err
	}
	return Preview{
		PNG: base64.StdEncoding.EncodeToString(buf.Bytes()),
		W:   img.Bounds().Dx(),
		H:   img.Bounds().Dy(),
	}, nil
}

// Cursor returns the pointer position in target-monitor coordinates.
func (s *Service) Cursor() (int, int) {
	s.mu.Lock()
	sc := s.screen
	s.mu.Unlock()
	if sc == nil {
		return 0, 0
	}
	return sc.CursorPos()
}
