// Command phase0 is the WAMI Auto Fisher Phase 0 spike.
//
// It validates the blocking assumptions in DESIGN.md §14 without a GUI:
//
//	probe    - report capture backend, screen size, active window title, spot list
//	calibrate - hover each prompt and record its ROI into spots.json
//	capture  - save a full desktop PNG plus one crop per prompt spot
//	watch    - capture in a loop and print the live white/blue score per spot
//	press    - inject one key after a countdown (verifies keys reach the game)
//
// It is read-only by default: only "press" (and "watch -fire -yes") injects input.
package main

import (
	"flag"
	"fmt"
	"os"
)

const usageText = `wami-spike - Phase 0 validation for WAMI Auto Fisher

usage:
  wami-spike probe     [-spots spots.json]
  wami-spike calibrate [-out spots.json] [-w 180] [-h 56] [-keys a,w,d,s]
  wami-spike capture   [-out DIR] [-spots spots.json]
  wami-spike watch     [-spots spots.json] [-fps 10] [-threshold 0.35] [-margin 1.25] [-fire -yes] [-seconds 0]
  wami-spike press     -key a [-delay 3] -yes
  wami-spike selftest                           # pointer tracker (target-local)
  wami-spike setup-kwin                         # KDE: authorize fast ScreenShot2 capture

commands:
  probe     report backend, screen size, displays, active window title, spots
  calibrate hover the pointer over each prompt and write spots.json
  capture   write full.png and one PNG per spot to -out (default spike/phase0/out)
  watch     live per-spot score table; -fire -yes injects the mapped key
  press     tap -key after -delay seconds (focus the game); -yes required
  selftest  read-only pointer tracker in target-local coordinates
  setup-kwin write the .desktop that authorizes KWin ScreenShot2 for this binary

flags:
  -spots PATH   JSON array of spots [{id,key,roi:{x,y,w,h}}]; default uses the
                approximate 1080p coordinates from DESIGN.md Appendix A
  -yes          acknowledge that input will be injected
`

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, usageText) }
	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "probe":
		probeCmd(os.Args[2:])
	case "calibrate":
		calibrateCmd(os.Args[2:])
	case "capture":
		captureCmd(os.Args[2:])
	case "watch":
		watchCmd(os.Args[2:])
	case "press":
		pressCmd(os.Args[2:])
	case "selftest":
		selftestCmd(os.Args[2:])
	case "setup-kwin":
		setupKwinCmd(os.Args[2:])
	case "help", "-h", "--help":
		flag.Usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		flag.Usage()
		os.Exit(2)
	}
}
