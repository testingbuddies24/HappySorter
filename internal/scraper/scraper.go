// Package scraper defines the pluggable metadata-source interface
// (docs/ARCHITECTURE.md § 2.6) and the manager that tries sources in
// priority order with fallback.
package scraper

import (
	"context"
	"errors"
)

// ErrNotFound is returned by an Adapter's Lookup when the source has no
// record of the given code (as opposed to a network/parse error).
var ErrNotFound = errors.New("scraper: code not found")

// SourceKind classifies where an adapter sits in the fallback order and
// what protections (proxy, age cookie) it typically needs.
type SourceKind string

const (
	KindStudio      SourceKind = "studio"
	KindDistributor SourceKind = "distributor"
	KindAggregator  SourceKind = "aggregator"
)

// Capabilities describes what an Adapter needs from the manager so it can
// be skipped early (with a clear log reason) instead of failing at request
// time (docs/ARCHITECTURE.md § 2.6, § 4.1).
type Capabilities struct {
	NeedsProxy     bool
	NeedsAgeCookie bool
	Kind           SourceKind
}

// Metadata is the normalised result of a successful lookup, and also the
// shape cached in the metadata_cache table (docs/ARCHITECTURE.md § 3).
type Metadata struct {
	Code        string
	Title       string
	Year        int
	ReleaseDate string
	Studio      string
	Director    string
	Runtime     int
	Plot        string
	Actresses   []string
	Genres      []string
	CoverURL    string
	FanartURL   string
	Source      string
}

// Adapter is a single metadata source (docs/ARCHITECTURE.md § 2.6).
type Adapter interface {
	Name() string
	Capabilities() Capabilities
	Lookup(ctx context.Context, code string) (*Metadata, error)
}
