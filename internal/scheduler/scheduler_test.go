package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mohammedfirdouss/netpulse/internal/config"
	"github.com/mohammedfirdouss/netpulse/internal/probe"
)

type fakeProber struct{}

func (fakeProber) Probe(context.Context, config.Target) probe.Result {
	return probe.Result{Success: true, Sent: 1, Received: 1}
}

func TestRunProbesEachTargetAndStops(t *testing.T) {
	targets := []config.Target{
		{Name: "a", Type: "fake", Interval: 20 * time.Millisecond},
		{Name: "b", Type: "fake", Interval: 20 * time.Millisecond},
		{Name: "c", Type: "missing", Interval: 20 * time.Millisecond},
	}
	var mu sync.Mutex
	counts := map[string]int{}
	record := func(t config.Target, _ probe.Result) {
		mu.Lock()
		counts[t.Name]++
		mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		Run(ctx, targets, map[string]probe.Prober{"fake": fakeProber{}}, record)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}

	mu.Lock()
	defer mu.Unlock()
	for _, name := range []string{"a", "b"} {
		if counts[name] < 2 {
			t.Errorf("target %s probed %d times, want at least 2", name, counts[name])
		}
	}
	if counts["c"] != 0 {
		t.Errorf("target with no prober was probed %d times", counts["c"])
	}
}
