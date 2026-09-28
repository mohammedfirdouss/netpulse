package probe

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/mohammedfirdouss/netpulse/internal/config"
)

// TCPProber times the 3-way handshake (SYN -> SYN-ACK -> ACK), which is
// roughly one RTT plus the server's accept time.
type TCPProber struct {
	Resolver *net.Resolver // nil means net.DefaultResolver
}

func (p TCPProber) Probe(ctx context.Context, t config.Target) Result {
	ctx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()

	// Resolve before starting the clock so DNS time isn't counted as
	// handshake latency. The DNS prober measures resolution separately.
	host, port, err := net.SplitHostPort(t.Address)
	if err != nil {
		return Result{Sent: 1, Err: err}
	}
	r := p.Resolver
	if r == nil {
		r = net.DefaultResolver
	}
	ips, err := r.LookupNetIP(ctx, t.Network(), host)
	if err != nil {
		return Result{Sent: 1, Err: fmt.Errorf("resolve %s: %w", host, err)}
	}
	addr := net.JoinHostPort(ips[0].Unmap().String(), port)

	var d net.Dialer
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return Result{Sent: 1, Err: err}
	}
	latency := time.Since(start)
	conn.Close()
	return Result{Success: true, Latency: latency, Sent: 1, Received: 1}
}
