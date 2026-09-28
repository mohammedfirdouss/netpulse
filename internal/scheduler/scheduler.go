package scheduler

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/mohammedfirdouss/netpulse/internal/config"
	"github.com/mohammedfirdouss/netpulse/internal/probe"
)

// RecordFunc receives every probe result.
type RecordFunc func(config.Target, probe.Result)

// Run starts one goroutine per target, each on its own ticker, and blocks
// until ctx is cancelled and every goroutine has returned.
func Run(ctx context.Context, targets []config.Target, probers map[string]probe.Prober, record RecordFunc) {
	var wg sync.WaitGroup
	for _, t := range targets {
		p, ok := probers[t.Type]
		if !ok {
			slog.Error("no prober for target type, skipping", "target", t.Name, "type", t.Type)
			continue
		}
		wg.Go(func() { runTarget(ctx, t, p, record) })
	}
	wg.Wait()
}

func runTarget(ctx context.Context, t config.Target, p probe.Prober, record RecordFunc) {
	// Spread first probes across the interval so N targets don't all fire
	// in the same instant at startup and stay phase-locked afterwards.
	select {
	case <-ctx.Done():
		return
	case <-time.After(rand.N(t.Interval)):
	}

	ticker := time.NewTicker(t.Interval)
	defer ticker.Stop()
	for {
		r := p.Probe(ctx, t)
		// A probe cut short by shutdown isn't a real failure; don't record it.
		if ctx.Err() != nil {
			return
		}
		record(t, r)
		if r.Err != nil {
			slog.Warn("probe failed", "target", t.Name, "type", t.Type, "err", r.Err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
