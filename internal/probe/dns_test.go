package probe

import (
	"context"
	"testing"
	"time"

	"github.com/mohammedfirdouss/netpulse/internal/config"
)

func TestDNSProberLocalhost(t *testing.T) {
	target := config.Target{Name: "local", Type: "dns", Address: "localhost", Timeout: time.Second}
	r := DNSProber{}.Probe(context.Background(), target)
	if !r.Success {
		t.Fatalf("expected success, got error: %v", r.Err)
	}
}

func TestDNSProberNXDomain(t *testing.T) {
	// .invalid is reserved by RFC 2606 and must never resolve.
	target := config.Target{Name: "nx", Type: "dns", Address: "netpulse.invalid", Timeout: time.Second}
	r := DNSProber{}.Probe(context.Background(), target)
	if r.Success {
		t.Fatal("expected failure for .invalid name")
	}
	if r.Sent != 1 || r.Received != 0 {
		t.Fatalf("Sent/Received = %d/%d, want 1/0", r.Sent, r.Received)
	}
}
