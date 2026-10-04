package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"
)

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func writePNG(path string, img *image.RGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func probeCmd(args []string) {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	spotsPath := fs.String("spots", "", "spots JSON (default: built-in approximations)")
	display := fs.Int("display", -1, "capture display index (-1 = primary)")
	_ = fs.Parse(args)
	displayIndex = *display

	sc, err := openScreen()
	if err != nil {
		fatal(err)
	}
	w, h := screenSize()
	ux, uy := unionOrigin()
	fmt.Printf("capture backend : %s\n", sc.Name())
	fmt.Printf("target size     : %dx%d (coordinates are target-local, origin 0,0)\n", w, h)
	fmt.Printf("displays        : %d\n", displayCount())
	for i := 0; i < displayCount(); i++ {
		r, ok := displayBounds(i)
		if !ok {
			continue
		}
		mark := ""
		if i == targetIndex() {
			mark += "  <- target"
		}
		if image.Pt(0, 0).In(image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H)) {
			mark += " (primary)"
		}
		fmt.Printf("  [%d] %dx%d at (%d,%d)%s\n", i, r.W, r.H, r.X, r.Y, mark)
	}
	fmt.Printf("virtual origin  : (%d,%d)  [Xinerama offset applied to the mouse]\n", ux, uy)
	ox, oy := captureOffset()
	fmt.Printf("capture offset  : (%d,%d)  [%s]\n", ox, oy, capturePath())
	if b, err := sc.Bounds(); err == nil {
		fmt.Printf("target bounds   : %v\n", b)
	}
	fmt.Printf("active title    : %q\n", activeTitle())
	if sc.kwinReason != "" {
		fmt.Printf("kwin fallback   : %s\n", sc.kwinReason)
	}
	mx, my := mousePos()
	fmt.Printf("pointer         : (%d,%d)  [target-local; move it around to sanity-check]\n", mx, my)

	spots, err := loadSpots(*spotsPath)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("spots (%d)      : %s\n", len(spots), spotIDs(spots))
}

func captureCmd(args []string) {
	fs := flag.NewFlagSet("capture", flag.ExitOnError)
	out := fs.String("out", "spike/phase0/out", "output directory")
	spotsPath := fs.String("spots", "", "spots JSON (default: built-in approximations)")
	display := fs.Int("display", -1, "capture display index (-1 = primary)")
	_ = fs.Parse(args)
	displayIndex = *display

	sc, err := openScreen()
	if err != nil {
		fatal(err)
	}
	spots, err := loadSpots(*spotsPath)
	if err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatal(err)
	}

	full, err := sc.Grab()
	if err != nil {
		fatal(err)
	}
	fullPath := filepath.Join(*out, "full.png")
	if err := writePNG(fullPath, full); err != nil {
		fatal(err)
	}
	fmt.Printf("full desktop %dx%d -> %s\n", full.Bounds().Dx(), full.Bounds().Dy(), fullPath)

	for _, s := range spots {
		img := crop(full, s.ROI.Bounds())
		white, blue := classifyRoi(full, s.ROI)
		path := filepath.Join(*out, s.ID+".png")
		if err := writePNG(path, img); err != nil {
			fatal(err)
		}
		fmt.Printf("  %-4s key=%-2s roi=%-16s white=%.3f blue=%.3f -> %s\n",
			s.ID, s.Key, s.ROI, white, blue, path)
	}
	fmt.Println("open the PNGs and confirm each ROI frames a prompt box when one is up")
}

func watchCmd(args []string) {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	spotsPath := fs.String("spots", "", "spots JSON (default: built-in approximations)")
	fps := fs.Int("fps", 10, "captures per second")
	threshold := fs.Float64("threshold", 0.35, "whiteFrac needed to call a spot present")
	clear := fs.Float64("clear", 0.15, "whiteFrac below which a spot is re-armed")
	margin := fs.Float64("margin", 1.25, "top score must beat the runner-up by this factor")
	refractoryMs := fs.Int("refractory", 100, "minimum ms between two fires of the same spot")
	retryMs := fs.Int("retry", 250, "re-tap the same spot after this many ms if it persists")
	fire := fs.Bool("fire", false, "inject the mapped key when a prompt is detected")
	yes := fs.Bool("yes", false, "acknowledge injection (required with -fire)")
	seconds := fs.Int("seconds", 0, "stop after N seconds (0 = until Ctrl-C)")
	display := fs.Int("display", -1, "capture display index (-1 = primary)")
	_ = fs.Parse(args)
	displayIndex = *display

	if *fire && !*yes {
		fatal(fmt.Errorf("-fire injects real input; add -yes to confirm"))
	}
	if *fps <= 0 {
		fatal(fmt.Errorf("-fps must be > 0"))
	}

	sc, err := openScreen()
	if err != nil {
		fatal(err)
	}
	spots, err := loadSpots(*spotsPath)
	if err != nil {
		fatal(err)
	}

	fmt.Printf("watch: backend=%s spots=%s fps=%d threshold=%.2f margin=%.2f fire=%v\n",
		sc.Name(), spotIDs(spots), *fps, *threshold, *margin, *fire)
	if *fire {
		fmt.Println("press Ctrl-C to stop. focus the game window now.")
	} else {
		fmt.Println("read-only mode. reconstruct with -fire -yes to inject. Ctrl-C to stop.")
	}
	time.Sleep(300 * time.Millisecond)

	armed := map[string]bool{}
	lastFire := map[string]time.Time{}
	for _, s := range spots {
		armed[s.ID] = true
	}
	retry := time.Duration(*retryMs) * time.Millisecond
	refractory := time.Duration(*refractoryMs) * time.Millisecond

	interval := time.Second / time.Duration(*fps)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	deadline := time.Time{}
	if *seconds > 0 {
		deadline = time.Now().Add(time.Duration(*seconds) * time.Second)
	}
	start := time.Now()
	clearOn := isTerminal()

	for range ticker.C {
		if !deadline.IsZero() && time.Now().After(deadline) {
			fmt.Println("stopped")
			return
		}
		frame, err := sc.GrabUnion(spots)
		if err != nil {
			fmt.Fprintln(os.Stderr, "capture error:", err)
			continue
		}
		scores := scoreAll(frame, spots)
		idx, ok, ambiguous := best(scores, *threshold, *margin)

		var fired string
		if *fire {
			now := time.Now()
			for i := range scores {
				s := scores[i].Spot
				if ok && i == idx {
					if armed[s.ID] && now.Sub(lastFire[s.ID]) >= refractory {
						if err := tapKey(s.Key); err != nil {
							fmt.Fprintln(os.Stderr, "inject error:", err)
							continue
						}
						armed[s.ID] = false
						lastFire[s.ID] = now
						fired = s.Key
					} else if !armed[s.ID] && now.Sub(lastFire[s.ID]) >= retry {
						_ = tapKey(s.Key)
						lastFire[s.ID] = now
						fired = s.Key + "(retry)"
					}
				} else if scores[i].WhiteFrac < *clear {
					armed[s.ID] = true
				}
			}
		}

		line := fmt.Sprintf("%s  %s", fmtClock(time.Since(start)), renderScores(scores, idx, ok, ambiguous))
		if fired != "" {
			line += "   FIRED " + fired
		}
		if clearOn {
			fmt.Print("\033[H\033[J", line, "\n")
		} else {
			fmt.Println(line)
		}
	}
}

func pressCmd(args []string) {
	fs := flag.NewFlagSet("press", flag.ExitOnError)
	key := fs.String("key", "a", "key to tap (e.g. a, w, s, d)")
	delay := fs.Int("delay", 3, "seconds to wait before tapping")
	yes := fs.Bool("yes", false, "acknowledge injection (required)")
	_ = fs.Parse(args)

	if !*yes {
		fatal(fmt.Errorf("-key injects a real key; add -yes to confirm"))
	}
	sc, err := openScreen()
	if err != nil {
		fatal(err)
	}
	fmt.Printf("backend=%s; tapping %q in %ds - focus the game window now\n", sc.Name(), *key, *delay)
	time.Sleep(time.Duration(*delay) * time.Second)
	if err := tapKey(*key); err != nil {
		fatal(err)
	}
	fmt.Printf("tapped %q\n", *key)
}

func renderScores(m []Metrics, idx int, ok, ambiguous bool) string {
	out := ""
	for i, s := range m {
		mark := " "
		if i == idx {
			mark = "*"
		}
		out += fmt.Sprintf("[%s %.2f/%.2f%s] ", s.Spot.ID, s.WhiteFrac, s.BlueFrac, mark)
	}
	switch {
	case ok:
		out += "-> " + m[idx].Spot.ID + " key=" + m[idx].Spot.Key
	case ambiguous:
		out += "-> ambiguous (ignored)"
	default:
		out += "-> none"
	}
	return out
}

func fmtClock(d time.Duration) string {
	t := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d:%02d", t/3600, (t%3600)/60, t%60)
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
