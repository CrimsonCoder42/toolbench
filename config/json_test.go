package config_test

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/CrimsonCoder42/toolbench/config"
)

func ExampleFromJSON() {
	l, err := config.FromJSON(strings.NewReader(`{
		"db": {"host": "db.internal", "port": 5432, "tls": true, "replica": null},
		"cors": {"origins": ["a.example", "b.example"]},
		"ratio": 1.5e3
	}`))
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, k := range []string{"db.host", "db.port", "db.tls", "db.replica", "cors.origins", "ratio"} {
		v, ok := l(k)
		fmt.Printf("%s=%q %v\n", k, v, ok)
	}
	// Output:
	// db.host="db.internal" true
	// db.port="5432" true
	// db.tls="true" true
	// db.replica="" false
	// cors.origins="a.example,b.example" true
	// ratio="1.5e3" true
}

func ExampleFromJSONFile() {
	fsys := fstest.MapFS{"config/app.json": {Data: []byte(`{"http": {"addr": ":8080"}}`)}} // os.DirFS in production
	l, err := config.FromJSONFile(fsys, "config/app.json")
	fmt.Println(config.String(l, "http.addr", ""), err)
	_, err = config.FromJSONFile(fsys, "config/missing.json")
	fmt.Println(errors.Is(err, fs.ErrNotExist))
	// Output:
	// :8080 <nil>
	// true
}

func TestFromJSON_Flatten(t *testing.T) {
	l, err := config.FromJSON(strings.NewReader(`{"a":{"b":{"c":"deep"}},"n":-0.5,"big":12345678901234567890,"f":false,"e":[],"s":"","x.y":"dotted"}`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"a.b.c": "deep", "n": "-0.5", "big": "12345678901234567890", "f": "false", "e": "", "s": "", "x.y": "dotted"}
	for k, w := range want {
		if v, ok := l(k); !ok || v != w {
			t.Errorf("%s: got (%q, %v), want (%q, true)", k, v, ok, w)
		}
	}
	for _, k := range []string{"a", "a.b", "A.B.C"} {
		if v, ok := l(k); ok {
			t.Errorf("%s: found %q; objects are not values and names are case-sensitive", k, v)
		}
	}
}

func TestFromJSON_EmptyObject(t *testing.T) {
	l, err := config.FromJSON(strings.NewReader(` { } `))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := l("a"); ok {
		t.Error("found a key in {}")
	}
}

func TestFromJSON_Errors(t *testing.T) {
	cases := map[string]string{
		"empty input":       ``,
		"syntax":            `{"a":}`,
		"truncated":         `{"a":"b"`,
		"not an object":     `["a"]`,
		"scalar top":        `"a"`,
		"trailing data":     `{"a":"b"} {"c":"d"}`,
		"duplicate":         `{"a":"1","a":"2"}`,
		"duplicate null":    `{"a":null,"a":"2"}`,
		"object collision":  `{"a":{"b":"1"},"a":{"c":"2"}}`,
		"dotted collision":  `{"a.b":"1","a":{"b":"2"}}`,
		"scalar vs object":  `{"a":"1","a":{"b":"2"}}`,
		"empty key":         `{"":"x"}`,
		"nested empty key":  `{"a":{"":"x"}}`,
		"array of objects":  `{"a":[{"b":1}]}`,
		"array of arrays":   `{"a":[[1]]}`,
		"array with null":   `{"a":[null]}`,
		"comma in element":  `{"a":["x,y"]}`,
		"too deep":          strings.Repeat(`{"a":`, 70) + `"x"` + strings.Repeat(`}`, 70),
		"too large":         `{"a":"` + strings.Repeat("x", 5<<20) + `"}`,
		"too large trailer": `{"a":"b"}` + strings.Repeat(" ", 5<<20),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			l, err := config.FromJSON(strings.NewReader(in))
			if !errors.Is(err, config.ErrJSON) {
				t.Errorf("got err %v, want ErrJSON", err)
			}
			if l != nil {
				t.Error("got a Lookup with an error")
			}
		})
	}
	if _, err := config.FromJSON(nil); !errors.Is(err, config.ErrJSON) {
		t.Errorf("nil reader: got %v", err)
	}
}

func TestFromJSON_DepthAtLimit(t *testing.T) {
	nest := func(n int) string { return strings.Repeat(`{"a":`, n) + `"x"` + strings.Repeat(`}`, n) }
	l, err := config.FromJSON(strings.NewReader(nest(64)))
	if err != nil {
		t.Fatalf("64 levels: %v", err)
	}
	if v, _ := l(strings.TrimSuffix(strings.Repeat("a.", 64), ".")); v != "x" {
		t.Errorf("got %q", v)
	}
	if _, err := config.FromJSON(strings.NewReader(nest(65))); !errors.Is(err, config.ErrJSON) {
		t.Errorf("65 levels: got %v, want ErrJSON", err)
	}
}

func TestFromJSON_SyntaxErrorOffsetOnly(t *testing.T) {
	_, err := config.FromJSON(strings.NewReader(`{"a" 1}`))
	if !errors.Is(err, config.ErrJSON) {
		t.Fatalf("got %v", err)
	}
	_, err = config.FromJSON(strings.NewReader(`{"a":Zebra}`))
	if msg := errText(err); !strings.HasPrefix(msg, "config: invalid JSON config: syntax error at byte ") || strings.ContainsAny(msg, "'Z") {
		t.Errorf("got %q, want the offset only, no input character", msg)
	}
}

func TestFromJSONFile_Errors(t *testing.T) {
	fsys := fstest.MapFS{"bad.json": {Data: []byte(`{`)}}
	if _, err := config.FromJSONFile(fsys, "nope.json"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing: got %v", err)
	}
	if _, err := config.FromJSONFile(fsys, "/abs.json"); err == nil {
		t.Error("invalid path: got nil error")
	}
	_, err := config.FromJSONFile(fsys, "bad.json")
	if !errors.Is(err, config.ErrJSON) || !strings.Contains(err.Error(), "bad.json") {
		t.Errorf("malformed: got %v, want ErrJSON naming the file", err)
	}
	if _, err := config.FromJSONFile(nil, "x.json"); !errors.Is(err, config.ErrJSON) {
		t.Errorf("nil fs: got %v", err)
	}
}
