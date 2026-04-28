package api

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestMuxListenerCloseClosesQueuedConnections(t *testing.T) {
	listener := newMuxListener(&net.TCPAddr{}, 1)
	serverConn, clientConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()

	if err := listener.Put(serverConn); err != nil {
		t.Fatalf("listener.Put() error = %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("listener.Close() error = %v", err)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	if _, err := clientConn.Read(buf); err == nil {
		t.Fatal("expected queued connection to be closed when listener closes")
	} else if err != io.EOF {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			t.Fatalf("expected queued connection closure, got timeout: %v", err)
		}
	}
}

func TestMuxListenerAcceptDoesNotReturnBufferedConnAfterCloseSignal(t *testing.T) {
	for i := 0; i < 256; i++ {
		listener := newMuxListener(&net.TCPAddr{}, 1)
		serverConn, clientConn := net.Pipe()

		if err := listener.Put(serverConn); err != nil {
			_ = clientConn.Close()
			t.Fatalf("listener.Put() error = %v", err)
		}
		close(listener.closeCh)

		conn, err := listener.Accept()
		if conn != nil {
			_ = conn.Close()
			_ = clientConn.Close()
			t.Fatalf("Accept() returned a queued connection after close signal on iteration %d", i+1)
		}
		if !errors.Is(err, net.ErrClosed) {
			_ = clientConn.Close()
			t.Fatalf("Accept() error = %v, want %v on iteration %d", err, net.ErrClosed, i+1)
		}

		select {
		case queued := <-listener.connCh:
			if queued != nil {
				_ = queued.Close()
			}
		default:
		}
		_ = clientConn.Close()
	}
}
