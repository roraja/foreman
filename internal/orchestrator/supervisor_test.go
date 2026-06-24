package orchestrator

import (
	"testing"
	"time"
)

func TestDecideRestartHealthyResets(t *testing.T) {
	st := &restartState{retries: 5, nextAttempt: time.Now().Add(time.Hour), gaveUp: true}
	dec := decideRestart(st, tickInput{failed: false, autoRestart: true, now: time.Now()})
	if dec.Restart || dec.GiveUp {
		t.Fatalf("healthy should produce no action, got %+v", dec)
	}
	if st.retries != 0 || !st.nextAttempt.IsZero() || st.gaveUp {
		t.Fatalf("healthy should reset state, got %+v", st)
	}
}

func TestDecideRestartNoAutoRestart(t *testing.T) {
	st := &restartState{}
	dec := decideRestart(st, tickInput{failed: true, autoRestart: false, now: time.Now()})
	if dec.Restart {
		t.Fatal("should not restart when auto_restart is false")
	}
}

func TestDecideRestartBackoff(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	st := &restartState{}
	in := tickInput{failed: true, autoRestart: true, maxRetries: 20, restartDelay: 5 * time.Minute, now: now}

	dec := decideRestart(st, in)
	if !dec.Restart {
		t.Fatal("first failure should restart")
	}
	if st.retries != 1 {
		t.Fatalf("expected retries 1, got %d", st.retries)
	}

	// Immediately after, we are within the backoff window -> no restart.
	in.now = now.Add(1 * time.Minute)
	if dec := decideRestart(st, in); dec.Restart {
		t.Fatal("should not restart within backoff window")
	}
	if st.retries != 1 {
		t.Fatalf("retries should stay 1 during backoff, got %d", st.retries)
	}

	// After the delay elapses, restart again.
	in.now = now.Add(6 * time.Minute)
	if dec := decideRestart(st, in); !dec.Restart {
		t.Fatal("should restart after backoff window")
	}
	if st.retries != 2 {
		t.Fatalf("expected retries 2, got %d", st.retries)
	}
}

func TestDecideRestartGiveUp(t *testing.T) {
	now := time.Now()
	st := &restartState{retries: 3}
	dec := decideRestart(st, tickInput{
		failed: true, autoRestart: true, maxRetries: 3, restartDelay: time.Second, now: now,
	})
	if !dec.GiveUp {
		t.Fatal("should give up once retries reach max_retries")
	}
	if dec.Restart {
		t.Fatal("should not restart when giving up")
	}
}

func TestDecideRestartUnlimited(t *testing.T) {
	now := time.Now()
	st := &restartState{retries: 1000}
	dec := decideRestart(st, tickInput{
		failed: true, autoRestart: true, maxRetries: 0, restartDelay: time.Second,
		now: now.Add(time.Hour),
	})
	if !dec.Restart {
		t.Fatal("max_retries <= 0 should allow unlimited restarts")
	}
}
