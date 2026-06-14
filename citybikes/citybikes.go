// Package citybikes is the library behind the citybikes command line:
// the HTTP client, request shaping, and the typed data models for the
// Citybik.es API (https://api.citybik.es/v2/).
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
package citybikes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultUserAgent identifies the client to Citybik.es.
const DefaultUserAgent = "citybikes-cli/0.1 (tamnd87@gmail.com)"

// Host is the API host this client talks to.
const Host = "api.citybik.es"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host

// Client talks to the Citybik.es API over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults: a 15s timeout, a 500ms
// minimum gap between requests, and three retries on transient errors.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		UserAgent: DefaultUserAgent,
		Rate:      500 * time.Millisecond,
		Retries:   3,
	}
}

// Get fetches url and returns the response body. It paces and retries according
// to the client's settings. The caller owns nothing extra; the body is read
// fully and closed here.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

func (c *Client) do(ctx context.Context, url string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- wire types (internal JSON shapes) ---

type wireNetwork struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Location struct {
		City      string  `json:"city"`
		Country   string  `json:"country"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"location"`
}

type wireStation struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Timestamp  string  `json:"timestamp"`
	FreeBikes  int     `json:"free_bikes"`
	EmptySlots int     `json:"empty_slots"`
}

// --- output types ---

// Network represents one bike-sharing network in the Citybik.es directory.
type Network struct {
	ID      string `kit:"id" json:"id"`
	Name    string `json:"name"`
	City    string `json:"city"`
	Country string `json:"country"`
	Lat     string `json:"lat"`
	Lon     string `json:"lon"`
}

// Station represents one docking station within a bike-sharing network.
type Station struct {
	ID         string `kit:"id" json:"id"`
	Name       string `json:"name"`
	FreeBikes  int    `json:"free_bikes"`
	EmptySlots int    `json:"empty_slots"`
	Lat        string `json:"lat"`
	Lon        string `json:"lon"`
	Updated    string `json:"updated"`
}

// --- API methods ---

// Networks returns all bike-sharing networks, optionally filtered by country
// code (case-insensitive, e.g. "US"). limit caps the returned slice; 0 = all.
func (c *Client) Networks(ctx context.Context, country string, limit int) ([]*Network, error) {
	url := BaseURL + "/v2/networks?fields=id,name,location"
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("networks: %w", err)
	}

	var resp struct {
		Networks []wireNetwork `json:"networks"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("networks: decode: %w", err)
	}

	var out []*Network
	for i := range resp.Networks {
		w := &resp.Networks[i]
		if country != "" && !eqFold(w.Location.Country, country) {
			continue
		}
		out = append(out, &Network{
			ID:      w.ID,
			Name:    w.Name,
			City:    w.Location.City,
			Country: w.Location.Country,
			Lat:     fmt.Sprintf("%.4f", w.Location.Latitude),
			Lon:     fmt.Sprintf("%.4f", w.Location.Longitude),
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Stations returns all docking stations for a network by its ID (e.g. "citi-bike-nyc").
func (c *Client) Stations(ctx context.Context, networkID string) ([]*Station, error) {
	url := BaseURL + "/v2/networks/" + networkID
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("stations %s: %w", networkID, err)
	}

	var resp struct {
		Network struct {
			Stations []wireStation `json:"stations"`
		} `json:"network"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("stations %s: decode: %w", networkID, err)
	}

	out := make([]*Station, 0, len(resp.Network.Stations))
	for i := range resp.Network.Stations {
		w := &resp.Network.Stations[i]
		out = append(out, &Station{
			ID:         w.ID,
			Name:       w.Name,
			FreeBikes:  w.FreeBikes,
			EmptySlots: w.EmptySlots,
			Lat:        fmt.Sprintf("%.4f", w.Latitude),
			Lon:        fmt.Sprintf("%.4f", w.Longitude),
			Updated:    w.Timestamp,
		})
	}
	return out, nil
}

// eqFold reports whether a and b are equal ignoring ASCII case.
func eqFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'a' && ca <= 'z' {
			ca -= 32
		}
		if cb >= 'a' && cb <= 'z' {
			cb -= 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
