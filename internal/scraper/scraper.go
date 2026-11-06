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

// ErrBlocked is returned by an Adapter's Lookup when the source answered,
// but the response looks like an age gate, bot challenge, or other
// interstitial rather than page data — the source is probably locked out,
// not missing the code. Unlike ErrNotFound this is logged by the manager on
// every occurrence, so a site silently locking the scraper out is noticed
// the same day instead of starving metadata for weeks.
var ErrBlocked = errors.New("scraper: source appears blocked")

// ErrOutOfScope is returned by an Adapter's Lookup when the source
// structurally doesn't carry the code shape at all — decided from the code
// alone, with no request made (javbus dropped FC2 entirely; missav and
// javmenu are FC2-only). Unlike ErrNotFound it says nothing about the
// source's health, so the manager skips it silently and it never counts
// toward the source's consecutive not-found streak.
var ErrOutOfScope = errors.New("scraper: code out of scope for this source")

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

	// Series is the JAV series/franchise this release belongs to, if the
	// source distinguishes it from a one-off title. Most releases have none.
	Series string
	// Label is the distributor/imprint brand (e.g. "S1 NO.1 STYLE"), which
	// aggregator sites list separately from the production Studio. Rarely
	// available — only JavBus currently exposes it.
	Label string
	// Rating is normalised to a 0-10 scale (Kodi/Jellyfin convention)
	// regardless of the source site's native scale. Zero means "not rated".
	Rating float64
}

// Adapter is a single metadata source (docs/ARCHITECTURE.md § 2.6).
type Adapter interface {
	Name() string
	Capabilities() Capabilities
	Lookup(ctx context.Context, code string) (*Metadata, error)
}
