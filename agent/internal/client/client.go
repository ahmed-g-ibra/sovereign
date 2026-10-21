// Package client talks to the Sovereign control plane.
package client

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sovereign/agent/internal/probe"
	"sovereign/agent/internal/targets"
)

// Client is an enrolled probe's connection to the control plane.
type Client struct {
	BaseURL string
	ProbeID string

	key  ed25519.PrivateKey
	http *http.Client
}

func New(baseURL, probeID string, key ed25519.PrivateKey) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		ProbeID: probeID,
		key:     key,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Config is the control plane's instructions.
type Config struct {
	IntervalS  int              `json:"interval_s"`
	TimeoutS   int              `json:"timeout_s"`
	ConfigTTLS int              `json:"config_ttl_s"`
	Targets    []targets.Target `json:"targets"`
}

type BatchResponse struct {
	Accepted    int  `json:"accepted"`
	Duplicates  int  `json:"duplicates"`
	Quarantined bool `json:"quarantined"`
	ClockSkewMs int  `json:"clock_skew_ms"`
}

// RegisterInfo describes this vantage point to the server.
type RegisterInfo struct {
	EnrollmentToken string `json:"enrollment_token"`
	PublicKey       string `json:"public_key"`
	Name            string `json:"name"`
	Country         string `json:"country"`
	City            string `json:"city,omitempty"`
	ISPClaimed      string `json:"isp_claimed,omitempty"`
	Tier            string `json:"tier,omitempty"`
}

// Register spends an enrolment token. It is the one unsigned call: the probe
// has no identity yet.
func Register(ctx context.Context, baseURL string, info RegisterInfo) (string, error) {
	body, err := json.Marshal(info)
	if err != nil {
		return "", err
	}
	url := strings.TrimRight(baseURL, "/") + "/v1/probes/register"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("register: %s: %s", resp.Status, snippet(raw))
	}
	var out struct {
		ProbeID string `json:"probe_id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("register: bad response: %w", err)
	}
	return out.ProbeID, nil
}

// Config fetches what to measure.
func (c *Client) Config(ctx context.Context) (*Config, error) {
	var cfg Config
	if err := c.do(ctx, http.MethodGet, "/v1/config", nil, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Upload sends a batch of measurements. Safe to retry: the server deduplicates
// on (probe, target, timestamp), so a batch that was stored but whose response
// we never saw costs nothing to send again.
func (c *Client) Upload(ctx context.Context, results []probe.Result) (*BatchResponse, error) {
	payload := struct {
		SentAt  time.Time    `json:"sent_at"`
		Samples []wireSample `json:"samples"`
	}{
		SentAt:  time.Now().UTC(),
		Samples: toWire(results),
	}
	var out BatchResponse
	if err := c.do(ctx, http.MethodPost, "/v1/samples", payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// wireSample is the server's view of a measurement.
//
// Separate from probe.Result on purpose: the API rejects a failed sample that
// carries timings, so the pointer fields here make "absent" expressible. The
// domain type stays simple and the contract stays explicit.
type wireSample struct {
	TargetID  string    `json:"target_id"`
	TS        time.Time `json:"ts"`
	DNSMs     *float64  `json:"dns_ms,omitempty"`
	ConnectMs *float64  `json:"connect_ms,omitempty"`
	TLSMs     *float64  `json:"tls_ms,omitempty"`
	TotalMs   *float64  `json:"total_ms,omitempty"`
	RemoteIP  string    `json:"remote_ip,omitempty"`
	Error     string    `json:"error,omitempty"`
	HostLoad1 *float64  `json:"host_load1,omitempty"`
}

func toWire(results []probe.Result) []wireSample {
	out := make([]wireSample, 0, len(results))
	for _, r := range results {
		w := wireSample{TargetID: r.TargetID, TS: r.Timestamp, RemoteIP: r.RemoteIP}
		if r.HostLoad1 > 0 {
			load := r.HostLoad1
			w.HostLoad1 = &load
		}
		if r.OK() {
			dns, conn, tls, total := r.DNSMs, r.ConnectMs, r.TLSMs, r.TotalMs
			w.DNSMs, w.ConnectMs, w.TLSMs, w.TotalMs = &dns, &conn, &tls, &total
		} else {
			// A failed probe reports only what happened. Partial timings from a
			// connection that never completed describe nothing.
			w.Error = r.Error
		}
		out = append(out, w)
	}
	return out
}

func (c *Client) do(ctx context.Context, method, path string, payload, out any) error {
	var body []byte
	if payload != nil {
		var err error
		if body, err = json.Marshal(payload); err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	ts := time.Now().Unix()
	req.Header.Set("content-type", "application/json")
	req.Header.Set(HeaderProbe, c.ProbeID)
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(ts, 10))
	req.Header.Set(HeaderSignature, signRequest(c.key, method, path, ts, body))

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, snippet(raw))
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:199] + "…"
	}
	return s
}
