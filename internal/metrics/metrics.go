package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/mohammedfirdouss/netpulse/internal/config"
	"github.com/mohammedfirdouss/netpulse/internal/probe"
)

var labels = []string{"target", "type"}

// Metrics holds the collectors for one registry. Keeping them off package
// globals lets tests use a fresh registry each time.
type Metrics struct {
	up          *prometheus.GaugeVec
	latency     *prometheus.HistogramVec
	jitter      *prometheus.GaugeVec
	sent        *prometheus.CounterVec
	received    *prometheus.CounterVec
	errorsTotal *prometheus.CounterVec
}

func New(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		up: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "netpulse_probe_up",
			Help: "1 if the last probe succeeded, 0 otherwise.",
		}, labels),
		// Histogram rather than summary: buckets aggregate across instances
		// and give percentiles in PromQL.
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "netpulse_probe_latency_seconds",
			Help:    "Probe latency (ICMP avg RTT, TCP handshake, DNS lookup).",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms .. ~2s
		}, labels),
		jitter: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "netpulse_icmp_jitter_seconds",
			Help: "Std deviation of ICMP RTT in the last probe.",
		}, labels),
		// Loss is derived from these two counters in PromQL, so it is
		// correct over any window rather than only for the last probe.
		sent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "netpulse_packets_sent_total",
			Help: "Probe packets/attempts sent.",
		}, labels),
		received: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "netpulse_packets_received_total",
			Help: "Probe packets/attempts that got a reply.",
		}, labels),
		errorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "netpulse_probe_errors_total",
			Help: "Probes that returned an error.",
		}, labels),
	}
	reg.MustRegister(m.up, m.latency, m.jitter, m.sent, m.received, m.errorsTotal)
	return m
}

// Init creates every counter series at zero so rate() has a starting point
// and a target that has never failed still exports errors_total 0.
func (m *Metrics) Init(targets []config.Target) {
	for _, t := range targets {
		l := prometheus.Labels{"target": t.Name, "type": t.Type}
		m.sent.With(l)
		m.received.With(l)
		m.errorsTotal.With(l)
	}
}

func (m *Metrics) Record(t config.Target, r probe.Result) {
	l := prometheus.Labels{"target": t.Name, "type": t.Type}
	m.sent.With(l).Add(float64(r.Sent))
	m.received.With(l).Add(float64(r.Received))
	if r.Err != nil {
		m.errorsTotal.With(l).Inc()
	}
	if !r.Success {
		m.up.With(l).Set(0)
		return
	}
	m.up.With(l).Set(1)
	m.latency.With(l).Observe(r.Latency.Seconds())
	if t.Type == "icmp" {
		m.jitter.With(l).Set(r.Jitter.Seconds())
	}
}
