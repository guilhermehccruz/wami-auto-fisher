//go:build !linux

package fisher

import "errors"

// WriteLauncher is a no-op outside Linux (the launcher entry is a freedesktop
// concept; Windows uses the .exe metadata directly).
func WriteLauncher(icon []byte) (string, error) {
	return "", errors.New("launcher install is Linux-only")
}
