// Package nboxclient is entrypushd's HTTP client for the nbox REST API.
// Isolated unit with its own DTO; never depends on nbox internals.
package nboxclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"resty.dev/v3"
)

const (
	maxResponseBytes = 32 << 20 // defensive body cap

	// Few retries: these calls are in the agent-connect critical path.
	retryCount       = 2
	retryWaitTime    = 100 * time.Millisecond
	retryMaxWaitTime = 1 * time.Second
)

// Entry mirrors the JSON of nbox's entry endpoints ({key,value,secure}).
type Entry struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secure bool   `json:"secure"`
}

// Client reads entry snapshots and values from nbox over HTTP.
type Client struct {
	r *resty.Client
}

// NewClient builds a Client for the given nbox base URL, reusing the
// provided *http.Client (nil => resty default).
func NewClient(baseURL string, hc *http.Client) *Client {
	var c *resty.Client
	if hc != nil {
		c = resty.NewWithClient(hc)
	} else {
		c = resty.New()
	}
	c.SetBaseURL(strings.TrimRight(baseURL, "/")).
		SetResponseBodyLimit(maxResponseBytes).
		SetLoggerWarnLevel(true). // entrypushd↔nbox is plain HTTP (TLS is future)
		SetRetryCount(retryCount).
		SetRetryWaitTime(retryWaitTime).
		SetRetryMaxWaitTime(retryMaxWaitTime).
		SetRetryDefaultConditions(false).
		AddRetryConditions(retryOnTransient)
	return &Client{r: c}
}

// retryOnTransient retries transport errors and 5xx; never 4xx (terminal)
// nor a cancelled context.
func retryOnTransient(resp *resty.Response, err error) bool {
	if err != nil {
		return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
	}
	return resp.StatusCode() >= http.StatusInternalServerError
}

// Snapshot does GET /api/entry/prefix?v=<prefix> as Bearer <token> and
// returns the entries. Token is per-call (never stored on the Client).
func (c *Client) Snapshot(ctx context.Context, token, prefix string) ([]Entry, error) {
	resp, err := c.r.R().
		SetContext(ctx).
		SetAuthToken(token).
		SetHeader("Accept", "application/json").
		SetQueryParam("v", prefix).
		SetQueryParam("leaves", "true").
		Get("/api/entry/prefix")
	if err != nil {
		return nil, fmt.Errorf("nbox snapshot: %s: %w", prefix, err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("nbox snapshot: %s: unexpected status %d", prefix, resp.StatusCode())
	}

	var entries []Entry
	if err := json.Unmarshal(resp.Bytes(), &entries); err != nil {
		return nil, fmt.Errorf("nbox snapshot: %s: decode body: %w", prefix, err)
	}
	return entries, nil
}

// Resolve does GET /api/entry/resolve?v=<key> as Bearer <token> and
// returns the entry's plaintext value.
func (c *Client) Resolve(ctx context.Context, token, key string) (string, error) {
	resp, err := c.r.R().
		SetContext(ctx).
		SetAuthToken(token).
		SetHeader("Accept", "application/json").
		SetQueryParam("v", key).
		Get("/api/entry/resolve")
	if err != nil {
		return "", fmt.Errorf("nbox resolve: %s: %w", key, err)
	}
	if resp.IsStatusFailure() {
		return "", fmt.Errorf("nbox resolve: %s: unexpected status %d", key, resp.StatusCode())
	}

	var e Entry
	if err := json.Unmarshal(resp.Bytes(), &e); err != nil {
		return "", fmt.Errorf("nbox resolve: %s: decode body: %w", key, err)
	}
	return e.Value, nil
}
