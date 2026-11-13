package config_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/CrimsonCoder42/toolbench/config"
)

func ExampleCollect() {
	l := config.FromMap(map[string]string{"db.host": "db.internal", "db.password": "hunter2", "db.port": "5432"})
	v := config.Collect(l, []string{"db.host", "db.port", "db.password", "db.user"})
	fmt.Println(v)
	fmt.Printf("%#v\n", v)
	// Output:
	// db.host="db.internal" db.port="5432" db.password=[REDACTED] db.user=(unset)
	// config.Values{"db.host": "db.internal", "db.port": "5432", "db.password": "[REDACTED]", "db.user": "(unset)"}
}

func ExampleValues() {
	l := config.FromMap(map[string]string{"api.token": "t-123", "region": "eu"})
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{ReplaceAttr: dropTime}))
	log.Info("config", "values", config.Collect(l, []string{"region", "api.token"}))
	fmt.Print(buf.String())
	// Output:
	// level=INFO msg=config values.region=eu values.api.token=[REDACTED]
}

func ExampleWithSecrets() {
	l := config.FromMap(map[string]string{"database.url": "postgres://app:pw@db/app"})
	fmt.Println(config.Collect(l, []string{"database.url"}, config.WithSecrets("Database.URL")))
	// Output:
	// database.url=[REDACTED]
}

func dropTime(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return a
}

// TestValues_NoSecretInAnyOutput hands the secret to Collect under default-secret keys and a WithSecrets
// key, then searches every printed form for it.
func TestValues_NoSecretInAnyOutput(t *testing.T) {
	l := config.FromMap(map[string]string{
		"db.password": secret, "API_KEY": secret, "auth.token": secret, "stripe.secret": secret,
		"db.dsn": "user:" + secret + "@tcp(db)/app", "tls.private_pem": secret, "aws.credentials": secret,
		"database.url": "postgres://u:" + secret + "@h/db", "region": "eu",
	})
	keys := []string{"db.password", "API_KEY", "auth.token", "stripe.secret", "db.dsn", "tls.private_pem",
		"aws.credentials", "database.url", "region"}
	v := config.Collect(l, keys, config.WithSecrets("database.url"))

	var text, js bytes.Buffer
	slog.New(slog.NewTextHandler(&text, nil)).Info("cfg", "v", v)
	slog.New(slog.NewJSONHandler(&js, nil)).Info("cfg", "v", v, slog.Any("ptr", &v))
	outputs := map[string]string{
		"String": v.String(), "GoString": v.GoString(),
		"%v": fmt.Sprintf("%v", v), "%+v": fmt.Sprintf("%+v", v), "%#v": fmt.Sprintf("%#v", v),
		"%s": fmt.Sprintf("%s", v), "%q": fmt.Sprintf("%q", v), "%x": fmt.Sprintf("%x", v), "%d": fmt.Sprintf("%d", v),
		"&v %+v": fmt.Sprintf("%+v", &v), "&v %#v": fmt.Sprintf("%#v", &v),
		"[]Values %#v": fmt.Sprintf("%#v", []config.Values{v}), "struct %+v": fmt.Sprintf("%+v", struct{ C config.Values }{v}),
		"Println": fmt.Sprintln(v), "slog text": text.String(), "slog json": js.String(),
	}
	for what, out := range outputs {
		noSecret(t, what, out)
	}
	if !strings.Contains(v.String(), `region="eu"`) {
		t.Errorf("a non-secret key is not shown: %s", masked(v.String()))
	}
	if n := strings.Count(v.String(), "[REDACTED]"); n != len(keys)-1 {
		t.Errorf("got %d redactions, want %d: %s", n, len(keys)-1, masked(v.String()))
	}
}

func TestValues_ZeroAndNil(t *testing.T) {
	var v config.Values
	if s := v.String(); s != "" {
		t.Errorf("zero Values prints %q", s)
	}
	if s := fmt.Sprintf("%#v", v); s != "config.Values{}" {
		t.Errorf("zero %%#v: %q", s)
	}
	got := config.Collect(nil, []string{"a"}, nil).String()
	if got != "a=(unset)" {
		t.Errorf("nil lookup: %q", got)
	}
}

func TestValues_QuotesValues(t *testing.T) {
	v := config.Collect(config.FromMap(map[string]string{"motd": "a b=c\n"}), []string{"motd"})
	if got, want := v.String(), `motd="a b=c\n"`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestValues_SharedDefaultWords(t *testing.T) {
	keys := []string{"Authorization", "session_cookie", "gpg.passphrase", "db.pass_word", "http.Set-Cookie"}
	m := map[string]string{}
	for _, k := range keys {
		m[k] = secret
	}
	v := config.Collect(config.FromMap(m), keys)
	noSecret(t, "String", v.String())
	noSecret(t, "%#v", fmt.Sprintf("%#v", v))
	if n := strings.Count(v.String(), "[REDACTED]"); n != len(keys) {
		t.Errorf("got %d redactions, want %d: %s", n, len(keys), masked(v.String()))
	}
}
