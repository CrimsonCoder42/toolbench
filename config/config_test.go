package config_test

import (
	"flag"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

// secret is a made-up credential the tests hand to the package; no output may ever contain it.
const secret = "hunter2-Zq9-s3cr3t"

// envOf is the injected environment of every test: a map, never the process environment.
func envOf(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

// flagsOf returns a fresh FlagSet with the given string flags defined and args parsed.
func flagsOf(t testing.TB, names []string, args ...string) *flag.FlagSet {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	for _, n := range names {
		fs.String(n, "flag-default", "")
	}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return fs
}

// noSecret fails the test when out contains the secret.
func noSecret(t *testing.T, what, out string) {
	t.Helper()
	if strings.Contains(out, secret) {
		t.Errorf("%s leaks the secret (%d bytes): %s", what, len(out), masked(out))
	}
}

// masked replaces the secret in s, so a failing assertion never prints it either.
func masked(s string) string { return strings.ReplaceAll(s, secret, "<SECRET>") }

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
