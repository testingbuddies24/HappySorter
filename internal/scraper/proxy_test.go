package scraper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/testingbuddies24/HappySorter/internal/config"
)

type capturingTransport struct {
	got *http.Request
}

func (t *capturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.got = req
	return httptest.NewRecorder().Result(), nil
}

func TestProxyTransportGoesDirectWhenUnset(t *testing.T) {
	cfgStore := config.NewStore("", config.Default())
	capture := &capturingTransport{}
	transport := &proxyTransport{base: capture, cfgStore: cfgStore}

	req, _ := http.NewRequest(http.MethodGet, "https://javdb.com/search?f=all&q=DASS-996", nil)
	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	if capture.got.URL.String() != "https://javdb.com/search?f=all&q=DASS-996" {
		t.Errorf("request was rewritten with no proxy configured: got %s", capture.got.URL.String())
	}
}

func TestProxyTransportRewritesThroughProxy(t *testing.T) {
	cfg := config.Default()
	cfg.Scraping.ProxyURL = "https://my-worker.workers.dev"
	cfgStore := config.NewStore("", cfg)
	capture := &capturingTransport{}
	transport := &proxyTransport{base: capture, cfgStore: cfgStore}

	req, _ := http.NewRequest(http.MethodGet, "https://javdb.com/search?f=all&q=DASS-996", nil)
	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	want := "https://my-worker.workers.dev/?url=https%3A%2F%2Fjavdb.com%2Fsearch%3Ff%3Dall%26q%3DDASS-996"
	if capture.got.URL.String() != want {
		t.Errorf("got %s, want %s", capture.got.URL.String(), want)
	}
	if capture.got.Host != "my-worker.workers.dev" {
		t.Errorf("got Host %q, want my-worker.workers.dev", capture.got.Host)
	}
}
