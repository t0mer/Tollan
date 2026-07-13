package input

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// maxDatagram bounds a single UDP read (GELF chunks and syslog fit comfortably).
const maxDatagram = 65535

// packetHandler handles one received datagram from source (remote host).
type packetHandler func(payload []byte, source string)

// connHandler handles one accepted stream connection; it should read and frame
// messages until ctx is cancelled or the peer closes.
type connHandler func(ctx context.Context, conn net.Conn, source string)

// netServer is a reusable UDP/TCP(/TLS) listener. Inputs configure the framing
// via onPacket (UDP) or onConn (stream).
type netServer struct {
	proto    Protocol
	bind     string
	tlsCfg   *tls.Config
	log      *slog.Logger
	onPacket packetHandler
	onConn   connHandler

	cancel   context.CancelFunc
	udp      net.PacketConn
	listener net.Listener
	conns    sync.WaitGroup
	loop     sync.WaitGroup
}

// start binds the listener and begins serving in the background.
func (s *netServer) start() error {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	switch s.proto {
	case UDP:
		pc, err := net.ListenPacket("udp", s.bind)
		if err != nil {
			cancel()
			return fmt.Errorf("binding udp %s: %w", s.bind, err)
		}
		s.udp = pc
		s.loop.Add(1)
		go s.readUDP(ctx)
	case TCP, TLS:
		ln, err := net.Listen("tcp", s.bind)
		if err != nil {
			cancel()
			return fmt.Errorf("binding tcp %s: %w", s.bind, err)
		}
		if s.proto == TLS && s.tlsCfg != nil {
			ln = tls.NewListener(ln, s.tlsCfg)
		}
		s.listener = ln
		s.loop.Add(1)
		go s.acceptTCP(ctx)
	default:
		cancel()
		return fmt.Errorf("unsupported protocol %q", s.proto)
	}
	return nil
}

func (s *netServer) readUDP(ctx context.Context) {
	defer s.loop.Done()
	buf := make([]byte, maxDatagram)
	for {
		n, addr, err := s.udp.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			s.log.Warn("udp read error", "error", err)
			continue
		}
		payload := make([]byte, n)
		copy(payload, buf[:n])
		s.onPacket(payload, hostOf(addr))
	}
}

func (s *netServer) acceptTCP(ctx context.Context) {
	defer s.loop.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			s.log.Warn("tcp accept error", "error", err)
			continue
		}
		s.conns.Add(1)
		go func() {
			defer s.conns.Done()
			defer conn.Close()
			s.onConn(ctx, conn, hostOf(conn.RemoteAddr()))
		}()
	}
}

// stop closes the listener, waits for the accept/read loop to exit, then waits
// (bounded by ctx) for in-flight connections to drain.
func (s *netServer) stop(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.udp != nil {
		_ = s.udp.Close()
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.loop.Wait()

	done := make(chan struct{})
	go func() { s.conns.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// listen opens a TCP listener on bind (used by HTTP-based inputs).
func listen(bind string) (net.Listener, error) {
	ln, err := net.Listen("tcp", bind)
	if err != nil {
		return nil, fmt.Errorf("binding %s: %w", bind, err)
	}
	return ln, nil
}

// hostOf extracts the host portion of a network address.
func hostOf(addr net.Addr) string {
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

// loadTLS builds a TLS config from a cert/key pair.
func loadTLS(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("loading TLS keypair: %w", err)
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
}

// nowUTC is the ingest clock; overridable in tests.
var nowUTC = func() time.Time { return time.Now().UTC() }
