package probe

import (
	"context"
	"errors"

	probing "github.com/prometheus-community/pro-bing"

	"github.com/mohammedfirdouss/netpulse/internal/config"
)

type ICMPProber struct{}

func (ICMPProber) Probe(ctx context.Context, t config.Target) Result {
	// Resolution happens inside RunWithContext, bounded by ResolveTimeout,
	// so a slow resolver can't stall the probe past its timeout.
	p := probing.New(t.Address)
	p.SetNetwork(t.Network())
	p.ResolveTimeout = t.Timeout
	p.Count = t.Count
	p.Interval = config.ICMPPacketInterval
	p.Timeout = t.Timeout
	p.SetPrivileged(false) // unprivileged "ping sockets" (UDP-based ICMP)

	if err := p.RunWithContext(ctx); err != nil {
		return Result{Sent: t.Count, Err: err}
	}
	s := p.Statistics()
	r := Result{
		Success:  s.PacketsRecv > 0,
		Latency:  s.AvgRtt,
		Jitter:   s.StdDevRtt,
		Sent:     s.PacketsSent,
		Received: s.PacketsRecv,
	}
	if !r.Success {
		r.Err = errors.New("no echo replies received")
	}
	return r
}
