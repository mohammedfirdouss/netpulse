package probe

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/mohammedfirdouss/netpulse/internal/config"
)

func TestTCPProberSuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	target := config.Target{Name: "local", Type: "tcp", Address: ln.Addr().String(), Timeout: time.Second}
	r := TCPProber{}.Probe(context.Background(), target)
	if !r.Success {
		t.Fatalf("expected success, got error: %v", r.Err)
	}
	if r.Latency <= 0 {
		t.Fatalf("expected positive latency, got %v", r.Latency)
	}
	if r.Sent != 1 || r.Received != 1 {
		t.Fatalf("Sent/Received = %d/%d, want 1/1", r.Sent, r.Received)
	}
}

func TestTCPProberClosedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listening now -> connection refused (RST)

	target := config.Target{Name: "closed", Type: "tcp", Address: addr, Timeout: time.Second}
	r := TCPProber{}.Probe(context.Background(), target)
	if r.Success {
		t.Fatal("expected failure on closed port")
	}
	if r.Sent != 1 || r.Received != 0 {
		t.Fatalf("Sent/Received = %d/%d, want 1/0", r.Sent, r.Received)
	}
}

func TestTCPProberTimeout(t *testing.T) {
	// 192.0.2.0/24 (TEST-NET-1) is reserved for documentation, so the SYN
	// goes nowhere and the dial has to hit our timeout.
	target := config.Target{Name: "blackhole", Type: "tcp", Address: "192.0.2.1:80", Timeout: 200 * time.Millisecond}
	start := time.Now()
	r := TCPProber{}.Probe(context.Background(), target)
	if r.Success {
		t.Fatal("expected failure on unroutable address")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("probe took %v, timeout not honoured", elapsed)
	}
}

func TestTCPProberBadAddress(t *testing.T) {
	target := config.Target{Name: "bad", Type: "tcp", Address: "no-port", Timeout: time.Second}
	r := TCPProber{}.Probe(context.Background(), target)
	if r.Success || r.Err == nil {
		t.Fatal("expected error for address without port")
	}
}
