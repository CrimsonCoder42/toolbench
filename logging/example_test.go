package logging_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/ctxkey"
	"github.com/CrimsonCoder42/toolbench/logging"
)

// exampleSecrets are the secret values the Examples pass in; TestExamples_NoSecretInOutput checks that none
// of them appears in any // Output: block of this file.
var exampleSecrets = []string{"hunter2", "078-05-1120", "tok_Q7xZ9pL2mW4v", "s3cr3t-value", "4921"}

func ExampleRedact() {
	var buf bytes.Buffer
	log := slog.New(logging.Redact(jsonTo(&buf)))
	log.Info("login", "user", "ada", "password", "hunter2", slog.Group("db", "Password", "hunter2"))
	fmt.Print(buf.String())
	// Output:
	// {"level":"INFO","msg":"login","user":"ada","password":"[REDACTED]","db":{"Password":"[REDACTED]"}}
}

func ExampleWithKeys() {
	var buf bytes.Buffer
	log := slog.New(logging.Redact(jsonTo(&buf), logging.WithKeys("ssn")))
	log.Info("applicant", "SSN", "078-05-1120", "name", "Ada")
	fmt.Print(buf.String())
	// Output:
	// {"level":"INFO","msg":"applicant","SSN":"[REDACTED]","name":"Ada"}
}

func ExampleWithPatterns() {
	var buf bytes.Buffer
	tokens := regexp.MustCompile(`tok_[A-Za-z0-9]+`)
	log := slog.New(logging.Redact(jsonTo(&buf), logging.WithPatterns(tokens)))
	log.Error("call failed", "err", errors.New("upstream rejected tok_Q7xZ9pL2mW4v"))
	fmt.Print(buf.String())
	// Output:
	// {"level":"ERROR","msg":"call failed","err":"upstream rejected [REDACTED]"}
}

func ExampleWithMask() {
	var buf bytes.Buffer
	log := slog.New(logging.Redact(jsonTo(&buf), logging.WithMask("***")))
	log.Info("auth", "token", "s3cr3t-value")
	fmt.Print(buf.String())
	// Output:
	// {"level":"INFO","msg":"auth","token":"***"}
}

func ExampleRedactOption() {
	var buf bytes.Buffer
	opts := []logging.RedactOption{logging.WithKeys("pin"), logging.WithMask("<hidden>")}
	log := slog.New(logging.Redact(jsonTo(&buf), opts...))
	log.Info("card", "PIN", "4921", "last4", "0005")
	fmt.Print(buf.String())
	// Output:
	// {"level":"INFO","msg":"card","PIN":"<hidden>","last4":"0005"}
}

func ExampleContext() {
	var buf bytes.Buffer
	requestID := ctxkey.New[string]("request-id")
	h := logging.Context(jsonTo(&buf), logging.FromKey("request_id", requestID))
	log := slog.New(h).WithGroup("http")
	ctx := requestID.With(context.Background(), "r-42")
	log.InfoContext(ctx, "served", "status", 200)
	fmt.Print(buf.String())
	// Output:
	// {"level":"INFO","msg":"served","request_id":"r-42","http":{"status":200}}
}

func ExampleFromKey() {
	var buf bytes.Buffer
	userID := ctxkey.New[int]("user-id")
	log := slog.New(logging.Context(jsonTo(&buf), logging.FromKey("user_id", userID)))
	log.InfoContext(userID.With(context.Background(), 7), "with user")
	log.InfoContext(context.Background(), "without user")
	fmt.Print(buf.String())
	// Output:
	// {"level":"INFO","msg":"with user","user_id":7}
	// {"level":"INFO","msg":"without user"}
}

func ExampleCombine() {
	var buf bytes.Buffer
	requestID := ctxkey.New[string]("request-id")
	tenant := ctxkey.New[string]("tenant")
	extract := logging.Combine(logging.FromKey("request_id", requestID), logging.FromKey("tenant", tenant))
	log := slog.New(logging.Context(jsonTo(&buf), extract))
	ctx := tenant.With(requestID.With(context.Background(), "r-1"), "acme")
	log.InfoContext(ctx, "ok")
	fmt.Print(buf.String())
	// Output:
	// {"level":"INFO","msg":"ok","request_id":"r-1","tenant":"acme"}
}

func ExampleMulti() {
	var all, warn bytes.Buffer
	text := slog.NewTextHandler(&warn, &slog.HandlerOptions{Level: slog.LevelWarn, ReplaceAttr: noTime})
	log := slog.New(logging.Multi(jsonTo(&all), text))
	log.Info("started")
	log.Warn("slow", "ms", 900)
	fmt.Print(all.String())
	fmt.Print(warn.String())
	// Output:
	// {"level":"INFO","msg":"started"}
	// {"level":"WARN","msg":"slow","ms":900}
	// level=WARN msg=slow ms=900
}

func ExampleRecorder() {
	var rec logging.Recorder
	log := slog.New(&rec).With("svc", "api").WithGroup("req")
	log.Info("hit", "path", "/x")
	var buf bytes.Buffer
	for _, r := range rec.Records() {
		_ = jsonTo(&buf).Handle(context.Background(), r)
	}
	fmt.Print(buf.String())
	// Output:
	// {"level":"INFO","msg":"hit","svc":"api","req":{"path":"/x"}}
}

// The house-format Examples use a fake clock so their output is exact (houseTime: 2026-11-13 09:04:05.678901 UTC).

func ExampleText() {
	var buf bytes.Buffer
	log := slog.New(logging.Text(&buf, logging.WithClock(clock.NewFake(houseTime))))
	log.Info("ListAuthUsers SUCCESS", "module", "AUTH_USER", "tenant_id", "e1c6acea", "active", 0)
	log.Info("cart loaded", "items", 3, "user_id", "", slog.Group("db", "rows", 2))
	fmt.Print(buf.String())
	// Output:
	// INFO: 2026/11/13 09:04:05.678901 AUTH_USER: ListAuthUsers SUCCESS | tenant_id=e1c6acea | active=0
	// INFO: 2026/11/13 09:04:05.678901 cart loaded | items=3 | user_id=absent | db.rows=2
}

func ExampleTextOption() {
	var buf bytes.Buffer
	opts := []logging.TextOption{
		logging.WithClock(clock.NewFake(houseTime)),
		logging.WithLevel(slog.LevelDebug),
		logging.WithTimeFormat("15:04:05"),
	}
	slog.New(logging.Text(&buf, opts...)).Debug("cache warm", "module", "CACHE", "keys", 120)
	fmt.Print(buf.String())
	// Output:
	// DEBUG: 09:04:05 CACHE: cache warm | keys=120
}

func ExampleWithLevel() {
	var buf bytes.Buffer
	var level slog.LevelVar // Info
	log := slog.New(logging.Text(&buf, logging.WithLevel(&level), logging.WithTimeFormat("")))
	log.Debug("hidden")
	level.Set(slog.LevelDebug)
	log.Debug("shown")
	fmt.Print(buf.String())
	// Output:
	// DEBUG: shown
}

func ExampleWithColor() {
	var buf bytes.Buffer
	slog.New(logging.Text(&buf, logging.WithColor(true), logging.WithTimeFormat(""))).Info("ready")
	fmt.Printf("%q\n", buf.String())
	// Output:
	// "\x1b[34mINFO: \x1b[0mready\n"
}

func ExampleWithSource() {
	var buf bytes.Buffer
	off := logging.WithSource(slog.LevelError + 4) // above ERROR: never
	slog.New(logging.Text(&buf, off, logging.WithClock(clock.NewFake(houseTime)))).Error("write failed", "module", "FEED")
	on := slog.New(logging.Text(&buf, logging.WithSource(slog.LevelInfo), logging.WithTimeFormat("")))
	on.Info("with source")
	fmt.Print(buf.String()[:strings.Index(buf.String(), "\n")+1])
	fmt.Println(strings.HasPrefix(lines(buf.String())[1], "INFO: example_test.go:"))
	// Output:
	// ERROR: 2026/11/13 09:04:05.678901 FEED: write failed
	// true
}

func ExampleWithTimeFormat() {
	var buf bytes.Buffer
	slog.New(logging.Text(&buf, logging.WithClock(clock.NewFake(houseTime)), logging.WithTimeFormat(time.RFC3339))).Info("m")
	slog.New(logging.Text(&buf, logging.WithTimeFormat(""))).Info("no time")
	fmt.Print(buf.String())
	// Output:
	// INFO: 2026-11-13T09:04:05Z m
	// INFO: no time
}

func ExampleWithClock() {
	var buf bytes.Buffer
	c := clock.NewFake(houseTime)
	log := slog.New(logging.Text(&buf, logging.WithClock(c)))
	log.Info("first")
	c.Advance(1500 * time.Microsecond)
	log.Info("second")
	fmt.Print(buf.String())
	// Output:
	// INFO: 2026/11/13 09:04:05.678901 first
	// INFO: 2026/11/13 09:04:05.680401 second
}

func ExampleWithJSON() {
	var buf bytes.Buffer
	log := logging.New(&buf, slog.LevelInfo, logging.WithJSON(true), logging.WithClock(clock.NewFake(houseTime)))
	log.Info("login", "module", "AUTH", "user_id", "u1", "password", "hunter2")
	fmt.Print(buf.String())
	// Output:
	// {"time":"2026-11-13T09:04:05.678901Z","level":"INFO","msg":"login","module":"AUTH","user_id":"u1","password":"[REDACTED]"}
}

func ExampleTrace() {
	var buf bytes.Buffer
	log := slog.New(logging.Text(&buf, logging.WithClock(clock.NewFake(houseTime))))
	logging.Trace(log, "CART-CHECKOUT-PROBE", "B2", "cart loaded", "items", 3, "user_id", "", "tenant_id")
	fmt.Print(buf.String())
	// Output:
	// INFO: 2026/11/13 09:04:05.678901 [CART-CHECKOUT-PROBE][B2] cart loaded | items=3 | user_id=absent | tenant_id=absent
}

func ExampleParseLevel() {
	for _, s := range []string{"debug", "WARNING", "error", "verbose"} {
		fmt.Println(s, "->", logging.ParseLevel(s))
	}
	// Output:
	// debug -> DEBUG
	// WARNING -> WARN
	// error -> ERROR
	// verbose -> INFO
}

func ExampleNew() {
	var buf bytes.Buffer // os.Stdout in a program: logging.New(os.Stdout, logging.ParseLevel(level))
	log := logging.New(&buf, slog.LevelInfo, logging.WithClock(clock.NewFake(houseTime)))
	log.Info("login", "module", "AUTH", "user_id", "u1", "password", "hunter2")
	log.Error("refresh failed", "module", "AUTH", "err", errors.New("token expired"), "user_id", "u1")
	fmt.Print(strings.Split(buf.String(), "\n")[0], "\n")
	fmt.Println(strings.HasSuffix(lines(buf.String())[1], " AUTH: refresh failed | user_id=u1 | err=token expired"))
	// Output:
	// INFO: 2026/11/13 09:04:05.678901 AUTH: login | user_id=u1 | password=[REDACTED]
	// true
}
