package metrics

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/mohammedfirdouss/netpulse/internal/config"
	"github.com/mohammedfirdouss/netpulse/internal/probe"
)

func TestRecord(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)
	target := config.Target{Name: "a", Type: "icmp"}
	m.Init([]config.Target{target})

	m.Record(target, probe.Result{Success: true, Latency: 10 * time.Millisecond, Jitter: 2 * time.Millisecond, Sent: 5, Received: 4})
	m.Record(target, probe.Result{Sent: 5, Received: 0, Err: errors.New("boom")})

	want := `
# HELP netpulse_packets_received_total Probe packets/attempts that got a reply.
# TYPE netpulse_packets_received_total counter
netpulse_packets_received_total{target="a",type="icmp"} 4
# HELP netpulse_packets_sent_total Probe packets/attempts sent.
# TYPE netpulse_packets_sent_total counter
netpulse_packets_sent_total{target="a",type="icmp"} 10
# HELP netpulse_probe_errors_total Probes that returned an error.
# TYPE netpulse_probe_errors_total counter
netpulse_probe_errors_total{target="a",type="icmp"} 1
# HELP netpulse_probe_up 1 if the last probe succeeded, 0 otherwise.
# TYPE netpulse_probe_up gauge
netpulse_probe_up{target="a",type="icmp"} 0
# HELP netpulse_icmp_jitter_seconds Std deviation of ICMP RTT in the last probe.
# TYPE netpulse_icmp_jitter_seconds gauge
netpulse_icmp_jitter_seconds{target="a",type="icmp"} 0.002
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(want),
		"netpulse_packets_received_total", "netpulse_packets_sent_total",
		"netpulse_probe_errors_total", "netpulse_probe_up", "netpulse_icmp_jitter_seconds")
	if err != nil {
		t.Fatal(err)
	}
	// Only the successful probe should land in the latency histogram.
	if n := testutil.CollectAndCount(m.latency); n != 1 {
		t.Fatalf("latency series = %d, want 1", n)
	}
}

func TestInitExportsZeroCounters(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)
	m.Init([]config.Target{{Name: "a", Type: "tcp"}, {Name: "b", Type: "dns"}})
	if n := testutil.CollectAndCount(m.errorsTotal); n != 2 {
		t.Fatalf("errors_total series = %d, want 2", n)
	}
}
