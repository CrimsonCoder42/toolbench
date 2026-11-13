package config_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/CrimsonCoder42/toolbench/config"
)

func ExampleRequired() {
	l := config.FromMap(map[string]string{"db.host": "db.internal", "db.password": ""})
	err := config.Required(l, "db.host", "db.password", "db.user")
	fmt.Println(err)
	fmt.Println(errors.Is(err, config.ErrMissing))
	// Output:
	// config: required key not set: "db.password"
	// config: required key not set: "db.user"
	// true
}

func TestRequired_AllMissingListed(t *testing.T) {
	keys := []string{"a", "b", "c", "d", "b"}
	err := config.Required(config.FromMap(map[string]string{"c": "set"}), keys...)
	if !errors.Is(err, config.ErrMissing) {
		t.Fatalf("got %v", err)
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("not an errors.Join: %T", err)
	}
	var got []string
	for _, e := range joined.Unwrap() {
		if !errors.Is(e, config.ErrMissing) {
			t.Errorf("part %v does not wrap ErrMissing", e)
		}
		got = append(got, e.Error())
	}
	want := []string{`config: required key not set: "a"`, `config: required key not set: "b"`, `config: required key not set: "d"`}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRequired_AllPresent(t *testing.T) {
	if err := config.Required(config.FromMap(map[string]string{"a": "1"}), "a"); err != nil {
		t.Errorf("got %v", err)
	}
	if err := config.Required(nil); err != nil {
		t.Errorf("no keys: got %v", err)
	}
	if err := config.Required(nil, "a"); !errors.Is(err, config.ErrMissing) {
		t.Errorf("nil lookup: got %v", err)
	}
}
