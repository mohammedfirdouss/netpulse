package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ICMPPacketInterval is the gap between echo requests within one ICMP probe.
const ICMPPacketInterval = 200 * time.Millisecond

type Target struct {
	Name      string        `yaml:"name"`
	Type      string        `yaml:"type"` // icmp | tcp | dns
	Address   string        `yaml:"address"`
	Interval  time.Duration `yaml:"interval"`
	Timeout   time.Duration `yaml:"timeout"`
	Count     int           `yaml:"count"`      // ICMP packets per probe
	IPVersion int           `yaml:"ip_version"` // 4 or 6, for icmp and tcp
}

// Network returns the Go network name ("ip4" or "ip6") for the target's
// IP version, used when resolving the address.
func (t Target) Network() string {
	if t.IPVersion == 6 {
		return "ip6"
	}
	return "ip4"
}

type Config struct {
	ListenAddr string   `yaml:"listen_addr"`
	Targets    []Target `yaml:"targets"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

func Parse(b []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ":9101"
	}
	if len(c.Targets) == 0 {
		return nil, errors.New("no targets configured")
	}
	seen := make(map[string]bool, len(c.Targets))
	for i := range c.Targets {
		t := &c.Targets[i]
		if t.Name == "" || t.Address == "" {
			return nil, fmt.Errorf("target %d: name and address are required", i)
		}
		if seen[t.Name] {
			return nil, fmt.Errorf("target %q: duplicate name", t.Name)
		}
		seen[t.Name] = true
		switch t.Type {
		case "icmp", "tcp", "dns":
		default:
			return nil, fmt.Errorf("target %q: unknown type %q", t.Name, t.Type)
		}
		if t.Interval == 0 {
			t.Interval = 10 * time.Second
		}
		if t.Timeout == 0 {
			t.Timeout = 2 * time.Second
		}
		if t.Interval < 0 || t.Timeout < 0 {
			return nil, fmt.Errorf("target %q: interval and timeout must be positive", t.Name)
		}
		if t.Timeout >= t.Interval {
			return nil, fmt.Errorf("target %q: timeout must be shorter than interval", t.Name)
		}
		if t.Count == 0 {
			t.Count = 3
		}
		// Default to IPv4: a host without an IPv6 route would otherwise fail
		// whenever the resolver happens to return an AAAA record first.
		if t.IPVersion == 0 {
			t.IPVersion = 4
		}
		if t.IPVersion != 4 && t.IPVersion != 6 {
			return nil, fmt.Errorf("target %q: ip_version must be 4 or 6", t.Name)
		}
		if t.Count < 0 {
			return nil, fmt.Errorf("target %q: count must be positive", t.Name)
		}
		// All echo requests must be sent before the probe times out, or the
		// unsent ones silently shrink the sample.
		if t.Type == "icmp" && time.Duration(t.Count-1)*ICMPPacketInterval >= t.Timeout {
			return nil, fmt.Errorf("target %q: %d packets at %v spacing do not fit in timeout %v",
				t.Name, t.Count, ICMPPacketInterval, t.Timeout)
		}
	}
	return &c, nil
}
