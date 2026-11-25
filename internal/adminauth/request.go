package adminauth

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"time"
)

// Admin request line exceeds RequestLimits.MaxLineBytes; the connection must be closed.
var ErrRequestTooLarge = errors.New("adminauth: admin request line too large")

// Bounds for reading one newline-delimited admin request from an accepted connection.
type RequestLimits struct {
	// Largest accepted request line, newline included.
	MaxLineBytes int
	// Longest wait for a complete request line; zero disables the deadline.
	IdleTimeout time.Duration
}

// Limits used by the Mirage and Ghost admin endpoints. The line cap leaves room for an
// execute_envelope request carrying a maximum-size frame (frame.DefaultLimits, 8 MiB payload)
// base64-encoded in JSON; idle connections are dropped after five minutes.
func DefaultRequestLimits() RequestLimits {
	return RequestLimits{MaxLineBytes: 16 * 1024 * 1024, IdleTimeout: 5 * time.Minute}
}

// Reads one request line with a bounded size and a read deadline. The deadline covers only the
// wait for and receipt of this line, never request handling. Returns ErrRequestTooLarge without
// buffering past the cap; a deadline expiry surfaces as the connection's timeout error.
func ReadRequestLine(conn net.Conn, r *bufio.Reader, limits RequestLimits) ([]byte, error) {
	if limits.IdleTimeout > 0 {
		if err := conn.SetReadDeadline(time.Now().Add(limits.IdleTimeout)); err != nil {
			return nil, err
		}
	}
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if limits.MaxLineBytes > 0 && len(line)+len(chunk) > limits.MaxLineBytes {
			return nil, fmt.Errorf("%w: limit %d bytes", ErrRequestTooLarge, limits.MaxLineBytes)
		}
		line = append(line, chunk...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			return nil, err
		}
		return line, nil
	}
}
