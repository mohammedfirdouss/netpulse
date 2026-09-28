package probe

import (
	"context"
	"net"
	"time"

	"github.com/mohammedfirdouss/netpulse/internal/config"
)

type DNSProber struct {
	Resolver *net.Resolver // nil means net.DefaultResolver
}

func (p DNSProber) Probe(ctx context.Context, t config.Target) Result {
	ctx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	r := p.Resolver
	if r == nil {
		r = net.DefaultResolver
	}
	start := time.Now()
	_, err := r.LookupHost(ctx, t.Address)
	if err != nil {
		return Result{Sent: 1, Err: err}
	}
	return Result{Success: true, Latency: time.Since(start), Sent: 1, Received: 1}
}
