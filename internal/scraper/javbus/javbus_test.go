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

func newTestAdapter(t *testing.T, handler http.HandlerFunc) (*Adapter, string) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return newAdapter(server.Client(), server.URL), server.URL
}

func TestLookupParsesDetailAndSendsAcceptLanguage(t *testing.T) {
	var gotAcceptLanguage, gotUserAgent string
	a, serverURL := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
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
	if meta.CoverURL == "" || !strings.HasPrefix(meta.CoverURL, serverURL) {
		t.Errorf("CoverURL = %q, want resolved against %s", meta.CoverURL, serverURL)
	}
}

func TestLookupNotFound404(t *testing.T) {
	a, _ := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	_, err := a.Lookup(context.Background(), "ZZZZ-999")
	if !errors.Is(err, scraper.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// Until October 2026 the gate redirected EVERY request to the driver-verify
// interstitial with an empty 302 body. That body is a lockout, not a miss:
// it must surface as ErrBlocked (which the manager logs loudly) — reporting
// it as plain not-found is exactly how the October outage hid for weeks.
func TestLookupEmptyGateRedirectIsBlocked(t *testing.T) {
	a, _ := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/doc/driver-verify")
		w.WriteHeader(http.StatusFound)
	})

	_, err := a.Lookup(context.Background(), "PRED-897")
	if !errors.Is(err, scraper.ErrBlocked) {
		t.Errorf("err = %v, want ErrBlocked", err)
	}
}

// Pre-October, the gate's 302 carried the REAL page HTML in its body (see
// the package doc). The adapter must keep parsing that body — treating a
// 302 as terminal not-found would drop every code the day the gate
// re-randomizes to this shape.
func TestLookupRedirectCarryingDetailBodyStillParses(t *testing.T) {
	a, _ := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/doc/driver-verify")
		w.WriteHeader(http.StatusFound)
		w.Write([]byte(detailHTML))
	})

	meta, err := a.Lookup(context.Background(), "PRED-897")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(meta.Actresses) != 1 || meta.Actresses[0] != "三好佑香" {
		t.Errorf("Actresses = %v, want [三好佑香]", meta.Actresses)
	}
}

// If the gate ever serves the interstitial as a 200 page instead of a
// redirect, it must still read as ErrBlocked rather than a silent
// not-found.
func TestLookupGateInterstitialPageIsBlocked(t *testing.T) {
	a, _ := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html><html><body><form id="driver-verify"><label>18 or older?</label><input type="checkbox"></form></body></html>`))
	})

	_, err := a.Lookup(context.Background(), "PRED-897")
	if !errors.Is(err, scraper.ErrBlocked) {
		t.Errorf("err = %v, want ErrBlocked", err)
	}
}

// The site's own HTML not-found page (the shape unknown codes returned
// before the October fix) is a routine miss, not a lockout.
func TestLookupHTMLNotFoundPage(t *testing.T) {
	a, _ := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html><html><body><h1>404 Page Not Found!</h1></body></html>`))
	})

	_, err := a.Lookup(context.Background(), "ZZZZ-999")
	if !errors.Is(err, scraper.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// A detail page for a DIFFERENT code (e.g. a fallback redirect like missav
// serves) must read as a miss — never as metadata, or one video gets
// another video's cast.
func TestLookupWrongCodePageIsNotFound(t *testing.T) {
	a, _ := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(detailHTML)) // page is for PRED-897
	})

	_, err := a.Lookup(context.Background(), "MIDE-001")
	if !errors.Is(err, scraper.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// A detail panel with no readable 識別碼 row means the page shape has
// drifted — ErrBlocked, loudly, rather than half-parsed metadata or a
// silent miss.
func TestLookupDetailPanelWithoutCodeRowIsBlocked(t *testing.T) {
	a, _ := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html><html><body><div class="col-md-3 info"><p><span class="header">發行日期:</span> 2026-10-16</p></div></body></html>`))
	})

	_, err := a.Lookup(context.Background(), "PRED-897")
	if !errors.Is(err, scraper.ErrBlocked) {
		t.Errorf("err = %v, want ErrBlocked", err)
	}
}

// JavBus no longer hosts FC2 at all, so the refusal is out-of-scope (a
// capability boundary, decided with no request) — not a health signal, and
// the manager must not count it toward the source's not-found streak.
func TestLookupFC2ImmediateOutOfScope(t *testing.T) {
	a, _ := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("FC2 lookup must not hit the network")
	})

	_, err := a.Lookup(context.Background(), "FC2-PPV-4956891")
	if !errors.Is(err, scraper.ErrOutOfScope) {
		t.Errorf("err = %v, want ErrOutOfScope", err)
	}
}

// The likeliest drift shape: the value stops being wrapped in a span and
// sits as bare text after the header. The header span must not be mistaken
// for the value — that would compare the label itself against the code and
// yield a silent not-found instead of the loud ErrBlocked.
func TestLookupBareTextCodeRowIsBlocked(t *testing.T) {
	a, _ := newTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html><html><body><div class="col-md-3 info"><p><span class="header">識別碼:</span> PRED-897</p></div></body></html>`))
	})

	_, err := a.Lookup(context.Background(), "PRED-897")
	if !errors.Is(err, scraper.ErrBlocked) {
		t.Errorf("err = %v, want ErrBlocked", err)
	}
}
