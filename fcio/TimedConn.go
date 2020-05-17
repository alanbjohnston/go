package fcio

import (
	"errors"
	"io"
	"net"
	"time"
)

// TimedConn provides ReadCloser interface for a net conn, with timeouts on reads
type TimedConn struct {
	io.Reader
	io.Closer
	conn     net.Conn
	duration time.Duration
}

// NewTimedConn creates an instance of a TimedConn
func NewTimedConn(conn net.Conn, duration time.Duration) (*TimedConn, error) {
	if conn == nil {
		return nil, errors.New("invalid conn (net.Conn) parameter")
	}

	return &TimedConn{
		conn:     conn,
		duration: duration,
	}, nil
}

func (tc TimedConn) Read(p []byte) (int, error) {
	if tc.duration > 0 {
		tc.conn.SetReadDeadline(time.Now().Add(tc.duration))
	}
	n, err := tc.conn.Read(p)
	if tc.duration > 0 {
		tc.conn.SetReadDeadline(time.Time{})
		if isTimeout(err) {
			// simplify calling code use EOF for socket timeout
			err = io.EOF
		}
	}
	return n, err
}

// Close the connection
func (tc TimedConn) Close() error {
	return tc.conn.Close()
}

func isTimeout(err error) bool {
	neterr, ok := err.(net.Error)
	return ok && neterr.Timeout()
}
