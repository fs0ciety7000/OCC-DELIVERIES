package domain

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Limit allows at most Max events per Window and per key, smoothed with the
// generic cell rate algorithm (GCRA): a burst of Max events is accepted, then
// one event every Window/Max. The whole state of a key is one instant, the
// theoretical arrival time (TAT, unix milliseconds; 0 = fresh key), which the
// server stores in SQLite (app/ratelimit.go mirrors Step in a single atomic
// INSERT … ON CONFLICT DO UPDATE). A refused event does not change the state.
type Limit struct {
	Max    int
	Window time.Duration
}

// IntervalMillis is the emission interval T = Window / Max (milliseconds).
func (l Limit) IntervalMillis() int64 {
	if l.Max <= 0 {
		return l.Window.Milliseconds()
	}
	return l.Window.Milliseconds() / int64(l.Max)
}

// ToleranceMillis is the burst tolerance τ = Window − T (milliseconds).
func (l Limit) ToleranceMillis() int64 {
	return l.Window.Milliseconds() - l.IntervalMillis()
}

// Step records an event at now (unix ms) for a key whose state is tat and
// returns the new state, whether the event is allowed and, when refused, the
// wait before the next allowed event.
func (l Limit) Step(tat, now int64) (next int64, allowed bool, retryAfter time.Duration) {
	t, tau := l.IntervalMillis(), l.ToleranceMillis()
	base := max(tat, now)
	if base-now > tau {
		return tat, false, time.Duration(base-now-tau) * time.Millisecond
	}
	return base + t, true, 0
}

// Refund gives back one event (e.g. the IP budget of a successful login).
// A state in the past is equivalent to a fresh key.
func (l Limit) Refund(tat int64) int64 {
	return tat - l.IntervalMillis()
}

// Expired reports whether a stored state is equivalent to a fresh key (it can
// be deleted).
func Expired(tat, now int64) bool { return tat <= now }

// RetryAfterSeconds is the Retry-After header value (≥ 1 s).
func RetryAfterSeconds(d time.Duration) int {
	s := int(math.Ceil(d.Seconds()))
	return max(s, 1)
}

// TooManyAttemptsMessage is the French 429 message of the sign-in limits.
func TooManyAttemptsMessage(retry time.Duration) string {
	m := max(int(math.Ceil(retry.Minutes())), 1)
	if m == 1 {
		return "Trop de tentatives, réessaie dans 1 minute."
	}
	return fmt.Sprintf("Trop de tentatives, réessaie dans %d minutes.", m)
}

// NormalizeIdentity is the rate limit key of a sign-in identity (e-mail or
// username): trimmed, lower-case.
func NormalizeIdentity(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
