// Package clock is the single source of time for the whole backend.
//
// Everything that used to call time.Now() directly now goes through
// clock.Now(). In production that is exactly the system clock — the offset
// below is zero and never changes, so behaviour is unchanged.
//
// The point of the detour is testing. The village runs on time: a task falls
// due after so many days, a request is staggered by an hour, a promise
// expires after a day. A test that waits for any of that is either slow or
// flaky — it stops asserting "the helper gets asked for an overdue task" and
// starts asserting "…within N seconds of wall clock", which depends on how
// busy the machine is and on nothing that matters.
//
// So the backend can be moved through time instead. The offset is applied to
// the real clock rather than freezing it, so time still flows: tickers,
// timeouts and rate limits keep behaving the way they do in production, they
// just start from a different instant.
//
// Because a travelling clock must be *the* clock, this is deliberately
// process-wide state and not a value threaded through every constructor.
// Half a system that travels and half that stands still is worse than no
// time travel at all. Components keep their injectable clock fields for unit
// tests; those fields simply default to clock.Now instead of time.Now.
//
// The knobs that move the offset live in internal/devclock and are only
// mounted when AUTH_MODE=insecure-dev. In production nothing can reach them,
// so the offset stays zero.
package clock

import (
	"sync/atomic"
	"time"
)

// offset is added to the system clock, in nanoseconds. Zero in production.
// Atomic because the HTTP handler that sets it and every reader run on
// different goroutines.
var offset atomic.Int64

// Now is the current time as the backend sees it.
func Now() time.Time {
	if d := offset.Load(); d != 0 {
		return time.Now().Add(time.Duration(d))
	}
	return time.Now()
}

// Offset reports how far the backend's clock is away from the system clock.
func Offset() time.Duration { return time.Duration(offset.Load()) }

// Travelling reports whether the clock has been moved away from the system
// clock. Handy for a warning in the log: nothing in production should ever
// see this true.
func Travelling() bool { return offset.Load() != 0 }

// Set moves the clock so that Now() returns t. It returns the resulting
// offset.
func Set(t time.Time) time.Duration {
	d := t.Sub(time.Now())
	offset.Store(int64(d))
	return d
}

// Advance moves the clock forward by d (backwards for a negative d) and
// returns the resulting offset.
func Advance(d time.Duration) time.Duration {
	return time.Duration(offset.Add(int64(d)))
}

// Reset puts the backend back on the system clock. Every test that travels
// has to come back: the next test would otherwise inherit a village that
// lives in the future.
func Reset() { offset.Store(0) }
