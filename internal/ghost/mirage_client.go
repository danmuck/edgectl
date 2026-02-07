package ghost

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/seeds"
	logs "github.com/danmuck/smplog"
)

var (
	ErrMirageAddressRequired = errors.New("ghost: mirage address required")
	ErrGhostIDRequired       = errors.New("ghost: ghost_id required")
	ErrRegistrationRejected  = errors.New("ghost: registration rejected")
	ErrAckRejected           = errors.New("ghost: event.ack rejected")
	ErrAckTimeout            = errors.New("ghost: event.ack timeout")
	ErrSessionClosed         = errors.New("ghost: mirage session closed")
)

type MirageClientConfig struct {
	Address            string
	GhostID            string
	PeerIdentity       string
	SeedList           []session.SeedInfo
	Session            session.Config
	MaxConnectAttempts int
}

func DefaultMirageClientConfig() MirageClientConfig {
	return MirageClientConfig{
		Session: session.DefaultConfig(),
	}
}

type MirageClient struct {
	cfg MirageClientConfig
	rng *rand.Rand
}

func NewMirageClient(cfg MirageClientConfig) (*MirageClient, error) {
	if strings.TrimSpace(cfg.Address) == "" {
		return nil, ErrMirageAddressRequired
	}
	if strings.TrimSpace(cfg.GhostID) == "" {
		return nil, ErrGhostIDRequired
	}
	if strings.TrimSpace(cfg.PeerIdentity) == "" {
		cfg.PeerIdentity = cfg.GhostID
	}
	cfg.Session = cfg.Session.WithDefaults()
	return &MirageClient{
		cfg: cfg,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

// ConnectAndRegister dials Mirage, performs registration handshake, and returns a live session.
func (c *MirageClient) ConnectAndRegister(ctx context.Context) (*MirageSession, error) {
	var attempt int
	for {
		attempt++
		conn, err := c.dial(ctx)
		if err != nil {
			logs.Warnf("ghost.MirageClient dial attempt=%d addr=%q err=%v", attempt, c.cfg.Address, err)
			if !c.shouldRetry(attempt) {
				return nil, err
			}
			if err := c.sleepBackoff(ctx, attempt); err != nil {
				return nil, err
			}
			continue
		}

		sessionConn, err := c.register(conn)
		if err == nil {
			return sessionConn, nil
		}
		_ = conn.Close()
		if errors.Is(err, ErrRegistrationRejected) || !c.shouldRetry(attempt) {
			return nil, err
		}
		if err := c.sleepBackoff(ctx, attempt); err != nil {
			return nil, err
		}
	}
}

func (c *MirageClient) dial(ctx context.Context) (net.Conn, error) {
	if err := c.cfg.Session.ValidateClientTransport(); err != nil {
		return nil, err
	}

	dialer := net.Dialer{Timeout: c.cfg.Session.ConnectTimeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", c.cfg.Address)
	if err != nil {
		return nil, err
	}
	if !c.cfg.Session.TLS.Enabled {
		return rawConn, nil
	}

	tlsCfg, err := c.clientTLSConfig()
	if err != nil {
		_ = rawConn.Close()
		return nil, err
	}
	conn := tls.Client(rawConn, tlsCfg)
	handshakeCtx, cancel := context.WithTimeout(ctx, c.cfg.Session.HandshakeTimeout)
	defer cancel()
	if err := conn.HandshakeContext(handshakeCtx); err != nil {
		_ = rawConn.Close()
		return nil, err
	}
	return conn, nil
}

func (c *MirageClient) clientTLSConfig() (*tls.Config, error) {
	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: c.cfg.Session.TLS.InsecureSkipVerify,
	}

	serverName := strings.TrimSpace(c.cfg.Session.TLS.ServerName)
	if serverName == "" {
		host, _, err := net.SplitHostPort(c.cfg.Address)
		if err != nil {
			return nil, err
		}
		serverName = host
	}
	cfg.ServerName = serverName

	if caPath := strings.TrimSpace(c.cfg.Session.TLS.CAFile); caPath != "" {
		caPEM, err := os.ReadFile(caPath)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(caPEM); !ok {
			return nil, fmt.Errorf("ghost: parse tls ca bundle: %s", caPath)
		}
		cfg.RootCAs = pool
	}

	if c.cfg.Session.TLS.Mutual {
		cert, err := tls.LoadX509KeyPair(c.cfg.Session.TLS.CertFile, c.cfg.Session.TLS.KeyFile)
		if err != nil {
			return nil, err
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}

func (c *MirageClient) shouldRetry(attempt int) bool {
	if c.cfg.MaxConnectAttempts <= 0 {
		return true
	}
	return attempt < c.cfg.MaxConnectAttempts
}

func (c *MirageClient) sleepBackoff(ctx context.Context, attempt int) error {
	delay := session.NextBackoffDelay(c.cfg.Session.Backoff, attempt, c.rng)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *MirageClient) register(conn net.Conn) (*MirageSession, error) {
	_ = conn.SetDeadline(time.Now().Add(c.cfg.Session.HandshakeTimeout))
	reader := bufio.NewReader(conn)
	reg := session.Registration{
		GhostID:      c.cfg.GhostID,
		PeerIdentity: c.cfg.PeerIdentity,
		SeedList:     copySeedList(c.cfg.SeedList),
	}
	if err := session.WriteRegistration(conn, reg); err != nil {
		return nil, err
	}
	ack, err := session.ReadRegistrationAck(reader)
	if err != nil {
		return nil, err
	}
	if ack.Status != session.AckStatusAccepted {
		return nil, fmt.Errorf("%w: code=%d message=%q", ErrRegistrationRejected, ack.Code, ack.Message)
	}
	_ = conn.SetDeadline(time.Time{})
	s := &MirageSession{
		conn:   conn,
		reader: reader,
		cfg:    c.cfg.Session,
		outbox: session.NewEventOutbox(),
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	s.nextMessageID.Store(uint64(time.Now().UnixNano()))
	return s, nil
}

type MirageSession struct {
	conn          net.Conn
	reader        *bufio.Reader
	cfg           session.Config
	outbox        *session.EventOutbox
	nextMessageID atomic.Uint64
	rng           *rand.Rand
	mu            sync.Mutex
}

func (s *MirageSession) Close() error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

func (s *MirageSession) OutboxSnapshot() []session.PendingEvent {
	return s.outbox.List()
}

// SendEventWithAck sends one event and retries until accepted ack or ack timeout.
func (s *MirageSession) SendEventWithAck(ctx context.Context, event EventEnv) (session.EventAck, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return session.EventAck{}, ErrSessionClosed
	}

	wireEvent := session.Event{
		EventID:     strings.TrimSpace(event.EventID),
		CommandID:   strings.TrimSpace(event.CommandID),
		IntentID:    strings.TrimSpace(event.IntentID),
		GhostID:     strings.TrimSpace(event.GhostID),
		SeedID:      strings.TrimSpace(event.SeedID),
		Outcome:     strings.TrimSpace(event.Outcome),
		TimestampMS: event.TimestampMS,
	}
	if wireEvent.TimestampMS == 0 {
		wireEvent.TimestampMS = uint64(time.Now().UnixMilli())
	}
	if err := wireEvent.Validate(); err != nil {
		return session.EventAck{}, err
	}

	start := time.Now()
	deadline := start.Add(s.cfg.AckTimeout)
	s.outbox.Upsert(session.PendingEvent{
		EventID:       wireEvent.EventID,
		CommandID:     wireEvent.CommandID,
		GhostID:       wireEvent.GhostID,
		QueuedAt:      start,
		AckDeadlineAt: deadline,
	})

	attempt := 0
	for {
		attempt++
		_, _ = s.outbox.MarkAttempt(wireEvent.EventID, time.Now(), "")
		ack, err := s.sendEventOnce(ctx, wireEvent)
		if err == nil {
			s.outbox.Remove(wireEvent.EventID)
			if ack.AckStatus == session.AckStatusAccepted {
				return ack, nil
			}
			return ack, fmt.Errorf("%w: status=%s code=%d", ErrAckRejected, ack.AckStatus, ack.AckCode)
		}

		_, _ = s.outbox.MarkAttempt(wireEvent.EventID, time.Now(), err.Error())
		if time.Now().After(deadline) {
			return session.EventAck{}, ErrAckTimeout
		}
		delay := session.NextBackoffDelay(s.cfg.Backoff, attempt, s.rng)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return session.EventAck{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *MirageSession) sendEventOnce(ctx context.Context, event session.Event) (session.EventAck, error) {
	payload, err := session.EncodeEventFrame(s.nextMessageID.Add(1), event)
	if err != nil {
		return session.EventAck{}, err
	}

	if err := s.setWriteDeadline(ctx); err != nil {
		return session.EventAck{}, err
	}
	if _, err := s.conn.Write(payload); err != nil {
		return session.EventAck{}, err
	}

	if err := s.setReadDeadline(ctx); err != nil {
		return session.EventAck{}, err
	}
	fr, err := session.ReadFrame(s.reader, frame.DefaultLimits())
	if err != nil {
		return session.EventAck{}, err
	}
	ack, err := session.DecodeEventAckFrame(fr)
	if err != nil {
		return session.EventAck{}, err
	}
	if ack.EventID != event.EventID {
		return session.EventAck{}, fmt.Errorf("ghost: ack/event mismatch event_id=%q ack_event_id=%q", event.EventID, ack.EventID)
	}
	return ack, nil
}

func (s *MirageSession) setWriteDeadline(ctx context.Context) error {
	deadline := time.Now().Add(s.cfg.WriteTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	return s.conn.SetWriteDeadline(deadline)
}

func (s *MirageSession) setReadDeadline(ctx context.Context) error {
	deadline := time.Now().Add(s.cfg.ReadTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	return s.conn.SetReadDeadline(deadline)
}

func SeedInfoFromMetadata(list []seeds.SeedMetadata) []session.SeedInfo {
	out := make([]session.SeedInfo, 0, len(list))
	for _, meta := range list {
		out = append(out, session.SeedInfo{
			ID:          meta.ID,
			Name:        meta.Name,
			Description: meta.Description,
		})
	}
	return out
}

func copySeedList(in []session.SeedInfo) []session.SeedInfo {
	if len(in) == 0 {
		return []session.SeedInfo{}
	}
	out := make([]session.SeedInfo, len(in))
	copy(out, in)
	return out
}
