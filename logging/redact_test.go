package logging_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"testing/slogtest"

	"github.com/CrimsonCoder42/toolbench/logging"
)

func TestRedact_Slogtest(t *testing.T) {
	var buf *bytes.Buffer
	slogtest.Run(t, func(*testing.T) slog.Handler {
		buf = new(bytes.Buffer)
		return logging.Redact(slog.NewJSONHandler(buf, nil), logging.WithPatterns(regexp.MustCompile(`never-matches-\d{40}`)))
	}, func(t *testing.T) map[string]any { return decodeOne(t, buf) })
}

// TestRedact_SameAsUnwrappedWithoutSecrets: with nothing to mask, the wrapper is invisible, including a
// WithGroup/With chain (the guide's ordering example).
func TestRedact_SameAsUnwrappedWithoutSecrets(t *testing.T) {
	var a, b bytes.Buffer
	run := func(h slog.Handler) {
		l := slog.New(h).WithGroup("g1").With("k1", 1).WithGroup("g2").With("k2", 2)
		l.Info("m", "k3", 3, slog.Group("g3", "k4", 4), slog.Group("empty"), "", "inline?")
		l.WithGroup("").Debug("d", slog.Attr{})
	}
	run(jsonTo(&a))
	run(logging.Redact(jsonTo(&b)))
	if a.String() != b.String() {
		t.Fatalf("redact changed clean output: plain %s, redact %s", withheld(a.String()), withheld(b.String()))
	}
}

func TestRedact_KeysCaseAndSeparatorsAtEveryDepth(t *testing.T) {
	const secret = "hunter2-Gx91"
	var buf bytes.Buffer
	l := slog.New(logging.Redact(jsonTo(&buf), logging.WithKeys("db-pass")))
	l.Info("m",
		"PASSWORD", secret,
		"Api-Key", secret,
		"access_token", secret,
		"DB_PASS", secret,
		slog.Group("a", slog.Group("b", "Password", secret, "ok", "visible")),
	)
	l.With("x-auth-token", secret).WithGroup("x").With("client.secret", secret).Info("m2")
	out := buf.String()
	if strings.Contains(out, secret) {
		t.Fatalf("secret leaked: %s", withheld(out))
	}
	if !strings.Contains(out, `"ok":"visible"`) {
		t.Fatalf("non-secret sibling lost: %s", withheld(out))
	}
	if got := strings.Count(out, "[REDACTED]"); got != 7 {
		t.Fatalf("masks = %d, want 7: %s", got, withheld(out))
	}
}

func TestRedact_AuthorizationAndCookieMaskedByDefault(t *testing.T) {
	var buf bytes.Buffer
	slog.New(logging.Redact(jsonTo(&buf))).Info("m", "Authorization", "Bearer zz9", "cookie", "sid=zz9", "Set-Cookie", "sid=zz9")
	m := decodeOne(t, &buf)
	if m["Authorization"] != "[REDACTED]" || m["cookie"] != "[REDACTED]" || m["Set-Cookie"] != "[REDACTED]" {
		t.Fatalf("not masked by default: %s", withheld(buf.String()))
	}
}

// TestRedact_WithKeysExactDefaultWordsContained: a WithKeys key matches exactly; a default word matches as a
// substring of the normalised key (over-matching accepted, as in config).
func TestRedact_WithKeysExactDefaultWordsContained(t *testing.T) {
	var buf bytes.Buffer
	slog.New(logging.Redact(jsonTo(&buf), logging.WithKeys("ssn"))).Info("m", "ssn_hint", "last four", "token_count", 12, "user", "ada")
	m := decodeOne(t, &buf)
	if m["ssn_hint"] != "last four" || m["user"] != "ada" || m["token_count"] != "[REDACTED]" {
		t.Fatalf("matching wrong: %s", withheld(buf.String()))
	}
}

func TestRedact_GroupKeyMasksWholeGroup(t *testing.T) {
	var buf bytes.Buffer
	slog.New(logging.Redact(jsonTo(&buf))).Info("m", slog.Group("password", "old", "a1", "new", "b2"))
	m := decodeOne(t, &buf)
	if m["password"] != "[REDACTED]" {
		t.Fatalf("group not masked: %s", withheld(buf.String()))
	}
}

func TestRedact_WithGroupSecretNameMasksEverythingUnder(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(logging.Redact(jsonTo(&buf))).WithGroup("account").With("user", "ada").WithGroup("Secret")
	l.Info("m", "v", "zz-9")
	out := buf.String()
	if strings.Contains(out, "zz-9") {
		t.Fatalf("value under secret group leaked: %s", withheld(out))
	}
	if !strings.Contains(out, `"user":"ada"`) {
		t.Fatalf("user under account should show: %s", withheld(out))
	}
}

// stringer is a fmt.Stringer whose text carries a token.
type stringer struct{}

func (stringer) String() string { return "conn using tok_STRINGER1" }

// nested carries a token two pointers deep, where %+v prints only an address but json.Marshal follows it.
type nested struct{ Inner *inner }
type inner struct{ Token *string }

// leakyValuer is a LogValuer whose resolved value carries a token under a harmless key.
type leakyValuer struct{}

func (leakyValuer) LogValue() slog.Value {
	return slog.GroupValue(slog.String("note", "bearer tok_VALUER1"), slog.String("id", "u-1"))
}

func TestRedact_PatternsEverywhere(t *testing.T) {
	re := regexp.MustCompile(`tok_[A-Za-z0-9]+`)
	deep := "tok_DEEPJSON1"
	cases := []struct {
		name string
		log  func(*slog.Logger)
	}{
		{"message", func(l *slog.Logger) { l.Info("got tok_MSG1 from header") }},
		{"string", func(l *slog.Logger) { l.Info("m", "header", "Bearer tok_STR1") }},
		{"error", func(l *slog.Logger) { l.Info("m", "err", fmt.Errorf("auth: %w", errors.New("bad tok_ERR1"))) }},
		{"stringer", func(l *slog.Logger) { l.Info("m", "conn", stringer{}) }},
		{"bytes", func(l *slog.Logger) { l.Info("m", "raw", []byte("tok_BYTES1")) }},
		{"json-deep", func(l *slog.Logger) { l.Info("m", "v", nested{Inner: &inner{Token: &deep}}) }},
		{"logvaluer", func(l *slog.Logger) { l.Info("m", "user", leakyValuer{}) }},
		{"key", func(l *slog.Logger) { l.Info("m", "tok_KEY1", 1) }},
		{"with-attrs", func(l *slog.Logger) { l.With("h", "tok_WITH1").Info("m") }},
		{"group-name", func(l *slog.Logger) { l.WithGroup("tok_GROUP1").Info("m", "a", 1) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var j, x bytes.Buffer
			var rec logging.Recorder
			l := slog.New(logging.Redact(logging.Multi(jsonTo(&j), textTo(&x), &rec), logging.WithPatterns(re)))
			c.log(l)
			out := j.String() + x.String() + render(t, &rec)
			if re.MatchString(out) {
				t.Fatalf("token leaked: %s", withheld(out))
			}
			if !strings.Contains(out, "[REDACTED]") {
				t.Fatalf("no mask written: %s", withheld(out))
			}
		})
	}
}

func TestRedact_PatternMissKeepsValueType(t *testing.T) {
	var rec logging.Recorder
	l := slog.New(logging.Redact(&rec, logging.WithPatterns(regexp.MustCompile(`tok_\w+`))))
	err := errors.New("plain failure")
	l.Info("m", "err", err)
	rs := rec.Records()
	var got any
	rs[0].Attrs(func(a slog.Attr) bool { got = a.Value.Any(); return true })
	if got != err {
		t.Fatalf("value replaced although nothing matched: %T", got)
	}
}

// panicMarshaler panics in MarshalText; Redact must survive it.
type panicMarshaler struct{}

func (panicMarshaler) MarshalText() ([]byte, error) { panic("boom") }

func TestRedact_PanickingMarshalerDoesNotCrash(t *testing.T) {
	var rec logging.Recorder
	l := slog.New(logging.Redact(&rec, logging.WithPatterns(regexp.MustCompile(`x`))))
	l.Info("m", "v", panicMarshaler{})
	if n := len(rec.Records()); n != 1 {
		t.Fatalf("records = %d", n)
	}
}

func TestRedact_MaskLiteralNotTemplate(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(logging.Redact(jsonTo(&buf), logging.WithMask("$1"), logging.WithPatterns(regexp.MustCompile(`(tok)_\w+`))))
	l.Info("m", "h", "tok_A1")
	if m := decodeOne(t, &buf); m["h"] != "$1" {
		t.Fatalf("mask expanded: %s", withheld(buf.String()))
	}
}

func TestRedact_NilInputs(t *testing.T) {
	h := logging.Redact(nil, nil, logging.WithPatterns(nil), logging.WithKeys(""))
	if h.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("nil handler should be a discard handler")
	}
	slog.New(h).Info("m", "password", "x") // must not panic
}

func TestRedact_EnabledDelegates(t *testing.T) {
	var lv slog.LevelVar
	lv.Set(slog.LevelWarn)
	var buf bytes.Buffer
	h := logging.Redact(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: &lv}))
	ctx := context.Background()
	if h.Enabled(ctx, slog.LevelInfo) || !h.Enabled(ctx, slog.LevelWarn) {
		t.Fatal("Enabled not delegated")
	}
	lv.Set(slog.LevelDebug)
	if !h.WithGroup("g").WithAttrs([]slog.Attr{slog.Int("a", 1)}).Enabled(ctx, slog.LevelDebug) {
		t.Fatal("level change not seen through WithGroup/WithAttrs")
	}
}

func TestRedact_CallerRecordUnchanged(t *testing.T) {
	var rec logging.Recorder
	h := logging.Redact(&rec, logging.WithPatterns(regexp.MustCompile(`tok_\w+`)))
	r := slog.NewRecord(fixed, slog.LevelInfo, "msg tok_A", 0)
	r.AddAttrs(slog.String("password", "p"), slog.String("h", "tok_B"))
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if r.Message != "msg tok_A" {
		t.Fatalf("message changed: %s", withheld(r.Message))
	}
	var vals []string
	r.Attrs(func(a slog.Attr) bool { vals = append(vals, a.Value.String()); return true })
	if strings.Join(vals, ",") != "p,tok_B" {
		t.Fatalf("caller's attrs changed: %d values", len(vals))
	}
	if got := rec.Records()[0].Time; !got.Equal(fixed) {
		t.Fatalf("time = %v", got)
	}
}

func TestRedact_WithAttrsDoesNotModifyReceiver(t *testing.T) {
	var buf bytes.Buffer
	base := logging.Redact(jsonTo(&buf))
	_ = base.WithAttrs([]slog.Attr{slog.String("a", "1")})
	_ = base.WithGroup("g")
	slog.New(base).Info("m")
	if got := decodeOne(t, &buf); len(got) != 2 {
		t.Fatalf("receiver changed: %s", withheld(buf.String()))
	}
}

// TestDefect_go26_ValueCarriesSecretPastKeyMask reproduces go_26_slog's key-only ReplaceAttr: the secret goes
// out under any other key, inside an error, inside a LogValuer result and inside a group named "password".
// Redact with a value pattern masks all of them.
func TestDefect_go26_ValueCarriesSecretPastKeyMask(t *testing.T) {
	const secret = "G0r0ck$-tok_GO26"
	keyOnly := func(_ []string, a slog.Attr) slog.Attr {
		if strings.ToLower(a.Key) == "password" {
			a.Value = slog.StringValue("<<MASKED>>")
		}
		return a
	}
	logAll := func(l *slog.Logger) {
		l.Info("m",
			"password", secret,
			"note", "pw is "+secret,
			"err", errors.New("login failed with "+secret),
			"user", leakyValuer{},
			slog.Group("Password", "v", secret),
		)
	}
	var leaky bytes.Buffer
	logAll(slog.New(slog.NewJSONHandler(&leaky, &slog.HandlerOptions{ReplaceAttr: keyOnly})))
	if !strings.Contains(leaky.String(), secret) {
		t.Fatal("the go_26 defect did not reproduce")
	}
	var fixedOut bytes.Buffer
	re := regexp.MustCompile(regexp.QuoteMeta(secret))
	logAll(slog.New(logging.Redact(jsonTo(&fixedOut), logging.WithPatterns(re))))
	if strings.Contains(fixedOut.String(), secret) {
		t.Fatalf("Redact leaked: %s", withheld(fixedOut.String()))
	}
}

func BenchmarkRedact(b *testing.B) {
	h := logging.Redact(slog.DiscardHandler, logging.WithPatterns(regexp.MustCompile(`tok_\w+`)))
	// DiscardHandler is never enabled, so call Handle directly.
	r := slog.NewRecord(fixed, slog.LevelInfo, "m", 0)
	r.AddAttrs(slog.String("user", "ada"), slog.String("password", "p"), slog.Any("err", errors.New("x")))
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		_ = h.Handle(ctx, r)
	}
}
