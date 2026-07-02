//go:build linux

package pointer

import (
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// x11Reader queries the pointer position from the X server. It works on X11
// sessions (and XWayland-hosted windows). Native Wayland compositors do not
// expose a global pointer position, which is a deliberate privacy design of
// Wayland; there the position cannot be read without compositor-specific
// protocols.
type x11Reader struct {
	conn *xgb.Conn
	root xproto.Window
}

// New connects to the X server referenced by $DISPLAY.
func New() (Reader, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("connect to X server (is DISPLAY set and is this an X11 session?): %w", err)
	}
	root := xproto.Setup(conn).DefaultScreen(conn).Root
	return &x11Reader{conn: conn, root: root}, nil
}

func (r *x11Reader) Position() (Point, error) {
	reply, err := xproto.QueryPointer(r.conn, r.root).Reply()
	if err != nil {
		return Point{}, fmt.Errorf("query pointer: %w", err)
	}
	return Point{X: int(reply.RootX), Y: int(reply.RootY)}, nil
}

func (r *x11Reader) Close() error {
	r.conn.Close()
	return nil
}
