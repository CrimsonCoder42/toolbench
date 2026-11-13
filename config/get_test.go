package config_test

import (
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/config"
)

func ExampleString() {
	l := config.FromMap(map[string]string{"name": "api", "empty": ""})
	fmt.Printf("%q %q %q\n", config.String(l, "name", "x"), config.String(l, "empty", "x"), config.String(l, "unset", "x"))
	// Output:
	// "api" "" "x"
}

func ExampleInt() {
	l := config.FromMap(map[string]string{"port": "8080", "workers": "eight"})
	p, err := config.Int(l, "port", 80)
	fmt.Println(p, err)
	d, err := config.Int(l, "retries", 3)
	fmt.Println(d, err)
	_, err = config.Int(l, "workers", 4)
	fmt.Println(err)
	fmt.Println(errors.Is(err, config.ErrParse), errors.Is(err, strconv.ErrSyntax))
	// Output:
	// 8080 <nil>
	// 3 <nil>
	// config: value does not parse: key "workers" is not a valid int: invalid syntax
	// true true
}

func ExampleDuration() {
	l := config.FromMap(map[string]string{"timeout": "1m30s"})
	d, err := config.Duration(l, "timeout", time.Second)
	fmt.Println(d, err)
	// Output:
	// 1m30s <nil>
}

func ExampleBool() {
	l := config.FromMap(map[string]string{"debug": "TRUE"})
	b, err := config.Bool(l, "debug", false)
	fmt.Println(b, err)
	// Output:
	// true <nil>
}

func TestInt(t *testing.T) {
	l := config.FromMap(map[string]string{"ok": "-42", "plus": "+7", "space": " 1", "hex": "0x10", "float": "1.5",
		"empty": "", "huge": "99999999999999999999999"})
	if v, err := config.Int(l, "ok", 0); v != -42 || err != nil {
		t.Errorf("ok: (%v, %v)", v, err)
	}
	if v, err := config.Int(l, "plus", 0); v != 7 || err != nil {
		t.Errorf("plus: (%v, %v)", v, err)
	}
	if v, err := config.Int(l, "unset", 9); v != 9 || err != nil {
		t.Errorf("unset: (%v, %v)", v, err)
	}
	for key, kind := range map[string]error{"space": strconv.ErrSyntax, "hex": strconv.ErrSyntax, "float": strconv.ErrSyntax,
		"empty": strconv.ErrSyntax, "huge": strconv.ErrRange} {
		v, err := config.Int(l, key, 5)
		if v != 0 || !errors.Is(err, config.ErrParse) || !errors.Is(err, kind) {
			t.Errorf("%s: got (%v, %v), want (0, ErrParse+%v)", key, v, err, kind)
		}
	}
	if v, err := config.Int(nil, "k", 1); v != 1 || err != nil {
		t.Errorf("nil lookup: (%v, %v)", v, err)
	}
}

func TestDuration(t *testing.T) {
	l := config.FromMap(map[string]string{"ok": "250ms", "zero": "0", "bare": "30", "empty": "", "junk": "soon"})
	if v, err := config.Duration(l, "ok", 0); v != 250*time.Millisecond || err != nil {
		t.Errorf("ok: (%v, %v)", v, err)
	}
	if v, err := config.Duration(l, "zero", time.Hour); v != 0 || err != nil {
		t.Errorf("zero: (%v, %v)", v, err)
	}
	if v, err := config.Duration(l, "unset", time.Hour); v != time.Hour || err != nil {
		t.Errorf("unset: (%v, %v)", v, err)
	}
	for _, key := range []string{"bare", "empty", "junk"} {
		v, err := config.Duration(l, key, time.Hour)
		if v != 0 || !errors.Is(err, config.ErrParse) {
			t.Errorf("%s: got (%v, %v), want (0, ErrParse)", key, v, err)
		}
	}
}

func TestBool(t *testing.T) {
	l := config.FromMap(map[string]string{"t": "1", "f": "False", "yes": "yes", "empty": ""})
	if v, err := config.Bool(l, "t", false); !v || err != nil {
		t.Errorf("t: (%v, %v)", v, err)
	}
	if v, err := config.Bool(l, "f", true); v || err != nil {
		t.Errorf("f: (%v, %v)", v, err)
	}
	if v, err := config.Bool(l, "unset", true); !v || err != nil {
		t.Errorf("unset: (%v, %v)", v, err)
	}
	for _, key := range []string{"yes", "empty"} {
		v, err := config.Bool(l, key, true)
		if v || !errors.Is(err, config.ErrParse) || !errors.Is(err, strconv.ErrSyntax) {
			t.Errorf("%s: got (%v, %v), want (false, ErrParse)", key, v, err)
		}
	}
}
