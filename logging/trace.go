package logging

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"

	"github.com/CrimsonCoder42/toolbench/clock"
)

// Trace logs one investigation line at Info on l: the message is "[tag][step] msg" and kv are alternating
// key, value pairs, each becoming one attribute, so the house text form prints
//
//	INFO: 2006/01/02 15:04:05.000000 [CART-CHECKOUT-PROBE][B2] cart loaded | items=3 | user_id=absent
//
// tag names the investigation (upper-case-dashed), step is its place in the request flow (B1, B2...);
// neither is changed or invented. Keys are written as given (snake_case by the house rule); a non-string key
// is printed with fmt's %v. An unpaired trailing key gets a nil value, which the text form prints as
// "absent" (JSON: null), so a missing field is visible. A nil l, or l not enabled at Info, logs nothing. The
// values go through l's handler, so New's Redact masks a secret key here as anywhere. The record carries
// the caller's source position (not Trace's), as slog's "Wrapping output methods" shows, and the current
// time; WithClock replaces it in the house handlers.
//
// ID:          TB-LOG-019
// Pattern:     trace logging (tagged investigation line)
// Authority:   pkg.go.dev log/slog, "Wrapping output methods", "func NewRecord" and "type Handler"
// Errors:      none returned: a handler error is dropped, as slog.Logger's output methods drop it
// Blocking:    as long as l's handler: the house handlers wait on their write mutex and the writer
// Tags:        logging, slog, trace, investigation, house-format
func Trace(l *slog.Logger, tag, step, msg string, kv ...any) {
	if l == nil {
		return
	}
	ctx := context.Background()
	if !l.Enabled(ctx, slog.LevelInfo) {
		return
	}
	attrs := make([]slog.Attr, 0, (len(kv)+1)/2)
	for i := 0; i < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok {
			key = fmt.Sprint(kv[i])
		}
		var v any
		if i+1 < len(kv) {
			v = kv[i+1]
		}
		attrs = append(attrs, slog.Any(key, v))
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip runtime.Callers and Trace: the caller's PC
	r := slog.NewRecord(clock.Real().Now(), slog.LevelInfo, "["+tag+"]["+step+"] "+msg, pcs[0])
	r.AddAttrs(attrs...)
	_ = l.Handler().Handle(ctx, r)
}
