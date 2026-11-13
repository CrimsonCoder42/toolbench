package config_test

// The defects of the tutorial repos this package replaces (doc/specs/config.md, "Defects"), one named test
// each. Every test hands the secret to the package and searches every output for it.

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/CrimsonCoder42/toolbench/config"
)

// go_17 read the database DSN with os.Getenv, so an unset variable and an empty one were the same empty
// string, and the godotenv load error was ignored. Here unset and empty are distinct, Required reports
// both, and a missing file is an error the caller sees.
func TestDefect_go17_GetenvUnsetLooksEmpty(t *testing.T) {
	l := config.FromEnv(envOf(map[string]string{"DB_CONN_STR": ""}), "")
	if _, ok := l("db.conn-str"); !ok {
		t.Error("an empty variable is reported unset")
	}
	if _, ok := l("db.other"); ok {
		t.Error("an unset variable is reported set")
	}
	err := config.Required(l, "db.conn-str", "db.other")
	if !errors.Is(err, config.ErrMissing) || !strings.Contains(errText(err), "db.conn-str") || !strings.Contains(errText(err), "db.other") {
		t.Errorf("Required: %s", masked(errText(err)))
	}
}

func TestDefect_go17_LoadErrorIgnored(t *testing.T) {
	_, err := config.FromJSONFile(fstest.MapFS{}, "app.json")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing config file returned %v, want an error wrapping fs.ErrNotExist", err)
	}
}

// go_31 kept the config in a package global filled by ReadConfig and panicked on a bad file. Here two
// configurations live side by side in one process, and a bad file is an error.
func TestDefect_go31_GlobalConfig(t *testing.T) {
	a := config.FromMap(map[string]string{"db": "a"})
	b := config.FromMap(map[string]string{"db": "b"})
	if config.String(a, "db", "") != "a" || config.String(b, "db", "") != "b" {
		t.Error("two configurations interfere")
	}
}

func TestDefect_go31_PanicOnBadFile(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %s", masked(fmt.Sprint(r)))
		}
	}()
	fsys := fstest.MapFS{"config.json": {Data: []byte(`{"db": "x", "fb_oauth": {"client_secret": "` + secret + `" "x": 1}}`)}}
	_, err := config.FromJSONFile(fsys, "config.json")
	if !errors.Is(err, config.ErrJSON) {
		t.Errorf("got %s, want ErrJSON", masked(errText(err)))
	}
	noSecret(t, "FromJSONFile error", errText(err))
}

// go_72 called log.Fatal inside LoadConfig (the process exits from a library) and ignored the error of
// viper.Unmarshal. Here a load failure and a malformed value are returned errors.
func TestDefect_go72_LogFatalInLoader(t *testing.T) {
	_, err := config.FromJSONFile(fstest.MapFS{"config/config.json": {Data: []byte(`not json`)}}, "config/config.json")
	if !errors.Is(err, config.ErrJSON) {
		t.Errorf("got %v", err)
	}
}

func TestDefect_go72_UnmarshalErrorIgnored(t *testing.T) {
	l := config.FromMap(map[string]string{"server.port": "80a"})
	if _, err := config.Int(l, "server.port", 8080); !errors.Is(err, config.ErrParse) {
		t.Errorf("got %v, want ErrParse", err)
	}
}

// go_72 concatenated the database password into a DSN string and go_31 committed client secrets: a secret
// that reaches configuration must not reach any output. Each getter, Required, FromJSON and Values is given
// the secret; every message and printed form is searched.
func TestDefect_go72_SecretLogged(t *testing.T) {
	l := config.Layer(
		config.FromMap(map[string]string{"db.port": secret, "db.timeout": secret, "db.tls": secret}),
		config.FromEnv(envOf(map[string]string{"APP_DB_PASSWORD": secret, "APP_DB_DSN": "app:" + secret + "@tcp(db)/x"}), "APP_"),
	)
	var outs []string
	_, err := config.Int(l, "db.port", 0)
	outs = append(outs, errText(err), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err))
	_, err = config.Duration(l, "db.timeout", 0)
	outs = append(outs, errText(err), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err))
	_, err = config.Bool(l, "db.tls", false)
	outs = append(outs, errText(err), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err))
	_, err = config.Int(l, "db.password", 0)
	outs = append(outs, errText(err), fmt.Sprintf("%#v", err))
	outs = append(outs, errText(config.Required(l, "db.password", "db.user")))
	for i, in := range []string{
		`{"a":["` + secret + `,x"]}`, `{"a":"` + secret + `" "b":1}`, `{"a":1,"a":"` + secret + `"}`,
		`["` + secret + `"]`, `{"a":"b"} "` + secret + `"`, `{"a":` + secret + `}`,
	} {
		_, err := config.FromJSON(strings.NewReader(in))
		if err == nil {
			t.Errorf("no error for input %d (%d bytes)", i, len(in))
		}
		outs = append(outs, errText(err), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err))
	}
	v := config.Collect(l, []string{"db.password", "db.dsn"})
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("startup", "config", v, "err", err)
	outs = append(outs, v.String(), fmt.Sprintf("%v|%+v|%#v", v, v, v), buf.String())
	for i, out := range outs {
		noSecret(t, fmt.Sprintf("output %d", i), out)
	}
}
