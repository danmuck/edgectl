package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	controlTypeRegister    = "seed.register"
	controlTypeRegisterAck = "seed.register.ack"

	AckStatusAccepted = "accepted"
	AckStatusRejected = "rejected"
)

var (
	ErrInvalidRegistration    = errors.New("session: invalid registration")
	ErrInvalidRegistrationAck = errors.New("session: invalid registration ack")
	ErrControlMessageTooLarge = errors.New("session: control message too large")
)

// Session handshake descriptor for one seed entry.
type SeedInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Session seed.register payload from Ghost to Mirage.
type Registration struct {
	GhostID      string     `json:"ghost_id"`
	PeerIdentity string     `json:"peer_identity"`
	SeedList     []SeedInfo `json:"seed_list"`
}

// Session seed.register validator for required payload fields.
func (r Registration) Validate() error {
	if strings.TrimSpace(r.GhostID) == "" {
		return fmt.Errorf("%w: missing ghost_id", ErrInvalidRegistration)
	}
	if r.SeedList == nil {
		return fmt.Errorf("%w: missing seed_list", ErrInvalidRegistration)
	}
	for i, seed := range r.SeedList {
		if strings.TrimSpace(seed.ID) == "" {
			return fmt.Errorf("%w: seed_list[%d] missing id", ErrInvalidRegistration, i)
		}
		if strings.TrimSpace(seed.Name) == "" {
			return fmt.Errorf("%w: seed_list[%d] missing name", ErrInvalidRegistration, i)
		}
		if strings.TrimSpace(seed.Description) == "" {
			return fmt.Errorf("%w: seed_list[%d] missing description", ErrInvalidRegistration, i)
		}
	}
	return nil
}

// Session seed.register.ack payload from Mirage to Ghost.
type RegistrationAck struct {
	Status      string `json:"status"`
	Code        uint32 `json:"code"`
	Message     string `json:"message"`
	GhostID     string `json:"ghost_id"`
	TimestampMS uint64 `json:"timestamp_ms"`
}

// Session seed.register.ack validator for required payload fields.
func (a RegistrationAck) Validate() error {
	status := strings.TrimSpace(a.Status)
	if status != AckStatusAccepted && status != AckStatusRejected {
		return fmt.Errorf("%w: invalid status", ErrInvalidRegistrationAck)
	}
	if strings.TrimSpace(a.GhostID) == "" {
		return fmt.Errorf("%w: missing ghost_id", ErrInvalidRegistrationAck)
	}
	if a.TimestampMS == 0 {
		return fmt.Errorf("%w: missing timestamp_ms", ErrInvalidRegistrationAck)
	}
	return nil
}

// Session control-plane envelope for handshake payload variants.
type controlEnvelope struct {
	Type string           `json:"type"`
	Reg  *Registration    `json:"registration,omitempty"`
	Ack  *RegistrationAck `json:"registration_ack,omitempty"`
}

// Session writer for one newline-delimited seed.register envelope.
func WriteRegistration(w io.Writer, reg Registration) error {
	if err := reg.Validate(); err != nil {
		return err
	}
	return writeControlEnvelope(w, controlEnvelope{
		Type: controlTypeRegister,
		Reg:  &reg,
	})
}

// Session reader for one validated seed.register envelope.
func ReadRegistration(r *bufio.Reader) (Registration, error) {
	env, err := readControlEnvelope(r)
	if err != nil {
		return Registration{}, err
	}
	if env.Type != controlTypeRegister || env.Reg == nil {
		return Registration{}, fmt.Errorf("%w: unexpected control type", ErrInvalidRegistration)
	}
	if err := env.Reg.Validate(); err != nil {
		return Registration{}, err
	}
	return *env.Reg, nil
}

// Session writer for one newline-delimited seed.register.ack envelope.
func WriteRegistrationAck(w io.Writer, ack RegistrationAck) error {
	if err := ack.Validate(); err != nil {
		return err
	}
	return writeControlEnvelope(w, controlEnvelope{
		Type: controlTypeRegisterAck,
		Ack:  &ack,
	})
}

// Session reader for one validated seed.register.ack envelope.
func ReadRegistrationAck(r *bufio.Reader) (RegistrationAck, error) {
	env, err := readControlEnvelope(r)
	if err != nil {
		return RegistrationAck{}, err
	}
	if env.Type != controlTypeRegisterAck || env.Ack == nil {
		return RegistrationAck{}, fmt.Errorf("%w: unexpected control type", ErrInvalidRegistrationAck)
	}
	if err := env.Ack.Validate(); err != nil {
		return RegistrationAck{}, err
	}
	return *env.Ack, nil
}

// Session helper that serializes one newline-delimited JSON control envelope.
func writeControlEnvelope(w io.Writer, env controlEnvelope) error {
	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if _, err := w.Write(payload); err != nil {
		return err
	}
	return nil
}

// Session helper that reads one newline-delimited JSON control envelope.
func readControlEnvelope(r *bufio.Reader) (controlEnvelope, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return controlEnvelope{}, err
	}
	if len(line) > 128*1024 {
		return controlEnvelope{}, ErrControlMessageTooLarge
	}
	var env controlEnvelope
	if err := json.Unmarshal(line, &env); err != nil {
		return controlEnvelope{}, err
	}
	return env, nil
}
