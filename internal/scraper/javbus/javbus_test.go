package javbus

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/testingbuddies24/HappySorter/internal/scraper"
)

// Fixture trimmed from a live javbus.com response for PRED-897
// (fetched 2026-10-27): the h3 title, the .col-md-3.info panel, and the
// bigImage cover link.
const detailHTML = `<!DOCTYPE html>
<html>
<head><title>PRED-897 大人アオハル - JavBus</title></head>
<body>
<h3>PRED-897 大人アオハル 結婚5年目の記念日に妻と制服いちゃラブ中出しSEX 三好佑香</h3>
<a class="bigImage" href="/pics/cover/clxb_b.jpg"><img src="/pics/cover/clxb_b.jpg"></a>
<div class="col-md-3 info">
	<p><span class="header">識別碼:</span> <span style="color:#CC0000;">PRED-897</span></p>
	<p><span class="header">發行日期:</span> 2026-10-16</p>
	<p><span class="header">長度:</span> 130分鐘</p>
	<p><span class="header">導演:</span> <a href="https://www.javbus.com/director/2nc">U吉</a></p>
	<p><span class="header">製作商:</span> <a href="https://www.javbus.com/studio/i6">プレミアム</a></p>
	<p><span class="header">發行商:</span> <a href="https://www.javbus.com/label/6k8">エレガンス</a></p>
	<p><span class="header">類別:</span></p>
	<p>
		<span class="genre"><label><input type="checkbox" name="gr_sel" value="3i"><a href="https://www.javbus.com/genre/3i">制服</a></label></span>
		<span class="genre"><label><input type="checkbox" name="gr_sel" value="4"><a href="https://www.javbus.com/genre/4">中出</a></label></span>
	</p>
	<ul>
		<li><div class="star-name"><a href="/star/13nd">三好佑香</a></div></li>
	</ul>
</div>
</body>
</html>`

func newTestAdapter(t *testing.T, handler http.HandlerFunc) *Adapter {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	baseURL = server.URL
	return New(server.Client())
}

func TestLookupParsesDetailAndSendsAcceptLanguage(t *testing.T) {
	var gotAcceptLanguage, gotUserAgent string
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		gotAcceptLanguage = r.Header.Get("Accept-Language")
		gotUserAgent = r.Header.Get("User-Agent")
		w.Write([]byte(detailHTML))
	})

	meta, err := a.Lookup(context.Background(), "PRED-897")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	// The October 2026 age/bot gate rejects requests without this header
	// (302 to an empty-body driver-verify interstitial), so its presence is
	// part of the contract, not decoration.
	if gotAcceptLanguage == "" {
		t.Error("request carried no Accept-Language header — the age gate will 302 it")
	}
	if gotUserAgent == "" {
		t.Error("request carried no User-Agent header")
	}
	if got := meta.Actresses; len(got) != 1 || got[0] != "三好佑香" {
		t.Errorf("Actresses = %v, want [三好佑香]", got)
	}
	if meta.Title != "大人アオハル 結婚5年目の記念日に妻と制服いちゃラブ中出しSEX 三好佑香" {
		t.Errorf("Title = %q", meta.Title)
	}
	if meta.ReleaseDate != "2026-10-16" {
		t.Errorf("ReleaseDate = %q, want 2026-10-16", meta.ReleaseDate)
	}
	if meta.Year != 2026 {
		t.Errorf("Year = %d, want 2026", meta.Year)
	}
	if meta.Runtime != 130 {
		t.Errorf("Runtime = %d, want 130", meta.Runtime)
	}
	if meta.Director != "U吉" {
		t.Errorf("Director = %q", meta.Director)
	}
	if meta.Studio != "プレミアム" {
		t.Errorf("Studio = %q", meta.Studio)
	}
	if meta.Label != "エレガンス" {
		t.Errorf("Label = %q", meta.Label)
	}
	if len(meta.Genres) != 2 || meta.Genres[0] != "制服" || meta.Genres[1] != "中出" {
		t.Errorf("Genres = %v", meta.Genres)
	}
	if meta.CoverURL == "" || !strings.HasPrefix(meta.CoverURL, baseURL) {
		t.Errorf("CoverURL = %q, want resolved against %s", meta.CoverURL, baseURL)
	}
}

func TestLookupNotFound404(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	_, err := a.Lookup(context.Background(), "ZZZZ-999")
	if !errors.Is(err, scraper.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// Until October 2026 the gate redirected EVERY request to the
// driver-verify interstitial. If it ever comes back above the header fix,
// the adapter must report not-found on the empty interstitial body rather
// than erroring or returning junk metadata.
func TestLookupEmptyGateRedirect(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", baseURL+"/doc/driver-verify")
		w.WriteHeader(http.StatusFound)
	})

	_, err := a.Lookup(context.Background(), "PRED-897")
	if !errors.Is(err, scraper.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestLookupFC2ImmediateNotFound(t *testing.T) {
	a := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("FC2 lookup must not hit the network")
	})

	_, err := a.Lookup(context.Background(), "FC2-PPV-4956891")
	if !errors.Is(err, scraper.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
