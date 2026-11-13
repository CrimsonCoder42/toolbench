package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
)

// The bounds of FromJSON: a config file is small and shallow; anything larger is refused, not read.
const (
	maxJSONBytes = 4 << 20 // 4 MiB
	maxJSONDepth = 64
)

// FromJSON reads one JSON object from r and returns a Lookup over its flattened keys. Flattening:
//   - nested objects join their names with '.': {"db":{"host":"x"}} sets "db.host";
//   - strings are taken as is; numbers keep their literal text ("8080", "1.5e3"; no float64 round trip);
//     booleans are "true" or "false";
//   - null leaves the key unset (a lower layer or the default applies);
//   - an array of strings, numbers and booleans is joined with ',' ([1,2] sets "1,2"; [] sets "");
//
// Names are matched exactly (no case folding): write file keys in canonical lowercase. Everything else is
// ErrJSON (see its doc): a duplicate key is an error, not last-wins, because a silently overridden setting
// is a defect. At most 4 MiB is read and nesting is bounded at 64 levels. r is read to the end of the object
// plus any trailing data and is not closed. A nil r is ErrJSON.
//
// ID:          TB-CONFIG-004
// Pattern:     layered configuration (file layer)
// Authority:   pkg.go.dev encoding/json, "func (*Decoder) Token" and "func (*Decoder) UseNumber", go1.27
// Errors:      ErrJSON; a syntax error is reported by byte offset only, never an input character or value
// Tags:        config, json, file, flatten
func FromJSON(r io.Reader) (Lookup, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil reader", ErrJSON)
	}
	lr := &io.LimitedReader{R: r, N: maxJSONBytes + 1}
	dec := json.NewDecoder(lr)
	dec.UseNumber()
	f := flattener{dec: dec, out: map[string]string{}, seen: map[string]bool{}}
	tok, err := dec.Token()
	if err != nil {
		return nil, f.wrap(lr, err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("%w: top-level value is not an object", ErrJSON)
	}
	if err := f.object("", 1); err != nil {
		return nil, f.wrap(lr, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("%w: data after the top-level object", ErrJSON)
		}
		return nil, f.wrap(lr, err)
	}
	if lr.N <= 0 {
		return nil, fmt.Errorf("%w: input larger than %d bytes", ErrJSON, maxJSONBytes)
	}
	return FromMap(f.out), nil
}

// FromJSONFile reads the file name from fsys (callers pass os.DirFS or os.Root.FS; tests pass
// fstest.MapFS) and returns FromJSON of its content. The file is closed before FromJSONFile returns. A
// missing file is an error wrapping fs.ErrNotExist, so a caller that wants an optional file checks
// errors.Is(err, fs.ErrNotExist) itself; nothing is ignored and nothing exits the process. A nil fsys is
// ErrJSON.
//
// ID:          TB-CONFIG-005
// Pattern:     layered configuration (file layer)
// Authority:   pkg.go.dev io/fs, "type FS" and "func ReadFile"; pkg.go.dev encoding/json, "func (*Decoder) Token", go1.27
// Errors:      the *fs.PathError from Open (fs.ErrNotExist, fs.ErrInvalid, ...); ErrJSON as FromJSON
// Tags:        config, json, file, fs
func FromJSONFile(fsys fs.FS, name string) (Lookup, error) {
	if fsys == nil {
		return nil, fmt.Errorf("%w: nil fs.FS", ErrJSON)
	}
	f, err := fsys.Open(name)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	l, err := FromJSON(f)
	cerr := f.Close()
	if err != nil {
		return nil, fmt.Errorf("config: %s: %w", name, err)
	}
	if cerr != nil {
		return nil, fmt.Errorf("config: %s: %w", name, cerr)
	}
	return l, nil
}

type flattener struct {
	dec  *json.Decoder
	out  map[string]string
	seen map[string]bool // every key defined, including null ones, for duplicate detection
}

// wrap turns a decoder error into ErrJSON; a read past the size bound is reported as such.
func (f *flattener) wrap(lr *io.LimitedReader, err error) error {
	if errors.Is(err, ErrJSON) {
		return err
	}
	if lr.N <= 0 {
		return fmt.Errorf("%w: input larger than %d bytes", ErrJSON, maxJSONBytes)
	}
	if errors.Is(err, io.EOF) {
		err = io.ErrUnexpectedEOF
	}
	if se, ok := errors.AsType[*json.SyntaxError](err); ok {
		// SyntaxError's text quotes the offending input character: keep only the offset.
		return fmt.Errorf("%w: syntax error at byte %d", ErrJSON, se.Offset)
	}
	return fmt.Errorf("%w: at byte %d: %w", ErrJSON, f.dec.InputOffset(), err)
}

func (f *flattener) define(key string) error {
	if f.seen[key] {
		return fmt.Errorf("%w: duplicate key %q", ErrJSON, key)
	}
	f.seen[key] = true
	return nil
}

// object reads the members of an object whose '{' was consumed, through its '}'.
func (f *flattener) object(prefix string, depth int) error {
	if depth > maxJSONDepth {
		return fmt.Errorf("%w: nesting deeper than %d", ErrJSON, maxJSONDepth)
	}
	for f.dec.More() {
		tok, err := f.dec.Token()
		if err != nil {
			return err
		}
		name, _ := tok.(string) // object keys are always strings
		if name == "" {
			return fmt.Errorf("%w: empty key under %q", ErrJSON, prefix)
		}
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		if err := f.value(key, depth); err != nil {
			return err
		}
	}
	_, err := f.dec.Token() // the closing '}'
	return err
}

func (f *flattener) value(key string, depth int) error {
	tok, err := f.dec.Token()
	if err != nil {
		return err
	}
	switch t := tok.(type) {
	case json.Delim:
		if err := f.define(key); err != nil {
			return err
		}
		if t == '{' {
			return f.object(key, depth+1)
		}
		return f.array(key)
	case nil:
		return f.define(key)
	default:
		if err := f.define(key); err != nil {
			return err
		}
		f.out[key] = scalar(t)
		return nil
	}
}

// array reads the elements of an array whose '[' was consumed, through its ']'.
func (f *flattener) array(key string) error {
	var parts []string
	for f.dec.More() {
		tok, err := f.dec.Token()
		if err != nil {
			return err
		}
		switch tok.(type) {
		case json.Delim, nil:
			return fmt.Errorf("%w: key %q: an array may hold only strings, numbers and booleans", ErrJSON, key)
		}
		s := scalar(tok)
		if strings.Contains(s, ",") {
			return fmt.Errorf("%w: key %q: an array element contains ','", ErrJSON, key)
		}
		parts = append(parts, s)
	}
	if _, err := f.dec.Token(); err != nil { // the closing ']'
		return err
	}
	f.out[key] = strings.Join(parts, ",")
	return nil
}

func scalar(tok json.Token) string {
	switch t := tok.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "true"
		}
		return "false"
	}
	return ""
}
