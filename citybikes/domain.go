package citybikes

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes citybikes as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/citybikes-cli/citybikes"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// citybikes:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone citybikes binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the citybikes driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "citybikes",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "citybikes",
			Short:  "Go CLI for Citybik.es — real-time bike sharing data for 800+ networks worldwide",
			Long: `Go CLI for Citybik.es — real-time bike sharing data for 800+ networks worldwide

citybikes reads public Citybik.es data over plain HTTPS, shapes it into
clean records, and prints output that pipes into the rest of your tools. No API
key, nothing to run alongside it.`,
			Site: Host,
			Repo: "https://github.com/tamnd/citybikes-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// networks: list all bike-sharing networks, optionally filtered by country.
	kit.Handle(app, kit.OpMeta{
		Name:    "networks",
		Group:   "read",
		List:    true,
		Summary: "List bike-sharing networks",
		URIType: "network",
	}, listNetworks)

	// stations: list docking stations for a given network ID.
	kit.Handle(app, kit.OpMeta{
		Name:    "stations",
		Group:   "read",
		List:    true,
		Summary: "List stations for a bike-sharing network",
		URIType: "station",
		Args:    []kit.Arg{{Name: "network", Help: "network ID (e.g. citi-bike-nyc)"}},
	}, listStations)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type networksIn struct {
	Country string  `kit:"flag" help:"2-letter country code filter (e.g. US)"`
	Limit   int     `kit:"flag,inherit" help:"max results"`
	Client  *Client `kit:"inject"`
}

type stationsIn struct {
	Network string  `kit:"arg" help:"network ID (e.g. citi-bike-nyc)"`
	Client  *Client `kit:"inject"`
}

// --- handlers ---

func listNetworks(ctx context.Context, in networksIn, emit func(*Network) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 100
	}
	nets, err := in.Client.Networks(ctx, in.Country, limit)
	if err != nil {
		return mapErr(err)
	}
	for _, n := range nets {
		if err := emit(n); err != nil {
			return err
		}
	}
	return nil
}

func listStations(ctx context.Context, in stationsIn, emit func(*Station) error) error {
	if in.Network == "" {
		return errs.Usage("network ID is required (e.g. citi-bike-nyc)")
	}
	stations, err := in.Client.Stations(ctx, in.Network)
	if err != nil {
		return mapErr(err)
	}
	for _, s := range stations {
		if err := emit(s); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver ---

// Classify turns a reference into (type, id). Citybik.es has two resource
// types: "network" and "station". A bare id with a slash is treated as a
// station (network/stationid); otherwise it's a network.
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty reference")
	}
	parts := strings.SplitN(input, "/", 2)
	if len(parts) == 2 && parts[1] != "" {
		return "station", input, nil
	}
	return "network", parts[0], nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "network":
		return BaseURL + "/v2/networks/" + id, nil
	case "station":
		parts := strings.SplitN(id, "/", 2)
		return BaseURL + "/v2/networks/" + parts[0], nil
	default:
		return "", errs.Usage("citybikes has no resource type %q", uriType)
	}
}

// mapErr converts a library error into the kit error kind that carries the
// right exit code.
func mapErr(err error) error {
	return err
}
