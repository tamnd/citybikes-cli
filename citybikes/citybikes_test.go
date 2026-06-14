package citybikes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0 // no pacing in the test

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestNetworks(t *testing.T) {
	payload := map[string]any{
		"networks": []any{
			map[string]any{
				"id":   "citi-bike-nyc",
				"name": "Citi Bike",
				"location": map[string]any{
					"city":      "New York",
					"country":   "US",
					"latitude":  40.7128,
					"longitude": -74.0060,
				},
			},
			map[string]any{
				"id":   "bicing",
				"name": "Bicing",
				"location": map[string]any{
					"city":      "Barcelona",
					"country":   "ES",
					"latitude":  41.3851,
					"longitude": 2.1734,
				},
			},
			map[string]any{
				"id":   "divvy",
				"name": "Divvy",
				"location": map[string]any{
					"city":      "Chicago",
					"country":   "US",
					"latitude":  41.8781,
					"longitude": -87.6298,
				},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.HTTP.Transport = rewriteTransport(srv.URL)

	// all networks, no filter
	nets, err := c.Networks(context.Background(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 3 {
		t.Fatalf("len(networks) = %d, want 3", len(nets))
	}
	if nets[0].ID != "citi-bike-nyc" {
		t.Errorf("networks[0].ID = %q, want citi-bike-nyc", nets[0].ID)
	}
	if nets[0].Country != "US" {
		t.Errorf("networks[0].Country = %q, want US", nets[0].Country)
	}
}

func TestNetworksCountryFilter(t *testing.T) {
	payload := map[string]any{
		"networks": []any{
			map[string]any{
				"id":   "citi-bike-nyc",
				"name": "Citi Bike",
				"location": map[string]any{
					"city": "New York", "country": "US",
					"latitude": 40.7128, "longitude": -74.0060,
				},
			},
			map[string]any{
				"id":   "bicing",
				"name": "Bicing",
				"location": map[string]any{
					"city": "Barcelona", "country": "ES",
					"latitude": 41.3851, "longitude": 2.1734,
				},
			},
			map[string]any{
				"id":   "divvy",
				"name": "Divvy",
				"location": map[string]any{
					"city": "Chicago", "country": "US",
					"latitude": 41.8781, "longitude": -87.6298,
				},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.HTTP.Transport = rewriteTransport(srv.URL)

	nets, err := c.Networks(context.Background(), "US", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 2 {
		t.Fatalf("len(networks filtered US) = %d, want 2", len(nets))
	}
	for _, n := range nets {
		if n.Country != "US" {
			t.Errorf("got country %q, want US", n.Country)
		}
	}
}

func TestNetworksLimit(t *testing.T) {
	nets := make([]any, 10)
	for i := range nets {
		nets[i] = map[string]any{
			"id": "net-" + string(rune('a'+i)), "name": "Net",
			"location": map[string]any{
				"city": "X", "country": "US", "latitude": 0.0, "longitude": 0.0,
			},
		}
	}
	payload := map[string]any{"networks": nets}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.HTTP.Transport = rewriteTransport(srv.URL)

	got, err := c.Networks(context.Background(), "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("len(networks limited) = %d, want 3", len(got))
	}
}

func TestStations(t *testing.T) {
	payload := map[string]any{
		"network": map[string]any{
			"id":   "citi-bike-nyc",
			"name": "Citi Bike",
			"stations": []any{
				map[string]any{
					"id":          "s1",
					"name":        "Main St & 1st Ave",
					"latitude":    40.7580,
					"longitude":   -73.9855,
					"timestamp":   "2026-06-14T21:00:00Z",
					"free_bikes":  5,
					"empty_slots": 10,
				},
				map[string]any{
					"id":          "s2",
					"name":        "Park Ave & 42nd St",
					"latitude":    40.7517,
					"longitude":   -73.9754,
					"timestamp":   "2026-06-14T21:00:00Z",
					"free_bikes":  0,
					"empty_slots": 15,
				},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.HTTP.Transport = rewriteTransport(srv.URL)

	stations, err := c.Stations(context.Background(), "citi-bike-nyc")
	if err != nil {
		t.Fatal(err)
	}
	if len(stations) != 2 {
		t.Fatalf("len(stations) = %d, want 2", len(stations))
	}
	s := stations[0]
	if s.ID != "s1" {
		t.Errorf("station.ID = %q, want s1", s.ID)
	}
	if s.FreeBikes != 5 {
		t.Errorf("station.FreeBikes = %d, want 5", s.FreeBikes)
	}
	if s.EmptySlots != 10 {
		t.Errorf("station.EmptySlots = %d, want 10", s.EmptySlots)
	}
	if s.Updated != "2026-06-14T21:00:00Z" {
		t.Errorf("station.Updated = %q, want 2026-06-14T21:00:00Z", s.Updated)
	}
	if s.Lat != "40.7580" {
		t.Errorf("station.Lat = %q, want 40.7580", s.Lat)
	}
}

// rewriteTransport returns an http.RoundTripper that rewrites every request to
// target the given base URL (scheme+host), so the client thinks it is talking
// to the real host while the test server answers.
type rewriteRT struct {
	base string
	orig http.RoundTripper
}

func rewriteTransport(base string) http.RoundTripper {
	return &rewriteRT{base: base, orig: http.DefaultTransport}
}

func (r *rewriteRT) RoundTrip(req *http.Request) (*http.Response, error) {
	base, err := url.Parse(r.base)
	if err != nil {
		return nil, err
	}
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = base.Scheme
	req2.URL.Host = base.Host
	return r.orig.RoundTrip(req2)
}
