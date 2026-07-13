// Package geoip enriches messages with geographic data from a MaxMind- or
// IPinfo-format .mmdb database. No database is bundled (licensing); the operator
// supplies a path. When no database is configured the resolver is a no-op.
package geoip

import (
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

// Resolver looks up geo data for IP addresses. A nil-db Resolver is disabled and
// safe to call.
type Resolver struct {
	mu sync.RWMutex
	db *maxminddb.Reader
}

// record covers the common subset of MaxMind City and IPinfo mmdb schemas.
type record struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
		Names   struct {
			EN string `maxminddb:"en"`
		} `maxminddb:"names"`
	} `maxminddb:"country"`
	City struct {
		Names struct {
			EN string `maxminddb:"en"`
		} `maxminddb:"names"`
	} `maxminddb:"city"`
	Location struct {
		Latitude  float64 `maxminddb:"latitude"`
		Longitude float64 `maxminddb:"longitude"`
	} `maxminddb:"location"`
	// IPinfo-style flat fields.
	CountryCode string `maxminddb:"country"`
	CityName    string `maxminddb:"city"`
}

// New returns a resolver. An empty path yields a disabled (no-op) resolver.
func New(path string) (*Resolver, error) {
	if path == "" {
		return &Resolver{}, nil
	}
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening geoip db %q: %w", path, err)
	}
	return &Resolver{db: db}, nil
}

// Enabled reports whether a database is loaded.
func (r *Resolver) Enabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.db != nil
}

// Lookup returns geo fields for an IP: country, city and location ("lat,lon").
func (r *Resolver) Lookup(ipStr string) (map[string]any, bool) {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return nil, false
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, false
	}
	var rec record
	if err := db.Lookup(ip, &rec); err != nil {
		return nil, false
	}

	out := map[string]any{}
	if c := firstNonEmpty(rec.Country.ISOCode, rec.CountryCode); c != "" {
		out["country"] = c
	}
	if rec.Country.Names.EN != "" {
		out["country_name"] = rec.Country.Names.EN
	}
	if c := firstNonEmpty(rec.City.Names.EN, rec.CityName); c != "" {
		out["city"] = c
	}
	if rec.Location.Latitude != 0 || rec.Location.Longitude != 0 {
		out["location"] = formatLatLon(rec.Location.Latitude, rec.Location.Longitude)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// Close releases the database.
func (r *Resolver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.db != nil {
		err := r.db.Close()
		r.db = nil
		return err
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func formatLatLon(lat, lon float64) string {
	return strconv.FormatFloat(lat, 'f', -1, 64) + "," + strconv.FormatFloat(lon, 'f', -1, 64)
}
