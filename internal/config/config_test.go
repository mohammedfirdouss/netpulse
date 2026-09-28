package config

import (
	"strings"
	"testing"
	"time"
)

func TestParseDefaults(t *testing.T) {
	c, err := Parse([]byte(`
targets:
  - name: a
    type: tcp
    address: example.com:443
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":9101" {
		t.Errorf("ListenAddr = %q, want :9101", c.ListenAddr)
	}
	got := c.Targets[0]
	if got.Interval != 10*time.Second || got.Timeout != 2*time.Second || got.Count != 3 || got.IPVersion != 4 {
		t.Errorf("defaults not applied: %+v", got)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{"no targets", `listen_addr: ":1"`, "no targets"},
		{"missing name", `
targets:
  - type: tcp
    address: x:1`, "name and address are required"},
		{"missing address", `
targets:
  - name: a
    type: tcp`, "name and address are required"},
		{"unknown type", `
targets:
  - name: a
    type: http
    address: x`, "unknown type"},
		{"duplicate name", `
targets:
  - {name: a, type: tcp, address: "x:1"}
  - {name: a, type: dns, address: x}`, "duplicate name"},
		{"timeout longer than interval", `
targets:
  - {name: a, type: tcp, address: "x:1", interval: 1s, timeout: 2s}`, "timeout must be shorter"},
		{"timeout equal to interval", `
targets:
  - {name: a, type: tcp, address: "x:1", interval: 2s, timeout: 2s}`, "timeout must be shorter"},
		{"negative count", `
targets:
  - {name: a, type: icmp, address: x, count: -1}`, "count must be positive"},
		{"icmp packets exceed timeout", `
targets:
  - {name: a, type: icmp, address: x, count: 20, timeout: 2s, interval: 10s}`, "do not fit in timeout"},
		{"bad ip version", `
targets:
  - {name: a, type: tcp, address: "x:1", ip_version: 5}`, "ip_version must be 4 or 6"},
		{"bad duration", `
targets:
  - {name: a, type: tcp, address: "x:1", interval: soon}`, "parse config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadRepoConfig(t *testing.T) {
	if _, err := Load("../../config.yaml"); err != nil {
		t.Fatalf("shipped config.yaml is invalid: %v", err)
	}
}
