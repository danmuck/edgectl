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

type ServiceConfig struct {
	ListenAddr             string
	RequireIdentityBinding bool
	Session                session.Config
}

func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		ListenAddr:             ":9000",
		RequireIdentityBinding: true,
		Session:                session.DefaultConfig(),
	}
}

type RegisteredGhost struct {
	GhostID      string
	RemoteAddr   string
	SeedList     []session.SeedInfo
	RegisteredAt time.Time
	LastEventAt  time.Time
	EventCount   uint64
	Connected    bool
}

type registeredGhostState struct {
	meta       RegisteredGhost
	ackByEvent map[string]session.EventAck
}

type peerAuth struct {
	PeerIdentity  string
	Authenticated bool
}

// Service is a minimal Mirage runtime for session/handshake contracts.
type Service struct {
	cfg ServiceConfig

	mu       sync.RWMutex
	registry map[string]*registeredGhostState
	conns    map[net.Conn]struct{}
}

func NewService() *Service {
	return NewServiceWithConfig(DefaultServiceConfig())
}

func NewServiceWithConfig(cfg ServiceConfig) *Service {
	if strings.TrimSpace(cfg.ListenAddr) == "" {
		cfg.ListenAddr = DefaultServiceConfig().ListenAddr
	}
	cfg.Session = cfg.Session.WithDefaults()
	return &Service{
		cfg:      cfg,
		registry: make(map[string]*registeredGhostState),
		conns:    make(map[net.Conn]struct{}),
	}
}

// Run starts the Mirage session endpoint and blocks until signal shutdown.
func (s *Service) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

// Serve accepts Ghost sessions on an existing listener.
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

func (s *Service) SnapshotRegisteredGhosts() []RegisteredGhost {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RegisteredGhost, 0, len(s.registry))
	for _, state := range s.registry {
		meta := state.meta
		meta.SeedList = copySeedList(meta.SeedList)
		out = append(out, meta)
	}
	return out
}

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
	defer s.unregisterGhost(reg.GhostID)

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
		ack := s.acceptEvent(reg.GhostID, event)
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

	registered := RegisteredGhost{
		GhostID:    reg.GhostID,
		RemoteAddr: conn.RemoteAddr().String(),
		SeedList:   copySeedList(reg.SeedList),
		Connected:  true,
	}

	s.mu.Lock()
	state, ok := s.registry[reg.GhostID]
	if !ok {
		state = &registeredGhostState{
			ackByEvent: make(map[string]session.EventAck),
		}
		s.registry[reg.GhostID] = state
	}
	if state.meta.RegisteredAt.IsZero() {
		state.meta.RegisteredAt = time.Now()
	}
	registered.RegisteredAt = state.meta.RegisteredAt
	registered.LastEventAt = state.meta.LastEventAt
	registered.EventCount = state.meta.EventCount
	state.meta = registered
	s.mu.Unlock()

	return reg, session.RegistrationAck{
		Status:      session.AckStatusAccepted,
		Code:        0,
		Message:     "registered",
		GhostID:     reg.GhostID,
		TimestampMS: now,
	}
}

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

func (s *Service) acceptEvent(ghostID string, event session.Event) session.EventAck {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.registry[ghostID]
	if !ok {
		state = &registeredGhostState{
			meta: RegisteredGhost{
				GhostID:      ghostID,
				RegisteredAt: time.Now(),
			},
			ackByEvent: make(map[string]session.EventAck),
		}
		s.registry[ghostID] = state
	}
	if ack, ok := state.ackByEvent[event.EventID]; ok {
		return ack
	}
	ack := session.EventAck{
		EventID:     event.EventID,
		CommandID:   event.CommandID,
		GhostID:     ghostID,
		AckStatus:   session.AckStatusAccepted,
		AckCode:     0,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}
	state.ackByEvent[event.EventID] = ack
	state.meta.LastEventAt = time.Now()
	state.meta.EventCount++
	return ack
}

func copySeedList(in []session.SeedInfo) []session.SeedInfo {
	if len(in) == 0 {
		return []session.SeedInfo{}
	}
	out := make([]session.SeedInfo, len(in))
	copy(out, in)
	return out
}

func (s *Service) unregisterGhost(ghostID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.registry[ghostID]
	if !ok {
		return
	}
	state.meta.Connected = false
	state.meta.RemoteAddr = ""
}

func (s *Service) trackConn(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns[conn] = struct{}{}
}

func (s *Service) untrackConn(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, conn)
}

func (s *Service) closeAllConns() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for conn := range s.conns {
		_ = conn.Close()
		delete(s.conns, conn)
	}
}
