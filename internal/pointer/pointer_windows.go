//go:build windows

package pointer

import (
	"fmt"
	"syscall"
	"unsafe"
)

// winReader reads the cursor position via user32!GetCursorPos. This uses only
// the standard library syscall interface, so no cgo or third-party code is
// pulled into the Windows build.
type winReader struct{}

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procGetCursorPos = user32.NewProc("GetCursorPos")
)

// New verifies GetCursorPos is available.
func New() (Reader, error) {
	if err := procGetCursorPos.Find(); err != nil {
		return nil, fmt.Errorf("locate user32!GetCursorPos: %w", err)
	}
	return winReader{}, nil
}

// winPoint mirrors the Win32 POINT struct (two 32-bit signed integers).
type winPoint struct {
	X, Y int32
}

func (winReader) Position() (Point, error) {
	var p winPoint
	r1, _, err := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	if r1 == 0 {
		return Point{}, fmt.Errorf("GetCursorPos failed: %w", err)
	}
	return Point{X: int(p.X), Y: int(p.Y)}, nil
}

func (winReader) Close() error { return nil }
