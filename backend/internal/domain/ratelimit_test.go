package domain

import (
	"testing"
	"time"
)

func TestLimitStep(t *testing.T) {
	sec := int64(1000)
	tests := []struct {
		name  string
		limit Limit
		steps []struct {
			at    int64 // ms
			want  bool
			retry time.Duration
		}
	}{
		{"3 par minute : rafale puis une toutes les 20 s", Limit{3, time.Minute}, []struct {
			at    int64
			want  bool
			retry time.Duration
		}{
			{0, true, 0}, {1 * sec, true, 0}, {2 * sec, true, 0},
			{3 * sec, false, 17 * time.Second}, // 4th of the burst
			{19 * sec, false, time.Second},
			{20 * sec, true, 0}, // one emission interval later
			{20 * sec, false, 20 * time.Second},
			{100 * sec, true, 0}, {100 * sec, true, 0}, {100 * sec, true, 0}, // fully replenished
			{100 * sec, false, 20 * time.Second},
		}},
		{"1 par 10 minutes", Limit{1, 10 * time.Minute}, []struct {
			at    int64
			want  bool
			retry time.Duration
		}{
			{0, true, 0},
			{9 * 60 * sec, false, time.Minute},
			{10 * 60 * sec, true, 0},
			{10*60*sec + 1, false, 10*time.Minute - time.Millisecond},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tat int64
			for i, s := range tt.steps {
				next, ok, retry := tt.limit.Step(tat, s.at)
				if ok != s.want || retry != s.retry {
					t.Fatalf("step %d at %dms: got (%v, %v), want (%v, %v)", i, s.at, ok, retry, s.want, s.retry)
				}
				if !ok && next != tat {
					t.Fatalf("step %d: a refused event changed the state", i)
				}
				tat = next
			}
		})
	}
}

func TestLimitBurstAndRefund(t *testing.T) {
	l := Limit{Max: 20, Window: 15 * time.Minute}
	if l.IntervalMillis() != 45000 || l.ToleranceMillis() != 15*60000-45000 {
		t.Fatalf("T=%d τ=%d", l.IntervalMillis(), l.ToleranceMillis())
	}
	now := int64(1_000_000)
	var tat int64
	for i := 0; i < 20; i++ {
		var ok bool
		if tat, ok, _ = l.Step(tat, now); !ok {
			t.Fatalf("attempt %d refused", i)
		}
	}
	if _, ok, retry := l.Step(tat, now); ok || retry != 45*time.Second {
		t.Fatalf("21st: ok=%v retry=%v", ok, retry)
	}
	// a refund gives exactly one attempt back
	tat = l.Refund(tat)
	if next, ok, _ := l.Step(tat, now); !ok {
		t.Fatal("refunded attempt refused")
	} else if _, ok, _ := l.Step(next, now); ok {
		t.Fatal("refund gave more than one attempt")
	}
	if !Expired(now, now) || Expired(now+1, now) {
		t.Fatal("Expired")
	}
}

func TestTooManyAttemptsMessage(t *testing.T) {
	tests := []struct {
		retry time.Duration
		want  string
		after int
	}{
		{0, "Trop de tentatives, réessaie dans 1 minute.", 1},
		{300 * time.Millisecond, "Trop de tentatives, réessaie dans 1 minute.", 1},
		{45 * time.Second, "Trop de tentatives, réessaie dans 1 minute.", 45},
		{61 * time.Second, "Trop de tentatives, réessaie dans 2 minutes.", 61},
		{14*time.Minute + time.Second, "Trop de tentatives, réessaie dans 15 minutes.", 841},
	}
	for _, tt := range tests {
		if got := TooManyAttemptsMessage(tt.retry); got != tt.want {
			t.Errorf("TooManyAttemptsMessage(%v) = %q", tt.retry, got)
		}
		if got := RetryAfterSeconds(tt.retry); got != tt.after {
			t.Errorf("RetryAfterSeconds(%v) = %d, want %d", tt.retry, got, tt.after)
		}
	}
	if NormalizeIdentity("  Alice@Example.ORG ") != "alice@example.org" {
		t.Error("NormalizeIdentity")
	}
}
