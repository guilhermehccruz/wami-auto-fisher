// Package fisher is the WAMI Auto Fisher core: the prompt classifier and the
// capture → detect → tap loop. It is UI-agnostic so the Wails service and tests
// can drive it.
package fisher

import (
	"image"
	"sync"
	"time"
)

// Rect is a region in target-monitor pixels (origin at the monitor's top-left).
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

func (r Rect) Bounds() image.Rectangle { return image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H) }
func (r Rect) Empty() bool             { return r.W <= 0 || r.H <= 0 }

// Spot is one prompt location and the key it commands.
type Spot struct {
	ID  string `json:"id"`
	Key string `json:"key"`
	ROI Rect   `json:"roi"`
}

// Config tunes the detector and the loop.
type Config struct {
	PollMs         int     `json:"pollMs"`
	WhiteThreshold float64 `json:"whiteThreshold"`
	ClearThreshold float64 `json:"clearThreshold"`
	Margin         float64 `json:"margin"`
	RefractoryMs   int     `json:"refractoryMs"`
	RetryMs        int     `json:"retryMs"`
}

func DefaultConfig() Config {
	return Config{
		PollMs:         40,
		WhiteThreshold: 0.35,
		ClearThreshold: 0.15,
		Margin:         1.25,
		RefractoryMs:   100,
		RetryMs:        250,
	}
}

// Metrics is the per-spot classifier output for one frame.
type Metrics struct {
	Spot  Spot    `json:"spot"`
	White float64 `json:"white"`
	Blue  float64 `json:"blue"`
	Score float64 `json:"score"`
}

// Classify scores one region: a prompt is a near-white box with a blue border
// over a darker green/blue scene.
func Classify(img *image.RGBA, r Rect) (white, blue float64) {
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

// ScoreAll classifies every spot against a captured frame.
func ScoreAll(img *image.RGBA, spots []Spot) []Metrics {
	out := make([]Metrics, len(spots))
	for i, s := range spots {
		w, b := Classify(img, s.ROI)
		out[i] = Metrics{Spot: s, White: w, Blue: b, Score: w + 0.25*b}
	}
	return out
}

// Best returns the highest-scoring spot that clears threshold and beats the
// runner-up by margin. ambiguous means two spots were too close to choose.
func Best(m []Metrics, threshold, margin float64) (idx int, ok, ambiguous bool) {
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
	if first < 0 || m[first].White < threshold {
		return 0, false, false
	}
	if second >= 0 && m[first].Score < m[second].Score*margin {
		return first, false, true
	}
	return first, true, false
}

// Capture grabs screenshots. GrabUnion returns an image whose bounds are the
// union of the spots, so callers index it with the spots' own coordinates.
type Capture interface {
	GrabUnion(spots []Spot) (*image.RGBA, error)
	Grab() (*image.RGBA, error)
	Name() string
	Bounds() (image.Rectangle, error)
}

// Input injects the detected key.
type Input interface {
	TapKey(key string) error
	ActiveTitle() string
}

// Status is the live runner state pushed to the UI.
type Status struct {
	Running   bool      `json:"running"`
	Spots     []Metrics `json:"spots"`
	Best      int       `json:"best"`
	Ambiguous bool      `json:"ambiguous"`
	Presses   int       `json:"presses"`
	LastKey   string    `json:"lastKey"`
	ElapsedMs int64     `json:"elapsedMs"`
	Error     string    `json:"error,omitempty"`
}

// Runner owns the detect loop.
type Runner struct {
	cap      Capture
	in       Input
	onChange func(Status)

	mu      sync.Mutex
	spots   []Spot
	cfg     Config
	status  Status
	stop    chan struct{}
	running bool
}

func NewRunner(c Capture, in Input, spots []Spot, cfg Config, onChange func(Status)) *Runner {
	return &Runner{cap: c, in: in, spots: spots, cfg: cfg, onChange: onChange}
}

func (r *Runner) Snapshot() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *Runner) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// SetSpots updates the spots and config used by the next run (safe while idle).
func (r *Runner) SetSpots(spots []Spot, cfg Config) {
	r.mu.Lock()
	r.spots = spots
	r.cfg = cfg
	r.mu.Unlock()
}

func (r *Runner) Start() {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.status = Status{Running: true}
	r.stop = make(chan struct{})
	stop := r.stop
	spots := append([]Spot(nil), r.spots...)
	cfg := r.cfg
	r.mu.Unlock()

	r.emit()
	go r.loop(stop, spots, cfg)
}

func (r *Runner) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	r.running = false
	close(r.stop)
	r.mu.Unlock()
}

func (r *Runner) emit() {
	if r.onChange != nil {
		r.onChange(r.Snapshot())
	}
}

func (r *Runner) loop(stop chan struct{}, spots []Spot, cfg Config) {
	interval := time.Duration(cfg.PollMs) * time.Millisecond
	if interval <= 0 {
		interval = 40 * time.Millisecond
	}
	refractory := time.Duration(cfg.RefractoryMs) * time.Millisecond
	retry := time.Duration(cfg.RetryMs) * time.Millisecond

	armed := make(map[string]bool, len(spots))
	lastFire := make(map[string]time.Time, len(spots))
	for _, s := range spots {
		armed[s.ID] = true
	}

	start := time.Now()
	next := start
	presses := 0
	lastKey := ""

	for {
		select {
		case <-stop:
			r.finish()
			return
		default:
		}
		next = next.Add(interval)
		if d := time.Until(next); d > 0 {
			t := time.NewTimer(d)
			select {
			case <-stop:
				t.Stop()
				r.finish()
				return
			case <-t.C:
			}
		}

		frame, err := r.cap.GrabUnion(spots)
		if err != nil {
			r.patch(func(st *Status) { st.Error = err.Error() })
			r.emit()
			continue
		}
		scores := ScoreAll(frame, spots)
		idx, ok, ambiguous := Best(scores, cfg.WhiteThreshold, cfg.Margin)
		if ok {
			s := scores[idx].Spot
			now := time.Now()
			switch {
			case armed[s.ID] && now.Sub(lastFire[s.ID]) >= refractory:
				if err := r.in.TapKey(s.Key); err != nil {
					r.patch(func(st *Status) { st.Error = err.Error() })
				} else {
					presses++
					lastKey = s.Key
				}
				armed[s.ID] = false
				lastFire[s.ID] = now
			case !armed[s.ID] && now.Sub(lastFire[s.ID]) >= retry:
				_ = r.in.TapKey(s.Key)
				presses++
				lastKey = s.Key
				lastFire[s.ID] = now
			}
		}
		for i := range scores {
			if ok && i == idx {
				continue
			}
			if scores[i].White < cfg.ClearThreshold {
				armed[scores[i].Spot.ID] = true
			}
		}
		r.patch(func(st *Status) {
			st.Spots = scores
			st.Best = idx
			st.Ambiguous = ambiguous
			st.Presses = presses
			st.LastKey = lastKey
			st.ElapsedMs = time.Since(start).Milliseconds()
		})
		r.emit()
	}
}

func (r *Runner) patch(f func(*Status)) {
	r.mu.Lock()
	f(&r.status)
	r.mu.Unlock()
}

func (r *Runner) finish() {
	r.mu.Lock()
	r.running = false
	r.status.Running = false
	r.mu.Unlock()
	r.emit()
}
