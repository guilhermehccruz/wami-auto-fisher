//go:build !linux && !windows

package fisher

import (
	"errors"
	"image"
)

var errUnsupported = errors.New("capture is not implemented on this platform yet")

// Screen is a placeholder on non-Linux platforms. The Windows backend (GDI) is
// added during the Windows pass.
type Screen struct{}

func OpenScreen(display int) (*Screen, error)                 { return nil, errUnsupported }
func (s *Screen) Name() string                                { return "unsupported" }
func (s *Screen) Bounds() (image.Rectangle, error)            { return image.Rectangle{}, errUnsupported }
func (s *Screen) Grab() (*image.RGBA, error)                  { return nil, errUnsupported }
func (s *Screen) GrabUnion(spots []Spot) (*image.RGBA, error) { return nil, errUnsupported }
func (s *Screen) TapKey(key string) error                     { return errUnsupported }
func (s *Screen) ActiveTitle() string                         { return "" }
func (s *Screen) CursorPos() (int, int)                       { return 0, 0 }
func (s *Screen) DisplayCount() int                           { return 0 }
func (s *Screen) Displays() []Display                         { return nil }

// Display describes one monitor.
type Display struct {
	Index   int  `json:"index"`
	X       int  `json:"x"`
	Y       int  `json:"y"`
	W       int  `json:"w"`
	H       int  `json:"h"`
	Primary bool `json:"primary"`
}
