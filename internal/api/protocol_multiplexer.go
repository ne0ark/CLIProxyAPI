package api

import (
	"bufio"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	muxProtocolDetectionTimeout = 5 * time.Second
	muxTemporaryAcceptDelayMin  = 5 * time.Millisecond
	muxTemporaryAcceptDelayMax  = 1 * time.Second
)

func normalizeHTTPServeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func normalizeListenerError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (s *Server) acceptMuxConnections(listener net.Listener, httpListener *muxListener) error {
	if s == nil || listener == nil {
		return net.ErrClosed
	}

	var tempDelay time.Duration
	for {
		conn, errAccept := listener.Accept()
		if errAccept != nil {
			if isTemporaryAcceptError(errAccept) {
				if tempDelay == 0 {
					tempDelay = muxTemporaryAcceptDelayMin
				} else {
					tempDelay *= 2
					if tempDelay > muxTemporaryAcceptDelayMax {
						tempDelay = muxTemporaryAcceptDelayMax
					}
				}
				log.WithError(errAccept).Warnf("temporary multiplexer accept error, retrying in %s", tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return errAccept
		}
		tempDelay = 0
		if conn == nil {
			continue
		}
		go s.routeMuxConnection(conn, httpListener)
	}
}

func (s *Server) routeMuxConnection(conn net.Conn, httpListener *muxListener) {
	if conn == nil {
		return
	}
	if err := conn.SetDeadline(time.Now().Add(muxProtocolDetectionTimeout)); err != nil && !errors.Is(err, net.ErrClosed) {
		log.WithError(err).Debug("failed to set protocol detection deadline")
	}

	tlsConn, ok := conn.(*tls.Conn)
	if ok {
		if errHandshake := tlsConn.Handshake(); errHandshake != nil {
			if errClose := conn.Close(); errClose != nil {
				log.Errorf("failed to close connection after TLS handshake error: %v", errClose)
			}
			return
		}
		proto := strings.TrimSpace(tlsConn.ConnectionState().NegotiatedProtocol)
		if proto == "h2" || proto == "http/1.1" {
			clearConnectionDeadline(tlsConn)
			if httpListener == nil {
				if errClose := conn.Close(); errClose != nil {
					log.Errorf("failed to close connection: %v", errClose)
				}
				return
			}
			if errPut := httpListener.Put(tlsConn); errPut != nil {
				if errClose := conn.Close(); errClose != nil {
					log.Errorf("failed to close connection after HTTP routing failure: %v", errClose)
				}
			}
			return
		}
	}

	reader := bufio.NewReader(conn)
	prefix, errPeek := reader.Peek(1)
	if errPeek != nil {
		if errClose := conn.Close(); errClose != nil {
			log.Errorf("failed to close connection after protocol peek failure: %v", errClose)
		}
		return
	}

	if isRedisRESPPrefix(prefix[0]) {
		clearConnectionDeadline(conn)
		if !s.managementRoutesEnabled.Load() {
			if errClose := conn.Close(); errClose != nil {
				log.Errorf("failed to close redis connection while management is disabled: %v", errClose)
			}
			return
		}
		s.handleRedisConnection(conn, reader)
		return
	}

	clearConnectionDeadline(conn)
	if httpListener == nil {
		if errClose := conn.Close(); errClose != nil {
			log.Errorf("failed to close connection without HTTP listener: %v", errClose)
		}
		return
	}

	if errPut := httpListener.Put(&bufferedConn{Conn: conn, reader: reader}); errPut != nil {
		if errClose := conn.Close(); errClose != nil {
			log.Errorf("failed to close connection after HTTP routing failure: %v", errClose)
		}
	}
}

func clearConnectionDeadline(conn net.Conn) {
	if conn == nil {
		return
	}
	if err := conn.SetDeadline(time.Time{}); err != nil && !errors.Is(err, net.ErrClosed) {
		log.WithError(err).Debug("failed to clear protocol detection deadline")
	}
}

func isTemporaryAcceptError(err error) bool {
	type temporaryError interface {
		Temporary() bool
	}
	var tempErr temporaryError
	if errors.As(err, &tempErr) {
		return tempErr.Temporary()
	}
	return false
}
