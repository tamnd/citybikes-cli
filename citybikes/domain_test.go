package citybikes

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring, which need no network. The client's HTTP behaviour is
// covered in citybikes_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "citybikes" {
		t.Errorf("Scheme = %q, want citybikes", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "citybikes" {
		t.Errorf("Identity.Binary = %q, want citybikes", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in, typ, id string
	}{
		{"citi-bike-nyc", "network", "citi-bike-nyc"},
		{"bicing", "network", "bicing"},
		{"citi-bike-nyc/stationabc", "station", "citi-bike-nyc/stationabc"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("network", "citi-bike-nyc")
	want := BaseURL + "/v2/networks/citi-bike-nyc"
	if err != nil || got != want {
		t.Errorf("Locate network = (%q, %v), want (%q, nil)", got, err, want)
	}

	got2, err2 := Domain{}.Locate("station", "citi-bike-nyc/s1")
	want2 := BaseURL + "/v2/networks/citi-bike-nyc"
	if err2 != nil || got2 != want2 {
		t.Errorf("Locate station = (%q, %v), want (%q, nil)", got2, err2, want2)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "foo")
	if err == nil {
		t.Error("Locate unknown type: want error, got nil")
	}
}

// TestHostWiring mounts the driver in a kit Host and checks that the domain
// is mounted correctly with the right scheme and ops available.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	info, ok := h.Domain("citybikes")
	if !ok {
		t.Fatal("citybikes domain not mounted")
	}
	if info.Scheme != "citybikes" {
		t.Errorf("Domain scheme = %q, want citybikes", info.Scheme)
	}

	// Classify routes "citi-bike-nyc" to a network URI.
	got, err := h.ResolveOn("citybikes", "citi-bike-nyc")
	if err != nil || got.String() != "citybikes://network/citi-bike-nyc" {
		t.Errorf("ResolveOn = (%q, %v), want citybikes://network/citi-bike-nyc", got.String(), err)
	}
}
