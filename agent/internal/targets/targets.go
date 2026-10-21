// Package targets loads the measurement target list.
//
// Targets are data, not code: the list lives in targets.json so that a region
// launch or a dead endpoint is a one-line edit and a redeploy of nothing.
package targets

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Status describes how much we trust a target's endpoint.
const (
	// StatusLive means the region is generally available and the endpoint is confirmed.
	StatusLive = "live"
	// StatusVerify means the endpoint is a documented pattern we have not confirmed
	// resolves. Run `probe -verify` and prune what fails.
	StatusVerify = "verify"
	// StatusAnnounced means the region is not open yet. Kept here so that launch day
	// is a status flip rather than a scramble.
	StatusAnnounced = "announced"
	// StatusRejected means we tested the endpoint and decided against it. The
	// entry stays so the finding is not rediscovered every few months; the Note
	// field carries the reason.
	StatusRejected = "rejected"
)

// Target is one measurement endpoint.
type Target struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Region   string `json:"region"`
	Country  string `json:"country"`
	City     string `json:"city"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Status   string `json:"status"`
	Control  bool   `json:"is_control,omitempty"`
	Note     string `json:"note,omitempty"`
}

// Addr returns the host:port to dial.
func (t Target) Addr(ip string) string {
	return fmt.Sprintf("%s:%d", ip, t.Port)
}

type file struct {
	Version   int      `json:"version"`
	UpdatedAt string   `json:"updated_at"`
	Targets   []Target `json:"targets"`
}

// Load reads and validates a targets file.
func Load(path string) ([]Target, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read targets: %w", err)
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse targets: %w", err)
	}
	seen := make(map[string]bool, len(f.Targets))
	for i := range f.Targets {
		t := &f.Targets[i]
		if t.ID == "" || t.Host == "" {
			return nil, fmt.Errorf("target %d: id and host are required", i)
		}
		if seen[t.ID] {
			return nil, fmt.Errorf("duplicate target id %q", t.ID)
		}
		seen[t.ID] = true
		if t.Port == 0 {
			t.Port = 443
		}
		if t.Status == "" {
			t.Status = StatusVerify
		}
	}
	if len(f.Targets) == 0 {
		return nil, fmt.Errorf("no targets defined in %s", path)
	}
	return f.Targets, nil
}

// Filter keeps targets matching every comma-separated term in expr. A term
// matches against id, provider, region, country or status. An empty expr keeps
// everything.
func Filter(ts []Target, expr string) []Target {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ts
	}
	terms := strings.Split(strings.ToLower(expr), ",")
	out := make([]Target, 0, len(ts))
	for _, t := range ts {
		hay := strings.ToLower(strings.Join([]string{
			t.ID, t.Provider, t.Region, t.Country, t.Status,
		}, " "))
		keep := true
		for _, term := range terms {
			if term = strings.TrimSpace(term); term != "" && !strings.Contains(hay, term) {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, t)
		}
	}
	return out
}
