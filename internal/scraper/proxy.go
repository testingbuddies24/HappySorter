package scraper

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/testingbuddies24/HappySorter/internal/config"
)

// proxyTransport rewrites every outgoing request through cfgStore's current
// Scraping.ProxyURL, using the query-param pass-through scheme implemented
// by deploy/cf-worker/worker.js (<proxy>/?url=<encoded target>) rather than
// standard forward-proxy semantics, since that's what the shipped Worker
// forwarder expects. Reading cfgStore on every request (instead of once at
// construction) means a Proxy URL saved via the GUI takes effect
// immediately, matching every other hot-reloadable setting.
type proxyTransport struct {
	base     http.RoundTripper
	cfgStore *config.Store
}

// NewProxyTransport wraps http.DefaultTransport so requests are forwarded
// through cfgStore's Proxy URL whenever one is set, and go direct otherwise.
func NewProxyTransport(cfgStore *config.Store) http.RoundTripper {
	return &proxyTransport{base: http.DefaultTransport, cfgStore: cfgStore}
}

func (t *proxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	proxyURL := t.cfgStore.Get().Scraping.ProxyURL
	if proxyURL == "" {
		return t.base.RoundTrip(req)
	}

	target := req.URL.String()
	proxied, err := url.Parse(strings.TrimSuffix(proxyURL, "/") + "/?url=" + url.QueryEscape(target))
	if err != nil {
		return nil, fmt.Errorf("scraper: parsing proxy url %q: %w", proxyURL, err)
	}

	clone := req.Clone(req.Context())
	clone.URL = proxied
	clone.Host = proxied.Host
	return t.base.RoundTrip(clone)
}
