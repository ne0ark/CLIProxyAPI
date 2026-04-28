package api

import (
	"net"
	"sync"
)

type muxListener struct {
	addr    net.Addr
	connCh  chan net.Conn
	closeCh chan struct{}
	mu      sync.RWMutex
	closed  bool
	once    sync.Once
}

func newMuxListener(addr net.Addr, buffer int) *muxListener {
	if buffer <= 0 {
		buffer = 1
	}
	return &muxListener{
		addr:    addr,
		connCh:  make(chan net.Conn, buffer),
		closeCh: make(chan struct{}),
	}
}

func (l *muxListener) Put(conn net.Conn) error {
	if conn == nil {
		return nil
	}
	if l.isClosed() {
		_ = conn.Close()
		return net.ErrClosed
	}
	select {
	case <-l.closeCh:
		_ = conn.Close()
		return net.ErrClosed
	case l.connCh <- conn:
		if l.isClosed() {
			_ = conn.Close()
			return net.ErrClosed
		}
		return nil
	}
}

func (l *muxListener) Accept() (net.Conn, error) {
	select {
	case <-l.closeCh:
		return nil, net.ErrClosed
	case conn := <-l.connCh:
		if conn == nil {
			return nil, net.ErrClosed
		}
		select {
		case <-l.closeCh:
			_ = conn.Close()
			return nil, net.ErrClosed
		default:
			return conn, nil
		}
	}
}

func (l *muxListener) Close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		l.mu.Lock()
		l.closed = true
		close(l.closeCh)
		l.mu.Unlock()
		for {
			select {
			case conn := <-l.connCh:
				if conn != nil {
					_ = conn.Close()
				}
			default:
				return
			}
		}
	})
	return nil
}

func (l *muxListener) isClosed() bool {
	if l == nil {
		return true
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.closed
}

func (l *muxListener) Addr() net.Addr {
	if l == nil {
		return &net.TCPAddr{}
	}
	if l.addr == nil {
		return &net.TCPAddr{}
	}
	return l.addr
}
