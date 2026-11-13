package config_test

import (
	"fmt"
	"testing"

	"github.com/CrimsonCoder42/toolbench/config"
)

func ExampleFromEnv() {
	// Production passes os.LookupEnv; the example passes a map so it reads no real environment.
	env := envOf(map[string]string{"APP_DB_HOST": "db.internal", "APP_HTTP_READ_TIMEOUT": "5s"})
	l := config.FromEnv(env, "APP_")
	fmt.Println(config.String(l, "db.host", ""))
	d, err := config.Duration(l, "http.read-timeout", 0)
	fmt.Println(d, err)
	// Output:
	// db.internal
	// 5s <nil>
}

func ExampleEnvName() {
	fmt.Println(config.EnvName("db.host"), config.EnvName("http.read-timeout"))
	// Output:
	// DB_HOST HTTP_READ_TIMEOUT
}

func TestFromEnv_Mapping(t *testing.T) {
	l := config.FromEnv(envOf(map[string]string{"X_DB_HOST": "h", "X_A_B_C": "abc", "DB_HOST": "unprefixed"}), "X_")
	cases := []struct {
		key, want string
		ok        bool
	}{
		{"db.host", "h", true},
		{"DB.Host", "h", true},
		{"a.b-c", "abc", true},
		{"a_b_c", "abc", true},
		{"missing", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		if v, ok := l(c.key); v != c.want || ok != c.ok {
			t.Errorf("%q: got (%q, %v), want (%q, %v)", c.key, v, ok, c.want, c.ok)
		}
	}
	if v, _ := config.FromEnv(envOf(map[string]string{"DB_HOST": "unprefixed"}), "")("db.host"); v != "unprefixed" {
		t.Errorf("empty prefix: got %q", v)
	}
}

func TestFromEnv_SetButEmpty(t *testing.T) {
	l := config.FromEnv(envOf(map[string]string{"PORT": ""}), "")
	if v, ok := l("port"); !ok || v != "" {
		t.Errorf("got (%q, %v), want (\"\", true)", v, ok)
	}
}

func TestFromEnv_NilLookup(t *testing.T) {
	if _, ok := config.FromEnv(nil, "APP_")("db.host"); ok {
		t.Error("nil lookup found a key")
	}
}
