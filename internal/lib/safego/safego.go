// Package safego contains panic guards for goroutines. A panic in any goroutine that is
// not recovered terminates the whole process, and chi's Recoverer only covers the
// request goroutine — so every background goroutine must install its own guard.
package safego

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

// Go runs fn in a new goroutine and logs instead of crashing if it panics.
func Go(log *slog.Logger, name string, fn func()) {
	go func() {
		defer Recover(log, name)
		fn()
	}()
}

// Run calls fn synchronously and converts a panic into a logged error. Use it for one
// iteration of a long-running loop so a single bad run does not stop the loop.
func Run(log *slog.Logger, name string, fn func()) {
	defer Recover(log, name)
	fn()
}

// Recover must be deferred directly (`defer safego.Recover(log, name)`); recover() only
// works when called from the deferred function itself.
func Recover(log *slog.Logger, name string) {
	if r := recover(); r != nil {
		log.Error("recovered from panic",
			slog.String("goroutine", name),
			slog.String("panic", fmt.Sprint(r)),
			slog.String("stack", string(debug.Stack())),
		)
	}
}
