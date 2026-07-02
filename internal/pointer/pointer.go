// Package pointer reads the absolute mouse cursor position in a
// cross-platform way. Implementations poll the OS for the current position;
// they never install global input hooks and never require elevated
// privileges, which keeps the tool safe to run as an ordinary user.
package pointer

// Point is an absolute cursor position in screen pixels.
type Point struct {
	X, Y int
}

// Reader returns the current cursor position. A Reader is created with New
// and must be closed when no longer needed. Position may be called
// repeatedly and is expected to be cheap.
type Reader interface {
	Position() (Point, error)
	Close() error
}
