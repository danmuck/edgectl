# Transport Slice (Go Definitions)

This file defines minimal transport-layer Go shapes for Mirage<->Ghost sessions.
Contract references:

- `../architecture/transport.md`
- `../architecture/definitions/protocol.toml`

## Connection Roles

```go
type TransportRole string
```

```go
const (
	RoleMirage TransportRole = "mirage"
	RoleGhost  TransportRole = "ghost"
)
```

## Endpoint Configuration

```go
type Endpoint struct {
	Address string
	NodeID  string
	Role    TransportRole
}
```

```go
type TLSConfig struct {
	Enabled  bool
	Mutual   bool
	CertPath string
	KeyPath  string
	CAPath   string
}
```

```go
type SessionConfig struct {
	ConnectTimeoutMS   uint64
	HandshakeTimeoutMS uint64
	ReadTimeoutMS      uint64
	WriteTimeoutMS     uint64
	HeartbeatEveryMS   uint64
	DeadAfterMS        uint64
	MaxFrameBytes      uint64
}
```

## Session Runtime Shapes

```go
type SessionState string
```

```go
const (
	SessionNew       SessionState = "new"
	SessionDialing   SessionState = "dialing"
	SessionReady     SessionState = "ready"
	SessionDraining  SessionState = "draining"
	SessionClosed    SessionState = "closed"
)
```

```go
type Session struct {
	ID         string
	State      SessionState
	Local      Endpoint
	Remote     Endpoint
	GhostID    string
	StartedMS  uint64
	LastSeenMS uint64
}
```

## Minimal Transport Interfaces

```go
type SessionConn interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
}
```

```go
type Dialer interface {
	Dial(cfg SessionConfig, local Endpoint, remote Endpoint, tls TLSConfig) (SessionConn, error)
}
```

```go
type Acceptor interface {
	Accept() (SessionConn, Endpoint, error)
	Close() error
}
```

```go
type Heartbeat interface {
	Tick(sessionID string, nowMS uint64) error
	IsDead(lastSeenMS uint64, nowMS uint64, deadAfterMS uint64) bool
}
```

## Hooks for Mirage/Ghost Runtime

```go
type SessionEvents interface {
	OnSessionReady(s Session)
	OnSessionClosed(s Session, cause error)
	OnPeerIdentified(s Session, ghostID string)
}
```

```go
func NewOutboundGhostSession(local Endpoint, mirage Endpoint, cfg SessionConfig, tls TLSConfig) Session
```

```go
func NewInboundMirageSession(local Endpoint, remote Endpoint, cfg SessionConfig) Session
```
