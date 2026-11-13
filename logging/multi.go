package logging

import "log/slog"

// Multi returns a handler that sends each record to every one of hs that is enabled for its level: it is
// slog.NewMultiHandler (go1.26) with nil handlers dropped, because the standard library panics on a nil one.
// Each handler gets its own clone of the record; Enabled is true when any handler is enabled; Handle returns
// errors.Join of the handlers' errors; WithAttrs and WithGroup apply to every handler. With no non-nil
// handler it is never enabled. A panic in one handler is not recovered (as in the standard library). It
// replaces an io.MultiWriter under one handler, which forces one format and one level on every output.
//
// ID:          TB-LOG-009
// Pattern:     fan-out (composite handler)
// Authority:   pkg.go.dev log/slog, "func NewMultiHandler" and "type MultiHandler", go1.26
// Errors:      errors.Join of every enabled handler's Handle error
// Blocking:    the handlers are called one after another, in order
// Tags:        logging, slog, fan-out, multi, handler
func Multi(hs ...slog.Handler) slog.Handler {
	keep := make([]slog.Handler, 0, len(hs))
	for _, h := range hs {
		if h != nil {
			keep = append(keep, h)
		}
	}
	return slog.NewMultiHandler(keep...)
}
