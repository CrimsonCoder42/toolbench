// Package ctxkey gives context values a typed key that cannot collide with another package's key: one
// package-level variable replaces the unexported key type, the two accessor functions and the type assertion
// that the context documentation asks every user of context.WithValue to write.
//
// ID short form: CTXKEY (IDs are TB-CTXKEY-NNN).
//
// Exports:
//
//	TB-CTXKEY-001  Key  a typed context key: With stores a T, Value reads it back as (T, bool)
//	TB-CTXKEY-002  New  returns a new, distinct key with a name used only for printing
//	TB-CTXKEY-003  RequestID  the one key every toolbench package uses for the request / correlation id
//	TB-CTXKEY-004  MaxRequestIDLen  the longest request id ValidRequestID accepts: 128 bytes
//	TB-CTXKEY-005  ErrRequestID  the sentinel WithRequestID wraps for an empty, too long or non-printable id
//	TB-CTXKEY-006  NewRequestID  a fresh request id from crypto/rand.Text (26 base32 characters, >= 128 bits)
//	TB-CTXKEY-007  ValidRequestID  reports whether s is 1..128 bytes of printable ASCII 0x21-0x7E
//	TB-CTXKEY-008  WithRequestID  validates id and stores it under RequestID, or returns ctx unchanged and ErrRequestID
//	TB-CTXKEY-009  RequestIDFrom  the request id stored in ctx as (string, bool)
//	TB-CTXKEY-010  EnsureRequestID  the stored valid request id, or a fresh one stored in a child context
package ctxkey
