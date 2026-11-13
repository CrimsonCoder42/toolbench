package config_test

import (
	"flag"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/config"
)

func ExampleFromFlags() {
	fs := flag.NewFlagSet("app", flag.ContinueOnError)
	fs.String("db-host", "ignored-default", "database host")
	fs.Int("db-port", 5432, "database port")
	_ = fs.Parse([]string{"-db-host=db.internal"})

	l := config.FromFlags(fs)
	fmt.Println(config.String(l, "db.host", "none"))
	fmt.Println(config.String(l, "db.port", "none")) // defined but not set: not found
	// Output:
	// db.internal
	// none
}

func ExampleFlagName() {
	fmt.Println(config.FlagName("db.host"), config.FlagName("http.read_timeout"))
	// Output:
	// db-host http-read-timeout
}

func TestFromFlags_OnlySet(t *testing.T) {
	fs := flagsOf(t, []string{"db-host", "db-port"}, "-db-port", "6432")
	l := config.FromFlags(fs)
	if v, ok := l("db.port"); !ok || v != "6432" {
		t.Errorf("set flag: got (%q, %v)", v, ok)
	}
	if v, ok := l("db.host"); ok {
		t.Errorf("unset flag found with %q: a flag default must not override lower layers", v)
	}
	if _, ok := l("undefined"); ok {
		t.Error("undefined flag found")
	}
}

func TestFromFlags_ReadAtLookup(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Bool("verbose", false, "")
	fs.Duration("timeout", 0, "")
	l := config.FromFlags(fs) // before Parse
	if err := fs.Parse([]string{"-verbose", "-timeout=90s"}); err != nil {
		t.Fatal(err)
	}
	if b, err := config.Bool(l, "verbose", false); err != nil || !b {
		t.Errorf("bool flag: got (%v, %v)", b, err)
	}
	if d, err := config.Duration(l, "timeout", 0); err != nil || d != 90*time.Second {
		t.Errorf("duration flag: got (%v, %v)", d, err)
	}
}

func TestFromFlags_Nil(t *testing.T) {
	if _, ok := config.FromFlags(nil)("x"); ok {
		t.Error("nil FlagSet found a key")
	}
	if _, ok := config.FromFlags(flagsOf(t, nil))(""); ok {
		t.Error("empty key found")
	}
}
