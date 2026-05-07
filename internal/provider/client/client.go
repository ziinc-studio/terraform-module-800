// Package client is a thin, typed HTTP client for the 800.com public API.
//
// Goals:
//   - One place to handle bearer auth, rate limiting, and the three
//     pagination shapes the API returns. Resource code never sees the
//     `X-RateLimit-*` headers or `links.next` URLs.
//   - Errors from the API land as a typed *APIError so resource code can
//     branch on status codes (e.g. 404 -> remove from state).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Endpoint              string
	Token                 string
	MaxRetries            int
	RequestTimeoutSeconds int
	UserAgent             string
}

type Client struct {
	cfg  Config
	http *http.Client
	base *url.URL
}

func New(cfg Config) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("endpoint is required")
	}
	if cfg.Token == "" {
		return nil, errors.New("token is required")
	}
	base, err := url.Parse(strings.TrimRight(cfg.Endpoint, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint %q: %w", cfg.Endpoint, err)
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}
	if cfg.RequestTimeoutSeconds <= 0 {
		cfg.RequestTimeoutSeconds = 30
	}
	return &Client{
		cfg:  cfg,
		base: base,
		http: &http.Client{Timeout: time.Duration(cfg.RequestTimeoutSeconds) * time.Second},
	}, nil
}

// Do issues a request against the API, encoding `body` as JSON when
// non-nil and decoding the response envelope into `out` (which may be
// nil for fire-and-forget calls). It transparently retries on 429 and
// 5xx up to MaxRetries times, honouring `Retry-After` when present.
//
// `out` should point to a struct whose JSON tags match the unwrapped
// payload — the wrapper `{"data": ...}` is stripped here.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(path, "/")
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		var rdr io.Reader
		if bodyBytes != nil {
			rdr = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, u.String(), rdr)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
		req.Header.Set("Accept", "application/json")
		if c.cfg.UserAgent != "" {
			req.Header.Set("User-Agent", c.cfg.UserAgent)
		}
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json;charset=UTF-8")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			if !shouldRetryNetwork(err) || attempt == c.cfg.MaxRetries {
				return err
			}
			time.Sleep(backoff(attempt))
			continue
		}

		// Rate-limit awareness: if the server tells us we're at zero
		// remaining and Retry-After is set, honour it without burning a
		// retry slot the next time the user calls.
		if resp.StatusCode == http.StatusTooManyRequests && attempt < c.cfg.MaxRetries {
			delay := parseRetryAfter(resp.Header.Get("Retry-After"))
			_ = resp.Body.Close()
			if delay <= 0 {
				delay = backoff(attempt)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			continue
		}

		if resp.StatusCode >= 500 && attempt < c.cfg.MaxRetries {
			_ = resp.Body.Close()
			time.Sleep(backoff(attempt))
			continue
		}

		return decodeResponse(resp, out)
	}

	if lastErr != nil {
		return lastErr
	}
	return errors.New("exhausted retries")
}

// decodeResponse handles the two envelope shapes the API returns.
// Success: {"data": <payload>}. Errors come in two shapes — see errors.go.
func decodeResponse(resp *http.Response, out any) error {
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return parseAPIError(resp.StatusCode, raw)
	}

	if out == nil || len(raw) == 0 {
		return nil
	}

	// Strip the {"data": ...} envelope when present. Some endpoints
	// (e.g. simple action POSTs) return the payload at top level —
	// fall back to direct decode in that case.
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err == nil && len(env.Data) > 0 {
		return json.Unmarshal(env.Data, out)
	}
	return json.Unmarshal(raw, out)
}

func backoff(attempt int) time.Duration {
	// 1s, 2s, 4s, capped — small surface, intentional.
	d := time.Duration(1<<attempt) * time.Second
	if d > 10*time.Second {
		d = 10 * time.Second
	}
	return d
}

func parseRetryAfter(h string) time.Duration {
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}

func shouldRetryNetwork(err error) bool {
	// Conservative: retry transient transport errors but not context
	// cancellations or timeouts the caller asked for.
	var netOpErr interface{ Timeout() bool }
	if errors.As(err, &netOpErr) && netOpErr.Timeout() {
		return true
	}
	return false
}
