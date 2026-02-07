package mirage

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/protocol/session"
	logs "github.com/danmuck/smplog"
)

// Mirage session endpoint configuration.
type ServiceConfig struct {
	ListenAddr             string
	RequireIdentityBinding bool
	Session                session.Config
}

// Mirage service defaults for session endpoint configuration.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		ListenAddr:             ":9000",
		RequireIdentityBinding: true,
		Session:                session.DefaultConfig(),
	}
}

// Mirage observed state for one connected Ghost identity.
type RegisteredGhost struct {
	GhostID      string
	RemoteAddr   string
	SeedList     []session.SeedInfo
	RegisteredAt time.Time
	LastEventAt  time.Time
	EventCount   uint64
	Connected    bool
}

// Mirage internal state with mutable registration metadata and ack idempotency map.
type registeredGhostState struct {
	meta       RegisteredGhost
	ackByEvent map[string]session.EventAck
}

// Mirage internal transport-authenticated peer identity details.
type peerAuth struct {
	PeerIdentity  string
	Authenticated bool
}

// Mirage runtime service for session and handshake contracts.
type Service struct {
	cfg ServiceConfig

	server *Server

	connsMu sync.Mutex
	conns   map[net.Conn]struct{}
}

// Mirage service constructor using default configuration.
func NewService() *Service {
	return NewServiceWithConfig(DefaultServiceConfig())
}

// Mirage service constructor using explicit configuration.
func NewServiceWithConfig(cfg ServiceConfig) *Service {
	if strings.TrimSpace(cfg.ListenAddr) == "" {
		cfg.ListenAddr = DefaultServiceConfig().ListenAddr
	}
	cfg.Session = cfg.Session.WithDefaults()
	return &Service{
		cfg:    cfg,
		server: NewServer(),
		conns:  make(map[net.Conn]struct{}),
	}
}

// Server returns the Mirage lifecycle/orchestration boundary owner.
func (s *Service) Server() *Server {
	return s.server
}

// Mirage runtime entrypoint that blocks until signal shutdown.
func (s *Service) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := s.server.Appear(MirageConfig{MirageID: "mirage.local"}); err != nil {
		return err
	}
	if err := s.server.Shimmer(); err != nil {
		return err
	}
	if err := s.server.Seed(); err != nil {
		return err
	}

	if err := s.cfg.Session.ValidateServerTransport(); err != nil {
		return err
	}

	ln, err := s.listen()
	if err != nil {
		return err
	}
	logs.Infof("mirage.Service.Run listening addr=%q", ln.Addr().String())
	return s.Serve(ctx, ln)
}

// Mirage listener builder for TCP or TLS based on transport policy.
func (s *Service) listen() (net.Listener, error) {
	if !s.cfg.Session.TLS.Enabled {
		return net.Listen("tcp", s.cfg.ListenAddr)
	}
	tlsCfg, err := s.serverTLSConfig()
	if err != nil {
		return nil, err
	}
	return tls.Listen("tcp", s.cfg.ListenAddr, tlsCfg)
}

// Mirage accept loop for Ghost sessions on an existing listener.
func (s *Service) Serve(ctx context.Context, ln net.Listener) error {
	if err := s.cfg.Session.ValidateServerTransport(); err != nil {
		return err
	}
	defer ln.Close()
	go func() {
		<-ctx.Done()
		s.closeAllConns()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		s.trackConn(conn)
		go s.handleConn(conn)
	}
}

// Mirage snapshot of observed Ghost registration state.
func (s *Service) SnapshotRegisteredGhosts() []RegisteredGhost {
	return s.server.SnapshotRegisteredGhosts()
}

// Mirage connection handler for registration and event ingestion.
func (s *Service) handleConn(conn net.Conn) {
	defer conn.Close()
	defer s.untrackConn(conn)
	reader := bufio.NewReader(conn)

	auth, err := s.authenticateConn(conn)
	if err != nil {
		logs.Warnf("mirage.handleConn transport auth err=%v", err)
		return
	}

	reg, ack := s.handleRegistration(conn, reader, auth)
	if ack.Status != session.AckStatusAccepted {
		_ = session.WriteRegistrationAck(conn, ack)
		return
	}
	if err := session.WriteRegistrationAck(conn, ack); err != nil {
		logs.Errf("mirage.handleConn write registration ack err=%v", err)
		return
	}
	logs.Infof("mirage.handleConn registered ghost_id=%q peer=%q", reg.GhostID, conn.RemoteAddr().String())
	defer s.server.MarkGhostDisconnected(reg.GhostID)

	if err := conn.SetDeadline(time.Time{}); err != nil {
		logs.Warnf("mirage.handleConn clear deadline err=%v", err)
	}

	for {
		_ = conn.SetReadDeadline(time.Now().Add(s.cfg.Session.ReadTimeout))
		fr, err := session.ReadFrame(reader, frame.DefaultLimits())
		if err != nil {
			return
		}
		if fr.Header.MessageType != schema.MsgEvent {
			logs.Warnf(
				"mirage.handleConn unexpected message_type=%d ghost_id=%q",
				fr.Header.MessageType,
				reg.GhostID,
			)
			return
		}

		event, err := session.DecodeEventFrame(fr)
		if err != nil {
			logs.Warnf("mirage.handleConn decode event err=%v", err)
			return
		}
		if report, matched, err := s.server.ObserveEvent(event); err != nil {
			logs.Warnf("mirage.handleConn observe event err=%v", err)
		} else if matched {
			logs.Infof(
				"mirage.handleConn report intent_id=%q phase=%q completion_state=%q command_id=%q event_id=%q",
				report.IntentID,
				report.Phase,
				report.CompletionState,
				report.CommandID,
				report.EventID,
			)
		}
		ack := s.server.AcceptEvent(reg.GhostID, event)
		ackPayload, err := session.EncodeEventAckFrame(fr.Header.MessageID, ack)
		if err != nil {
			logs.Warnf("mirage.handleConn encode event.ack err=%v", err)
			return
		}
		_ = conn.SetWriteDeadline(time.Now().Add(s.cfg.Session.WriteTimeout))
		if _, err := conn.Write(ackPayload); err != nil {
			logs.Warnf("mirage.handleConn write event.ack err=%v", err)
			return
		}
	}
}

// Mirage registration handler for one seed.register handshake payload.
func (s *Service) handleRegistration(
	conn net.Conn,
	reader *bufio.Reader,
	auth peerAuth,
) (session.Registration, session.RegistrationAck) {
	_ = conn.SetDeadline(time.Now().Add(s.cfg.Session.HandshakeTimeout))
	now := uint64(time.Now().UnixMilli())

	reg, err := session.ReadRegistration(reader)
	if err != nil {
		logs.Warnf("mirage.handleRegistration read err=%v", err)
		return session.Registration{}, session.RegistrationAck{
			Status:      session.AckStatusRejected,
			Code:        1001,
			Message:     "invalid registration payload",
			GhostID:     "unknown",
			TimestampMS: now,
		}
	}

	if s.cfg.RequireIdentityBinding {
		if auth.Authenticated {
			if auth.PeerIdentity != reg.GhostID {
				logs.Warnf(
					"mirage.handleRegistration tls identity mismatch ghost_id=%q peer_identity=%q",
					reg.GhostID,
					auth.PeerIdentity,
				)
				return reg, session.RegistrationAck{
					Status:      session.AckStatusRejected,
					Code:        1002,
					Message:     "identity binding failure",
					GhostID:     reg.GhostID,
					TimestampMS: now,
				}
			}
			if peer := strings.TrimSpace(reg.PeerIdentity); peer != "" && peer != auth.PeerIdentity {
				logs.Warnf(
					"mirage.handleRegistration declared peer mismatch ghost_id=%q declared_peer=%q tls_peer=%q",
					reg.GhostID,
					peer,
					auth.PeerIdentity,
				)
				return reg, session.RegistrationAck{
					Status:      session.AckStatusRejected,
					Code:        1003,
					Message:     "declared peer mismatch",
					GhostID:     reg.GhostID,
					TimestampMS: now,
				}
			}
		} else if reg.PeerIdentity != reg.GhostID {
			logs.Warnf(
				"mirage.handleRegistration identity bind mismatch ghost_id=%q peer_identity=%q",
				reg.GhostID,
				reg.PeerIdentity,
			)
			return reg, session.RegistrationAck{
				Status:      session.AckStatusRejected,
				Code:        1002,
				Message:     "identity binding failure",
				GhostID:     reg.GhostID,
				TimestampMS: now,
			}
		}
	}

	return reg, s.server.UpsertRegistration(conn.RemoteAddr().String(), reg)
}

// Mirage transport-auth helper enforcing TLS/mTLS and extracting peer identity.
func (s *Service) authenticateConn(conn net.Conn) (peerAuth, error) {
	mode := session.NormalizeSecurityMode(s.cfg.Session.SecurityMode)
	if !s.cfg.Session.TLS.Enabled {
		if mode == session.SecurityModeProduction {
			return peerAuth{}, session.ErrTLSRequired
		}
		return peerAuth{}, nil
	}

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return peerAuth{}, fmt.Errorf("mirage: expected tls connection")
	}
	_ = tlsConn.SetDeadline(time.Now().Add(s.cfg.Session.HandshakeTimeout))
	if err := tlsConn.Handshake(); err != nil {
		return peerAuth{}, err
	}
	state := tlsConn.ConnectionState()

	needPeer := s.cfg.Session.TLS.Mutual || mode == session.SecurityModeProduction
	if !needPeer && len(state.PeerCertificates) == 0 {
		return peerAuth{}, nil
	}
	if len(state.PeerCertificates) == 0 {
		return peerAuth{}, session.ErrMTLSRequired
	}
	peerID := peerIdentityFromCert(state.PeerCertificates[0])
	if peerID == "" {
		return peerAuth{}, fmt.Errorf("mirage: empty peer identity from certificate")
	}
	return peerAuth{PeerIdentity: peerID, Authenticated: true}, nil
}

// Mirage certificate identity extractor using CN/URI/DNS preference order.
func peerIdentityFromCert(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	if v := strings.TrimSpace(cert.Subject.CommonName); v != "" {
		return v
	}
	if len(cert.URIs) > 0 {
		if v := strings.TrimSpace(cert.URIs[0].String()); v != "" {
			return v
		}
	}
	if len(cert.DNSNames) > 0 {
		if v := strings.TrimSpace(cert.DNSNames[0]); v != "" {
			return v
		}
	}
	return ""
}

// Mirage TLS server-config builder for listener transport enforcement.
func (s *Service) serverTLSConfig() (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(s.cfg.Session.TLS.CertFile, s.cfg.Session.TLS.KeyFile)
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.NoClientCert,
	}

	mode := session.NormalizeSecurityMode(s.cfg.Session.SecurityMode)
	if s.cfg.Session.TLS.Mutual || mode == session.SecurityModeProduction {
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
		caPEM, err := os.ReadFile(s.cfg.Session.TLS.CAFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(caPEM); !ok {
			return nil, fmt.Errorf("mirage: parse tls ca bundle: %s", s.cfg.Session.TLS.CAFile)
		}
		cfg.ClientCAs = pool
	}
	return cfg, nil
}

// Mirage helper that returns a defensive copy of registered seed descriptors.
func copySeedList(in []session.SeedInfo) []session.SeedInfo {
	if len(in) == 0 {
		return []session.SeedInfo{}
	}
	out := make([]session.SeedInfo, len(in))
	copy(out, in)
	return out
}

// Mirage connection-tracking add operation for coordinated shutdown.
func (s *Service) trackConn(conn net.Conn) {
	s.connsMu.Lock()
	defer s.connsMu.Unlock()
	s.conns[conn] = struct{}{}
}

// Mirage connection-tracking remove operation after connection teardown.
func (s *Service) untrackConn(conn net.Conn) {
	s.connsMu.Lock()
	defer s.connsMu.Unlock()
	delete(s.conns, conn)
}

// Mirage shutdown helper that closes and drains tracked active connections.
func (s *Service) closeAllConns() {
	s.connsMu.Lock()
	defer s.connsMu.Unlock()
	for conn := range s.conns {
		_ = conn.Close()
		delete(s.conns, conn)
	}
}
