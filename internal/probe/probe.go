package probe

import (
	"context"
	"time"

	"github.com/mohammedfirdouss/netpulse/internal/config"
)

type Result struct {
	Success  bool
	Latency  time.Duration
	Jitter   time.Duration // ICMP only (RTT std dev)
	Sent     int
	Received int
	Err      error
}

type Prober interface {
	Probe(ctx context.Context, t config.Target) Result
}
