// Package hysteria is a thin client for the Hysteria 2 Traffic Stats API.
// See https://v2.hysteria.network/zh/docs/advanced/Traffic-Stats-API/
package hysteria

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TrafficEntry is one user's cumulative counters from GET /traffic.
type TrafficEntry struct {
	Tx int64 `json:"tx"`
	Rx int64 `json:"rx"`
}

// Stream mirrors one entry of GET /dump/streams.
type Stream struct {
	State         string `json:"state"`
	Auth          string `json:"auth"`
	Connection    int64  `json:"connection"`
	Stream        int64  `json:"stream"`
	ReqAddr       string `json:"req_addr"`
	HookedReqAddr string `json:"hooked_req_addr"`
	Tx            int64  `json:"tx"`
	Rx            int64  `json:"rx"`
	InitialAt     string `json:"initial_at"`
	LastActiveAt  string `json:"last_active_at"`
}

type dumpResponse struct {
	Streams []Stream `json:"streams"`
}

// Client talks to a single Hysteria node's stats API.
type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

// New builds a client. baseURL is e.g. http://1.2.3.4:9999 (no trailing slash needed).
func New(baseURL, secret string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		secret:  secret,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *Client) get(ctx context.Context, path string, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.secret != "" {
		req.Header.Set("Authorization", c.secret)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	return c.http.Do(req)
}

func decode(resp *http.Response, out any) error {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hysteria api returned %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Traffic returns the cumulative tx/rx counters keyed by auth string.
func (c *Client) Traffic(ctx context.Context) (map[string]TrafficEntry, error) {
	resp, err := c.get(ctx, "/traffic", "")
	if err != nil {
		return nil, err
	}
	out := map[string]TrafficEntry{}
	if err := decode(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Online returns the device (client instance) count keyed by auth string.
func (c *Client) Online(ctx context.Context) (map[string]int, error) {
	resp, err := c.get(ctx, "/online", "")
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	if err := decode(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DumpStreams returns the live TCP streams snapshot.
func (c *Client) DumpStreams(ctx context.Context) ([]Stream, error) {
	resp, err := c.get(ctx, "/dump/streams", "")
	if err != nil {
		return nil, err
	}
	var out dumpResponse
	if err := decode(resp, &out); err != nil {
		return nil, err
	}
	return out.Streams, nil
}

// Ping hits /traffic to validate the address and secret. Returns latency.
func (c *Client) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	resp, err := c.get(ctx, "/traffic", "")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return 0, fmt.Errorf("unauthorized: bad secret")
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return time.Since(start), nil
}
