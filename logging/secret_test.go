package logging_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"testing"

	"github.com/CrimsonCoder42/toolbench/ctxkey"
	"github.com/CrimsonCoder42/toolbench/logging"
)

const (
	patSecret = "tok_S3cr3tV4lu3x" // matches the value pattern; given through every path
	keySecret = "hunter2-Zq8"      // matches no pattern; given only under secret keys
)

// account carries the token in a field, as go_26's User carried its password.
type account struct {
	Name  string
	Token string
}

// tokenValuer is a LogValuer that returns the secret itself.
type tokenValuer struct{}

func (tokenValuer) LogValue() slog.Value { return slog.StringValue("value is " + patSecret) }

// credsValuer is a LogValuer that returns a group holding a secret key.
type credsValuer struct{}

func (credsValuer) LogValue() slog.Value {
	return slog.GroupValue(slog.String("user", "ada"), slog.String("PassWord", keySecret))
}

// TestSecret_NeverInAnyOutput gives two secrets to the logger through every path a value can take (message,
// keys, groups, WithAttrs, WithGroup, LogValuer results, error strings, wrapped errors, structs, pointers,
// []byte, context values) and searches the full text of every output (JSON, text, the Recorder rendered by
// both stdlib handlers and printed with %+v) for either secret.
func TestSecret_NeverInAnyOutput(t *testing.T) {
	ctx := testCtx(t)
	tokKey := ctxkey.New[string]("tok")
	ctx = tokKey.With(ctx, patSecret)
	deep := patSecret

	var j, x bytes.Buffer
	var rec logging.Recorder
	out := logging.Redact(logging.Multi(jsonTo(&j), textTo(&x), &rec), logging.WithPatterns(regexp.MustCompile(`tok_[A-Za-z0-9]+`)))
	l := slog.New(logging.Context(out, logging.Combine(logging.FromKey("bearer", tokKey), logging.FromKey("api_key", tokKey))))

	l.InfoContext(ctx, "login with "+patSecret,
		"note", "header was "+patSecret,
		"password", keySecret,
		slog.Group("db", slog.Group("conn", "Password", keySecret, "dsn", "postgres://u:"+patSecret+"@h/db")),
		"err", fmt.Errorf("login: %w", errors.New("rejected "+patSecret)),
		"joined", errors.Join(errors.New("a"), errors.New(patSecret)),
		"acct", account{Name: "ada", Token: patSecret},
		"ptr", &struct{ P *string }{&deep},
		"raw", []byte(patSecret),
		"valuer", tokenValuer{},
		"creds", credsValuer{},
	)
	l.With("x-auth-token", keySecret, "x-trace", patSecret).WithGroup("secret").With("k", keySecret).InfoContext(ctx, "m", "v", keySecret)
	l.WithGroup("svc").WarnContext(ctx, "retry", slog.Any("e", errors.New("token "+patSecret+" expired")))

	all := j.String() + x.String() + render(t, &rec) + fmt.Sprintf("%+v %v", rec.Records(), rec.Records())
	if n := len(lines(j.String())); n != 3 {
		t.Fatalf("json lines = %d", n)
	}
	for _, s := range []string{patSecret, keySecret} {
		if strings.Contains(all, s) {
			t.Fatalf("a secret reached an output")
		}
	}
	if strings.Count(j.String(), "[REDACTED]") < 15 {
		t.Fatalf("fewer masks than paths: %d", strings.Count(j.String(), "[REDACTED]"))
	}
}
