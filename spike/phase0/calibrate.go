package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// calibrateCmd walks the operator through the prompt spots. For each key it
// runs a countdown, then reads the pointer — while the operator is still in the
// game with the pointer over the prompt — and derives an ROI box around it.
//
// The countdown (rather than "press Enter to capture") is deliberate: reading
// the pointer after Alt-Tabbing back to the terminal would capture the
// terminal's position instead of the game's.
func calibrateCmd(args []string) {
	fs := flag.NewFlagSet("calibrate", flag.ExitOnError)
	out := fs.String("out", "spike/phase0/spots.json", "output spots JSON")
	w := fs.Int("w", defaultROIW, "ROI width in pixels")
	h := fs.Int("h", defaultROIH, "ROI height in pixels")
	keysCSV := fs.String("keys", "a,w,d,s", "keys, in the order you will hover them")
	delay := fs.Int("delay", 4, "seconds to hover over each prompt before it is captured")
	display := fs.Int("display", -1, "capture display index (-1 = primary)")
	_ = fs.Parse(args)
	displayIndex = *display

	if *w <= 0 || *h <= 0 {
		fatal(fmt.Errorf("-w and -h must be positive"))
	}
	if *delay < 1 {
		fatal(fmt.Errorf("-delay must be >= 1"))
	}
	if _, err := openScreen(); err != nil {
		fatal(err)
	}

	var keys []string
	for _, k := range strings.Split(*keysCSV, ",") {
		if k = strings.TrimSpace(k); k != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		fatal(fmt.Errorf("no keys given"))
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Calibrating %d spots on display %d, ROI %dx%d.\n", len(keys), targetIndex(), *w, *h)
	fmt.Println("For each key: Alt-Tab to the game, hover the pointer over the CENTER of")
	fmt.Println("that prompt box, and wait. The position is read while you are still in")
	fmt.Println("the game, so it cannot pick up the terminal instead.")
	fmt.Println()

	spots := make([]Spot, 0, len(keys))
	for _, key := range keys {
		id := "s_" + key
		fmt.Printf("  %-4s (key %q): hovering over the prompt...\n", id, key)
		for s := *delay; s > 0; s-- {
			fmt.Printf("\r    capturing in %d...", s)
			time.Sleep(time.Second)
		}
		cx, cy := mousePos()
		spots = append(spots, Spot{
			ID:  id,
			Key: key,
			ROI: Rect{cx - *w/2, cy - *h/2, *w, *h},
		})
		fmt.Printf("\r    captured center (%d, %d) -> roi %s          \n", cx, cy, spots[len(spots)-1].ROI)
		fmt.Print("    Alt-Tab back and press Enter for the next spot... ")
		if _, err := reader.ReadString('\n'); err != nil {
			fatal(err)
		}
	}

	raw, err := json.MarshalIndent(spots, "", "  ")
	if err != nil {
		fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(*out, raw, 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("\nwrote %d spots to %s\n", len(spots), *out)
	fmt.Println("next:  go run ./spike/phase0 capture -out /tmp/wami0   # check the crops")
	fmt.Println("       go run ./spike/phase0 watch -seconds 20           # check live scores")
}
