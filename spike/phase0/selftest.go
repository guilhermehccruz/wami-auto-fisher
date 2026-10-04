package main

import (
	"flag"
	"fmt"
	"time"
)

// selftestCmd is a read-only pointer tracker. It cannot move the pointer itself
// (XTEST synthetic motion is ignored by the Wayland compositor here), and
// exact-pixel targets are impractical, so it just prints the live pointer in
// target-local coordinates. Use it to confirm the values track the mouse and
// stay inside the target monitor; the real alignment check is calibrate+capture.
func selftestCmd(args []string) {
	fs := flag.NewFlagSet("selftest", flag.ExitOnError)
	seconds := fs.Int("seconds", 8, "how long to track")
	_ = fs.Parse(args)
	if _, err := openScreen(); err != nil {
		fatal(err)
	}

	w, h := screenSize()
	fmt.Printf("Tracking the pointer for %ds in target-local space (0,0)-(%d,%d).\n", *seconds, w, h)
	fmt.Println("Move the mouse across the monitor: values should follow it and stay on target.")
	fmt.Println()

	deadline := time.Now().Add(time.Duration(*seconds) * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		if time.Now().After(deadline) {
			break
		}
		x, y := mousePos()
		where := "off target"
		if x >= 0 && y >= 0 && x < w && y < h {
			where = "on target"
		}
		fmt.Printf("\r  pointer=(%4d,%4d)  %-11s ", x, y, where)
	}
	fmt.Println()
}
