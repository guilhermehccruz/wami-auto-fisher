package fisher

import (
	"image"
	"image/color"
	"testing"
	"time"
)

type fakeCapture struct{ img *image.RGBA }

func (f *fakeCapture) GrabUnion(spots []Spot) (*image.RGBA, error) { return f.img, nil }
func (f *fakeCapture) Grab() (*image.RGBA, error)                  { return f.img, nil }
func (f *fakeCapture) Name() string                                { return "fake" }
func (f *fakeCapture) Bounds() (image.Rectangle, error)            { return f.img.Bounds(), nil }

type fakeInput struct{ taps []string }

func (f *fakeInput) TapKey(key string) error { f.taps = append(f.taps, key); return nil }
func (f *fakeInput) ActiveTitle() string     { return "" }

func blank(w, h int) *image.RGBA { return image.NewRGBA(image.Rect(0, 0, w, h)) }

func withPrompt(w, h int, r Rect) *image.RGBA {
	img := blank(w, h)
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			img.SetRGBA(x, y, color.RGBA{245, 245, 245, 255})
		}
	}
	return img
}

func waitUntil(t *testing.T, cond func() bool, d time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

func TestBestNoneIsMinusOne(t *testing.T) {
	img := blank(64, 64)
	spots := []Spot{{ID: "s_a", Key: "a", ROI: Rect{0, 0, 10, 10}}}
	m := ScoreAll(img, spots)
	idx, ok, ambiguous := Best(m, 0.35, 1.25)
	if ok || ambiguous {
		t.Fatalf("blank frame should not detect: idx=%d ok=%v ambiguous=%v", idx, ok, ambiguous)
	}
}

func TestIdleAutoStop(t *testing.T) {
	c := &fakeCapture{img: blank(64, 64)}
	in := &fakeInput{}
	spots := []Spot{{ID: "s_a", Key: "a", ROI: Rect{0, 0, 20, 20}}}
	cfg := DefaultConfig()
	cfg.PollMs = 5
	cfg.IdleTimeoutMs = 60

	r := NewRunner(c, in, spots, cfg, nil)
	r.Start()
	if !waitUntil(t, func() bool { return !r.Running() }, 2*time.Second) {
		t.Fatal("runner did not idle-stop")
	}
	st := r.Snapshot()
	if st.Best != -1 {
		t.Fatalf("best = %d, want -1", st.Best)
	}
	if st.Error == "" {
		t.Fatal("expected an idle-stop reason in Status.Error")
	}
	if len(in.taps) != 0 {
		t.Fatalf("unexpected taps: %v", in.taps)
	}
}

func TestOneTapPerPrompt(t *testing.T) {
	roi := Rect{10, 10, 30, 20}
	c := &fakeCapture{img: withPrompt(64, 64, roi)}
	in := &fakeInput{}
	spots := []Spot{{ID: "s_a", Key: "a", ROI: roi}}
	cfg := DefaultConfig()
	cfg.PollMs = 5
	cfg.IdleTimeoutMs = 0 // no auto-stop for this test

	r := NewRunner(c, in, spots, cfg, nil)
	r.Start()
	if !waitUntil(t, func() bool { return len(in.taps) >= 1 }, time.Second) {
		t.Fatal("runner did not tap the detected prompt")
	}
	// Give it a few more polls: it must not fire repeatedly for one prompt.
	time.Sleep(120 * time.Millisecond)
	r.Stop()
	if len(in.taps) != 1 {
		t.Fatalf("taps = %d (%v), want exactly 1", len(in.taps), in.taps)
	}
}
