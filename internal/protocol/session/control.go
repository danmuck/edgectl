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

// SeedInfo is the handshake shape for one seed descriptor.
type SeedInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Registration is the Ghost->Mirage session-start payload.
type Registration struct {
	GhostID      string     `json:"ghost_id"`
	PeerIdentity string     `json:"peer_identity"`
	SeedList     []SeedInfo `json:"seed_list"`
}

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

// RegistrationAck is the Mirage->Ghost registration response.
type RegistrationAck struct {
	Status      string `json:"status"`
	Code        uint32 `json:"code"`
	Message     string `json:"message"`
	GhostID     string `json:"ghost_id"`
	TimestampMS uint64 `json:"timestamp_ms"`
}

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

type controlEnvelope struct {
	Type string           `json:"type"`
	Reg  *Registration    `json:"registration,omitempty"`
	Ack  *RegistrationAck `json:"registration_ack,omitempty"`
}

func WriteRegistration(w io.Writer, reg Registration) error {
	if err := reg.Validate(); err != nil {
		return err
	}
	return writeControlEnvelope(w, controlEnvelope{
		Type: controlTypeRegister,
		Reg:  &reg,
	})
}

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

func WriteRegistrationAck(w io.Writer, ack RegistrationAck) error {
	if err := ack.Validate(); err != nil {
		return err
	}
	return writeControlEnvelope(w, controlEnvelope{
		Type: controlTypeRegisterAck,
		Ack:  &ack,
	})
}

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
