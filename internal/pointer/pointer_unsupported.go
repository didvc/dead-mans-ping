//go:build !linux && !windows

package pointer

import (
	"fmt"
	"runtime"
)

// New reports that cursor reading is unsupported on this OS.
func New() (Reader, error) {
	return nil, fmt.Errorf("mouse position reading is not supported on %s", runtime.GOOS)
}
