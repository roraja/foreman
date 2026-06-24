package orchestrator

import (
	"context"
	"log"
	"time"

	"github.com/anthropic/foreman/internal/process"
	"github.com/anthropic/foreman/internal/types"
)

// restartState tracks per-service supervision state across ticks.
type restartState struct {
	retries     int
	nextAttempt time.Time // earliest time a restart may be attempted
	lastCheck   time.Time // last health probe time
	gaveUp      bool
}

// tickInput captures everything decideRestart needs to make a decision.
type tickInput struct {
	failed       bool // process crashed or failed its health check
	autoRestart  bool
	maxRetries   int // <= 0 means unlimited
	restartDelay time.Duration
	now          time.Time
}

// tickDecision is the outcome of decideRestart.
type tickDecision struct {
	Restart bool
	GiveUp  bool
}

// decideRestart is the pure supervision state machine. It mutates st's retry
// counters and returns whether to restart or give up. Keeping it pure makes
// the backoff/retry behavior unit-testable without real processes.
func decideRestart(st *restartState, in tickInput) tickDecision {
	if !in.failed {
		// Healthy: clear the retry budget so future failures get a fresh start.
		st.retries = 0
		st.nextAttempt = time.Time{}
		st.gaveUp = false
		return tickDecision{}
	}
	if !in.autoRestart {
		return tickDecision{}
	}
	if in.maxRetries > 0 && st.retries >= in.maxRetries {
		return tickDecision{GiveUp: true}
	}
	if in.now.Before(st.nextAttempt) {
		return tickDecision{}
	}
	st.retries++
	st.nextAttempt = in.now.Add(in.restartDelay)
	return tickDecision{Restart: true}
}

// StartSupervisor launches the background supervision loop. It restarts
// processes that crash or fail their port health check, honoring each
// service's auto_restart, restart_delay and max_retries settings.
func (o *Orchestrator) StartSupervisor(ctx context.Context) {
	go o.superviseLoop(ctx)
}

func (o *Orchestrator) superviseLoop(ctx context.Context) {
	const baseTick = 5 * time.Second
	ticker := time.NewTicker(baseTick)
	defer ticker.Stop()

	states := make(map[string]*restartState)
	log.Printf("supervisor started (base tick %s)", baseTick)

	for {
		select {
		case <-ctx.Done():
			log.Printf("supervisor stopping")
			return
		case <-ticker.C:
			o.superviseTick(ctx, states, time.Now())
		}
	}
}

// superviseTick evaluates every supervised process once.
func (o *Orchestrator) superviseTick(ctx context.Context, states map[string]*restartState, now time.Time) {
	o.mu.RLock()
	procs := make([]*process.Process, 0, len(o.processes))
	for _, p := range o.processes {
		procs = append(procs, p)
	}
	o.mu.RUnlock()

	for _, p := range procs {
		cfg := p.Config
		// Only supervise processes that opt into auto_restart or define a health check.
		if !cfg.AutoRestart && !p.HasHealthCheck() {
			continue
		}

		id := p.ID
		st := states[id]
		if st == nil {
			st = &restartState{}
			states[id] = st
		}

		status := p.Status()
		failed := false

		switch status {
		case types.StatusCrashed:
			failed = true
		case types.StatusRunning, types.StatusUnhealthy:
			if p.HasHealthCheck() {
				if now.Sub(st.lastCheck) >= cfg.HealthCheck.IntervalDuration() {
					healthy, note := p.CheckHealth()
					p.RecordHealth(healthy, note)
					st.lastCheck = now
					if !healthy {
						log.Printf("[supervisor] %s unhealthy: %s", id, note)
					}
					failed = !healthy
				} else {
					failed = status == types.StatusUnhealthy
				}
			}
		case types.StatusStopped:
			// Intentionally stopped — clear state, don't resurrect.
			st.retries = 0
			st.nextAttempt = time.Time{}
			st.gaveUp = false
			continue
		default:
			// starting / stopping / building — transient, leave alone.
			continue
		}

		dec := decideRestart(st, tickInput{
			failed:       failed,
			autoRestart:  cfg.AutoRestart,
			maxRetries:   cfg.MaxRetries,
			restartDelay: cfg.RestartDelayDuration(),
			now:          now,
		})

		if dec.GiveUp {
			if !st.gaveUp {
				log.Printf("[supervisor] %s giving up after %d retries", id, cfg.MaxRetries)
				st.gaveUp = true
			}
			continue
		}
		if dec.Restart {
			if ctx.Err() != nil {
				return
			}
			if p.HasHealthCheck() && cfg.HealthCheck.KillPortHolder {
				if killed := p.KillPortHolders(); len(killed) > 0 {
					log.Printf("[supervisor] %s killed port holders %v", id, killed)
				}
			}
			log.Printf("[supervisor] restarting %s (attempt %d, status=%s)", id, st.retries, status)
			go func(pr *process.Process) {
				if err := pr.Restart(); err != nil {
					log.Printf("[supervisor] restart of %s failed: %v", pr.ID, err)
				}
			}(p)
		}
	}
}
