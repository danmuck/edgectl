package adminauth

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"testing"
	"time"
)

// Serves ReadRequestLine on one end of a pipe and returns the client end plus the result channel.
func readOverPipe(t *testing.T, limits RequestLimits) (net.Conn, <-chan error, <-chan []byte) {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })
	errc, linec := make(chan error, 1), make(chan []byte, 1)
	go func() {
		line, err := ReadRequestLine(server, bufio.NewReaderSize(server, 16), limits)
		linec <- line
		errc <- err
	}()
	return client, errc, linec
}

// A line within the cap is returned whole even when it spans several buffer fills.
func TestReadRequestLineReturnsLineWithinCap(t *testing.T) {
	client, errc, linec := readOverPipe(t, RequestLimits{MaxLineBytes: 256, IdleTimeout: time.Second})
	want := append(bytes.Repeat([]byte("a"), 100), '\n')
	go client.Write(want)
	if line := <-linec; !bytes.Equal(line, want) {
		t.Fatalf("unexpected line len=%d", len(line))
	}
	if err := <-errc; err != nil {
		t.Fatalf("read: %v", err)
	}
}

// A line longer than the cap is refused without waiting for its newline.
func TestReadRequestLineRejectsOversizeLine(t *testing.T) {
	client, errc, _ := readOverPipe(t, RequestLimits{MaxLineBytes: 64, IdleTimeout: time.Second})
	go client.Write(bytes.Repeat([]byte("a"), 200))
	if err := <-errc; !errors.Is(err, ErrRequestTooLarge) {
		t.Fatalf("expected ErrRequestTooLarge, got %v", err)
	}
}

// A connection that sends nothing is released when the idle deadline passes.
func TestReadRequestLineTimesOutWhenIdle(t *testing.T) {
	_, errc, _ := readOverPipe(t, RequestLimits{MaxLineBytes: 64, IdleTimeout: 50 * time.Millisecond})
	select {
	case err := <-errc:
		var ne net.Error
		if !errors.As(err, &ne) || !ne.Timeout() {
			t.Fatalf("expected timeout, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("idle read was not released")
	}
}
