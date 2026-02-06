# EdgeCTL TLV Specification (Pseudocode)

This file defines the control-plane TLV mapping used by all envelopes.
It is a documentation contract for field layout and field sections.

## Payload Layout

```go
// Payload = zero or more flat TLV fields
struct TLVField {
  FieldID uint16
  Type    uint8
  Length  uint32
  Value   []byte
}
```

## Primitive Types

```go
// Canonical FieldType IDs
const (
  TYPE_U8     uint8 = 1
  TYPE_U16    uint8 = 2
  TYPE_U32    uint8 = 3
  TYPE_U64    uint8 = 4
  TYPE_BOOL   uint8 = 5
  TYPE_STRING uint8 = 6
  TYPE_BYTES  uint8 = 7
)
```

## MessageType IDs

```go
// MessageType drives semantic parsing only
const (
  MSG_INTENT      	uint32 = 1
  MSG_COMMAND     	uint32 = 2
  MSG_EVENT       	uint32 = 3
  MSG_STREAM_OPEN		uint32 = 4
  MSG_STREAM_DATA		uint32 = 5
  MSG_STREAM_CLOSE	uint32 = 6
  MSG_ERROR       	uint32 = 7
)
```

## Field Sections

```go
// Section A: common correlation fields (shared across envelopes)
const (
  F_INTENT_ID    uint16 = 1  // TYPE_STRING
  F_COMMAND_ID   uint16 = 2  // TYPE_STRING
  F_EXECUTION_ID uint16 = 3  // TYPE_STRING
  F_EVENT_ID     uint16 = 4  // TYPE_STRING
  F_PHASE        uint16 = 5  // TYPE_STRING
  F_TIMESTAMP_MS uint16 = 6  // TYPE_U64
)
```

```go
// Section B: issue envelope fields (User -> Mirage)
const (
  F_ACTOR        uint16 = 100 // TYPE_STRING
  F_TARGET_SCOPE uint16 = 101 // TYPE_STRING
  F_OBJECTIVE    uint16 = 102 // TYPE_STRING
)
```

```go
// Section C: command envelope fields (Mirage -> Ghost)
const (
  F_GHOST_ID      uint16 = 200 // TYPE_STRING
  F_SEED_SELECTOR uint16 = 201 // TYPE_STRING
  F_OPERATION     uint16 = 202 // TYPE_STRING
  F_ARGS_JSON     uint16 = 203 // TYPE_BYTES
)
```

```go
// Section D: seed_execute envelope fields (Ghost -> Seed)
const (
  F_SEED_ID        uint16 = 300 // TYPE_STRING
  F_EXEC_OPERATION uint16 = 301 // TYPE_STRING
  F_EXEC_ARGS_JSON uint16 = 302 // TYPE_BYTES
)
```

```go
// Section E: seed_result envelope fields (Seed -> Ghost)
const (
  F_STATUS    uint16 = 400 // TYPE_STRING
  F_STDOUT    uint16 = 401 // TYPE_BYTES
  F_STDERR    uint16 = 402 // TYPE_BYTES
  F_EXIT_CODE uint16 = 403 // TYPE_U32
)
```

```go
// Section F: event envelope fields (Ghost -> Mirage)
const (
  F_OUTCOME uint16 = 500 // TYPE_STRING
)
```

```c
// Section G: report envelope fields (Mirage -> User)
const (
  F_SUMMARY          uint16 = 600 // TYPE_STRING
  F_COMPLETION_STATE uint16 = 601 // TYPE_STRING
)
```

## Required Field Sets

```bash
// Required per semantic envelope
required[MSG_INTENT] = {
  F_INTENT_ID, F_ACTOR, F_TARGET_SCOPE, F_OBJECTIVE
}

required[MSG_COMMAND] = {
  F_COMMAND_ID, F_INTENT_ID, F_GHOST_ID, F_SEED_SELECTOR, F_OPERATION
}

required[seed_execute] = {
  F_EXECUTION_ID, F_COMMAND_ID, F_SEED_ID, F_EXEC_OPERATION
}

required[seed_result] = {
  F_EXECUTION_ID, F_SEED_ID, F_STATUS, F_STDOUT, F_STDERR, F_EXIT_CODE
}

required[MSG_EVENT] = {
  F_EVENT_ID, F_COMMAND_ID, F_INTENT_ID, F_GHOST_ID, F_SEED_ID, F_OUTCOME
}

required[report] = {
  F_INTENT_ID, F_PHASE, F_SUMMARY, F_COMPLETION_STATE
}
```

## Decoder/Parser Rules

```go
// Decoder rules
// - decode TLV fields without MessageType branching
// - reject malformed lengths
// - preserve unknown FieldID values for re-encode
// - ignore unknown flags

// Semantic parser rules
// - use MessageType to validate required fields
// - map known fields into typed envelope structs
// - ignore unknown fields
```
