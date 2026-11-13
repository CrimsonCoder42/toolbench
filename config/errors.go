package config

import "errors"

// ErrMissing is wrapped, once per key, by the error Required returns for a key that is not set or is set
// to the empty string. The message names the key, never a value.
//
// ID:          TB-CONFIG-017
// Pattern:     sentinel error
// Authority:   pkg.go.dev errors, "func Is" and "func Join"
// Tags:        errors, sentinel, config
var ErrMissing = errors.New("config: required key not set")

// ErrParse is wrapped by Int, Duration and Bool when a key is set but its value does not parse. The message
// names the key and the kind of failure (strconv.ErrSyntax or strconv.ErrRange, also wrapped), never the
// value, so a secret mistyped into a numeric key cannot reach a log through the error.
//
// ID:          TB-CONFIG-018
// Pattern:     sentinel error
// Authority:   pkg.go.dev errors, "func Is"; pkg.go.dev strconv, "type NumError"
// Tags:        errors, sentinel, config, parse
var ErrParse = errors.New("config: value does not parse")

// ErrJSON is wrapped by FromJSON and FromJSONFile when the input is not one JSON object that flattens to
// string keys: a syntax error, a top-level value that is not an object, trailing data, a duplicate or
// colliding key, an empty key, an array holding an object, an array or null, an array element containing a
// comma, nesting deeper than 64 levels, or more than 4 MiB of input. The message names the key or the byte
// offset, never a value or any character of the input (a json.SyntaxError, whose text quotes one input
// character, is replaced by its offset). An error from the caller's io.Reader is wrapped unchanged.
//
// ID:          TB-CONFIG-019
// Pattern:     sentinel error
// Authority:   pkg.go.dev errors, "func Is"; pkg.go.dev encoding/json, "type Decoder" (method Token) and "type SyntaxError"
// Tags:        errors, sentinel, config, json
var ErrJSON = errors.New("config: invalid JSON config")
