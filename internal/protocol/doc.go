// Package protocol implements the binary control-plane protocol wire format.
//
// It provides encoding/decoding for the fixed header, optional auth block,
// and flat TLV payload fields, plus an optional semantic parsing layer.
package protocol
