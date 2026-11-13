package config_test

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/CrimsonCoder42/toolbench/config"
)

func ExampleLayer() {
	defaults := config.FromMap(map[string]string{"db.host": "localhost", "db.port": "5432", "log.level": "info"})
	file, _ := config.FromJSON(strings.NewReader(`{"db": {"host": "db.internal"}}`))
	env := config.FromEnv(envOf(map[string]string{"APP_DB_PORT": "6432"}), "APP_") // os.LookupEnv in production
	l := config.Layer(defaults, file, env)
	fmt.Println(config.String(l, "db.host", ""), config.String(l, "db.port", ""), config.String(l, "log.level", ""))
	// Output:
	// db.internal 6432 info
}

func ExampleFromMap() {
	l := config.FromMap(map[string]string{"http.addr": ":8080"})
	v, ok := l("http.addr")
	_, missing := l("http.tls")
	fmt.Println(v, ok, missing)
	// Output:
	// :8080 true false
}

func ExampleLookup() {
	var l config.Lookup = func(key string) (string, bool) {
		if key == "mode" {
			return "", true // set, to the empty string
		}
		return "", false
	}
	v, ok := l("mode")
	fmt.Printf("%q %v\n", v, ok)
	// Output:
	// "" true
}

// TestLayer_Precedence checks defaults < file < environment < flags on four keys, each set in a growing
// number of layers.
func TestLayer_Precedence(t *testing.T) {
	defaults := config.FromMap(map[string]string{"a": "default", "b": "default", "c": "default", "d": "default"})
	file, err := config.FromJSONFile(fstest.MapFS{"app.json": {Data: []byte(`{"b":"file","c":"file","d":"file"}`)}}, "app.json")
	if err != nil {
		t.Fatal(err)
	}
	env := config.FromEnv(envOf(map[string]string{"APP_C": "env", "APP_D": "env"}), "APP_")
	flags := config.FromFlags(flagsOf(t, []string{"a", "b", "c", "d"}, "-d=flag"))
	l := config.Layer(defaults, file, env, flags)
	for key, want := range map[string]string{"a": "default", "b": "file", "c": "env", "d": "flag"} {
		if got := config.String(l, key, "none"); got != want {
			t.Errorf("%s: got %q, want %q", key, got, want)
		}
	}
	if got := config.String(l, "e", "none"); got != "none" {
		t.Errorf("unset key: got %q, want the default", got)
	}
}

func TestLayer_EmptyOverrides(t *testing.T) {
	l := config.Layer(config.FromMap(map[string]string{"k": "low"}), config.FromMap(map[string]string{"k": ""}))
	if v, ok := l("k"); !ok || v != "" {
		t.Errorf("got (%q, %v), want (\"\", true): an empty string set later is set", v, ok)
	}
}

func TestLayer_NilAndEmpty(t *testing.T) {
	if _, ok := config.Layer()("k"); ok {
		t.Error("Layer() found a key")
	}
	l := config.Layer(nil, config.FromMap(map[string]string{"k": "v"}), nil)
	if v, ok := l("k"); !ok || v != "v" {
		t.Errorf("got (%q, %v), want (v, true)", v, ok)
	}
	if got := config.String(nil, "k", "def"); got != "def" {
		t.Errorf("nil Lookup: got %q", got)
	}
}

func TestLayer_ArgumentsCopied(t *testing.T) {
	ls := []config.Lookup{config.FromMap(map[string]string{"k": "first"})}
	l := config.Layer(ls...)
	ls[0] = config.FromMap(map[string]string{"k": "changed"})
	if v, _ := l("k"); v != "first" {
		t.Errorf("got %q after changing the argument slice", v)
	}
}

func TestFromMap_Copied(t *testing.T) {
	m := map[string]string{"k": "v"}
	l := config.FromMap(m)
	m["k"] = "changed"
	delete(m, "k")
	if v, ok := l("k"); !ok || v != "v" {
		t.Errorf("got (%q, %v) after mutating the map", v, ok)
	}
	if _, ok := config.FromMap(nil)("k"); ok {
		t.Error("nil map found a key")
	}
}
