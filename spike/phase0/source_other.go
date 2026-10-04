//go:build !linux

package main

import (
	"errors"
	"image"
)

// screen is a placeholder on platforms the Phase 0 spike does not cover yet.
// The Windows backend is added during the Windows Phase 0 pass.
type screen struct{ kwinReason string }

var errUnsupported = errors.New("phase0 spike currently supports Linux only; Windows backend pending")

// displayIndex selects which monitor to capture; -1 means the primary.
var displayIndex = -1

func targetIndex() int { return 0 }

func openScreen() (*screen, error)                            { return nil, errUnsupported }
func (s *screen) Name() string                                { return "unsupported" }
func (s *screen) Bounds() (image.Rectangle, error)            { return image.Rectangle{}, errUnsupported }
func (s *screen) Grab() (*image.RGBA, error)                  { return nil, errUnsupported }
func (s *screen) GrabRect(r Rect) (*image.RGBA, error)        { return nil, errUnsupported }
func (s *screen) GrabUnion(spots []Spot) (*image.RGBA, error) { return nil, errUnsupported }
func setupKwinCmd(args []string)                              { fatal(errUnsupported) }
func crop(src *image.RGBA, r image.Rectangle) *image.RGBA     { return nil }
func tapKey(key string) error                                 { return errUnsupported }
func mousePos() (int, int)                                    { return 0, 0 }
func moveMouse(x, y int) error                                { return errUnsupported }
func captureOffset() (int, int)                               { return 0, 0 }
func capturePath() string                                     { return "unsupported" }
func unionOrigin() (int, int)                                 { return 0, 0 }
func displayBounds(i int) (Rect, bool)                        { return Rect{}, false }
func activeTitle() string                                     { return "" }
func screenSize() (int, int)                                  { return 0, 0 }
func displayCount() int                                       { return 0 }
