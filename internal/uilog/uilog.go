// Package uilog is a tiny terminal logger shared by every goroutine (the
// activity engine, the heartbeat loop and the control server). It serializes
// writes so their output never interleaves, and it keeps a single in-place
// status line that event lines cleanly overwrite.
package uilog

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// Logger is safe for concurrent use. A nil or disabled Logger is a no-op.
type Logger struct {
	mu      sync.Mutex
	w       io.Writer
	enabled bool
	now     func() time.Time
	dirty   bool // an unterminated status line is currently on screen
}

// New returns a Logger writing to w. When enabled is false all methods no-op.
func New(w io.Writer, enabled bool) *Logger {
	return &Logger{w: w, enabled: enabled, now: time.Now}
}

// Enabled reports whether output is produced.
func (l *Logger) Enabled() bool { return l != nil && l.enabled }

// Event prints a timestamped line, first clearing any pending status line.
func (l *Logger) Event(format string, args ...any) {
	if !l.Enabled() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.w, "\r\033[K[%s] %s\n", l.now().Format("15:04:05"), fmt.Sprintf(format, args...))
	l.dirty = false
}

// Status overwrites the in-place status line (no trailing newline).
func (l *Logger) Status(text string) {
	if !l.Enabled() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.w, "\r\033[K%s", text)
	l.dirty = true
}

// Finish terminates any pending status line so the shell prompt starts clean.
func (l *Logger) Finish() {
	if !l.Enabled() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.dirty {
		fmt.Fprintln(l.w)
		l.dirty = false
	}
}
