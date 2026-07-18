# HappySorter — Development Roadmap

> Status: **Draft v1** (2026-07-13)
> See also: [`SPEC.md`](SPEC.md), [`ARCHITECTURE.md`](ARCHITECTURE.md)

This roadmap sequences the build as **thin vertical slices** — each milestone
is independently runnable and verifiable, so we always have a working binary
and never a big-bang integration at the end. Studio-first source strategy
(from `research/source-test-results.md`) means the first working scraper needs
no proxy, so we can prove the end-to-end pipeline early.

## Milestone 0 — Skeleton that boots ✅ done

**Goal:** `docker run` produces a container that serves an empty dashboard.

- `go mod init`, `cmd/server/main.go`, config load from `/config/config.yaml`
  (generate defaults if missing), `slog` JSON logging to stdout + SQLite.
- HTTP server (stdlib `net/http`, Go 1.22+ method/pattern routing — no
  framework dependency needed yet for two routes) with `/` dashboard and
  `/healthz`.
- SQLite open + migrations (`config`, `files`, `metadata_cache`, `logs`, `scrape_sources`),
  embedded via `go:embed` under `internal/database/migrations/`.
- Dockerfile (multi-stage, alpine, non-root UID 1000), docker-compose.yml.

**Verify:** binary built and run locally — `/healthz` returns 200 with
`{version, uptime_seconds, queue_size}`; dashboard renders; `config.yaml`
and `happy-sorter.db` created on first run with all 5 tables plus
`schema_migrations`; log records land in both stdout (JSON) and the `logs`
table. `go vet` and `gofmt -l` clean. (Docker image build not yet verified
in this environment — Docker isn't installed on this dev machine; the
Dockerfile follows the same multi-stage pattern documented in
`DEPLOYMENT.md` and should be verified on first NAS/Docker deploy.)

## Milestone 1 — Watcher → filter → review (no scraping) ✅ done

**Goal:** dropped files get triaged into review folders correctly.

- `fsnotify` watcher on `/watch` with polling fallback (60s) and an initial
  full scan on startup, so nothing dropped while offline is missed.
- Rubbish filter (extension allow-list, 50MB size floor, junk-extension and
  junk-substring patterns) → `review/_filter/`.
- Code extractor (normalise + regex `^([A-Z0-9]{2,5})-?(\d{2,5})$`, release-suffix
  stripping) → on miss, `review/_unmatched/`.
- Cross-device-safe move helper (rename first, copy+rename+remove fallback for
  when `/watch` and `/library` are separate volumes).
- `files` table records every seen file + its state; `Seen()` lookup makes
  processing idempotent across restarts, regardless of which of the three
  detection paths (startup scan, fsnotify, poll) re-emits a path.
- `/healthz`'s `queue_size` now reports the live count of files in `scrape`
  state (extracted, awaiting Milestone 2's scraper).

**Verify:** dropped `SSIS-001.mp4` (51MB) → stayed in `/watch`, `files` row
`state=scrape, code=SSIS-001`; `notes.txt` → moved to `review/_filter/`,
`state=review_filter, reason="junk extension .txt"`; `random.mp4` (51MB, no
code) → moved to `review/_unmatched/`, `state=review_unmatched, reason="no
JAV code found in filename"`. Restarted the process with `SSIS-001.mp4`
still in place → no new log entries, no duplicate `files` rows, `queue_size`
unchanged. `go build`, `go vet`, `gofmt -l` all clean. (Verified by running
the binary directly against a scratch watch/library tree; Docker image
build still unverified in this environment, per M0's note.)

Also fixed while building this milestone: `docker-compose.yml`, `README.md`,
and `DEPLOYMENT.md` previously mounted `/watch` read-only (`:ro`), which
directly contradicted this milestone's requirement to move files out of
`/watch` — corrected to a writable mount in all three places.

## Milestone 2 — First scraper (S1) → organise → NFO ✅ done

**Goal:** full pipeline end-to-end for a studio-direct code, no proxy.

- Scrape manager (`internal/scraper`) + `Adapter` interface + `Manager.Lookup`
  fallback loop; `Manager.Empty()` preserves Milestone 1's "no scraper
  enabled yet, stay queued" behaviour when zero sources are enabled.
- `s1` adapter (`internal/scraper/s1`): studio-direct, no Cloudflare, no age
  gate. Verified live against `s1s1s1.com`: the detail page sits at a
  predictable URL (`/works/detail/<CODE-NO-HYPHEN>`), so no search step is
  needed; unknown codes return HTTP 200 with a generic page rather than a
  real 404, so "not found" is detected by the absence of the title element.
- Organiser (`internal/organiser`): creates `<CODE> (<YEAR>)/`, downloads
  `poster.jpg` + `fanart.jpg` (S1 has no separate wide/backdrop asset, so
  the box cover is reused for both), writes `backdrop.jpg` as an alias of
  `fanart.jpg`, moves+renames the video via the shared `internal/fsutil`
  move helper (promoted out of `internal/pipeline` so both packages use the
  same cross-device-safe move).
- NFO writer (`internal/nfo`): Kodi `movie.nfo` XML (title, plot, runtime,
  premiered, year, studio, director, genre[], actor[], uniqueid).
- `metadata_cache` (`internal/store/metadata.go`) populated on every
  successful scrape; a second file with the same code hits the cache and
  skips the HTML scrape entirely (verified: both files land in the same
  `<CODE> (<YEAR>)/` folder — real multi-disc behaviour, not just avoided
  re-work). Note: `cover_path`/`fanart_path` currently cache the *source
  URL*, not a local path — the organiser still re-downloads images on a
  cache hit, so only the scrape+parse step is actually saved. Local-image
  reuse is a possible fast-follow, not done here.
- Deliberately deferred out of this slice (not forgotten): `actors/<name>.jpg`
  per-actress photos and `thumb.jpg`. Both need extra scraping (an actress
  detail-page fetch each) beyond what this milestone's verify step requires;
  picking them up is a small addition whenever the GUI/polish milestones
  need them.

**Verify:** ran the built binary directly (no Docker in this dev environment,
consistent with M0/M1's caveat) against a scratch watch/library tree with
`s1` enabled in config:
- Real code `SSIS-001.mp4` (60MB) → scraped live from `s1s1s1.com`,
  organised within ~2s into `/library/SSIS-001 (2021)/` containing the
  renamed video, `poster.jpg`, `fanart.jpg`, `backdrop.jpg`, and a
  `movie.nfo` with correct title/plot/runtime/genres/actresses/director in
  Japanese — `files.state=done`.
- Second file, same code, different container (`SSIS-001.mkv`) → logged
  "metadata cache hit, skipping scrape", landed in the *same* release
  folder alongside the first file (multi-disc).
- Well-formed but nonexistent code (`ZFAK-999.mp4`) → all sources failed,
  routed to `review/_unmatched/` with `state=failed,
  reason="scrape failed: all sources failed for code ZFAK-999"`.
- `go build`, `go vet`, `gofmt -l` all clean.
- Not verified in this environment: pointing an actual Jellyfin instance at
  `/library` and confirming it renders the metadata (no Jellyfin install
  available here) — the NFO/image/folder layout matches the Kodi schema
  Jellyfin expects, but this last hop is unverified.

### Addendum — duplicate-destination handling

The organiser no longer auto-suffixes when a file already sits at the
computed video destination (previously it would have via
`fsutil.UniquePath`). It now computes the destination path first, before
any side effect (folder creation, image download, NFO write), and returns
a typed `*organiser.DuplicateError` on collision. The pipeline routes this
case to a new `review/_duplicate/` folder with `state=review_duplicate`,
distinct from the generic `failed` path — the existing organised release
is left completely untouched, and the incoming file is left for the user
to compare and resolve by hand.

**Verify:** built the binary and ran it against a persistent `testbed/`
folder (see `testbed/README.md`) rather than a throwaway scratch dir:
- `SSIS-001.mp4` (60MB) organised normally into `SSIS-001 (2021)/`.
- A second file with the same code and extension (`SSIS-001-UC.mp4`,
  normalises to the same code) hit the metadata cache, then the organiser's
  new collision check fired — logged `"duplicate file, routing for manual
  review"` with the existing path, landed untouched in
  `review/_duplicate/`, `state=review_duplicate`.
- Confirmed the original `SSIS-001 (2021)/SSIS-001 (2021).mp4` was
  byte-for-byte unchanged (same size/mtime/checksum) after the collision.
- `go build`, `go vet`, `gofmt -l` all clean.

## Milestone 3 — Setup GUI (folders, sources, rename) ✅ done

**Goal:** everything configurable without editing YAML by hand.

- Plain HTML forms (stdlib `net/http` + `html/template`, Post/Redirect/Get
  with query-param flash messages) instead of HTMX — avoids an external
  CDN/vendored-JS dependency for a self-hosted NAS tool with uncertain
  internet access; no functional requirement below needs JS.
- `/setup/folders`, `/setup/sources` (enable/priority/QPS per source),
  `/setup/rename` (folder/file templates + unknown-year placeholder).
- `/review` list, grouped by `review_filter` / `review_unmatched` /
  `review_duplicate` / `failed`, with retry/delete actions per row and a
  bulk `/review/empty`.
- `/rescan`, `/pause`, `/resume` controls; dashboard shows per-state counts,
  paused/running status, and recent activity.
- `/logs` viewer (level filter + limit) backed by the existing `logs` table.
- Config writes persist to `config.yaml` and hot-reload without a restart:
  `config.Store` (copy-on-write, `internal/config/store.go`) is now read
  fresh by the organiser and pipeline on every call instead of being
  captured once at startup; `scraper.ManagerStore`
  (`internal/scraper/store.go`) lets `/setup/sources` rebuild the adapter
  list live. Only the `watch` path and server port still require a restart
  (flagged with a warning banner when changed via the GUI).
- `Watcher` gained `Pause()`/`Resume()`/`Rescan()`, backing the dashboard
  controls; `Pipeline.Retry()` lets `/review`'s retry button reprocess a
  file from its current on-disk path, bypassing the original path's
  `Seen()` record.
- New `internal/scraper/registry` package (`BuildAdapters`) factors the
  name→adapter switch out of `cmd/server/main.go` so both it and
  `internal/httpserver` can build a `*scraper.Manager` without an import
  cycle (adapter subpackages import `scraper`, so the registry can't live
  inside `scraper` itself).

**Verify:** ran the built binary against `testbed/` (see `testbed/README.md`):
- Dashboard, `/setup/folders`, `/setup/sources`, `/setup/rename`, `/review`,
  `/logs` all render 200 with real data (`/setup/sources` correctly showed
  `s1` pre-checked from `testbed/config/config.yaml`).
- Dropped `SSIS-777.mp4` (60MB) with `s1` enabled → scraped live, organised
  into `SSIS-777 (2023)/`, showed up in the dashboard's recent activity and
  the `Organised` count.
- Dropped a same-code file that got briefly misclassified as filtered
  (a pre-existing fsnotify create-before-write race, not a Milestone 3
  bug — see below) → used `/review`'s **Retry** button, which reprocessed
  it from its review-folder path and correctly re-routed it to
  `review_duplicate` this time, with the collision reason recorded. Then
  used **Delete**, which removed the file from disk and its row.
- **Pause** → dropped a file → it sat untouched in `watch/` (`queue_size`
  didn't change). **Resume** → its auto-triggered rescan picked the file up
  and processed it immediately, no restart needed.
- Disabled `s1` via `/setup/sources` (unchecked the box, submitted) →
  `config.yaml` updated (`enabled: false`) and the live `ManagerStore` was
  rebuilt in the same request — a file dropped immediately after correctly
  queued in `scrape` state (no network call) instead of trying `s1`, proving
  the config change applied without a restart. Re-enabled `s1` the same way
  to restore the testbed's default state.
- `go build`, `go vet`, `gofmt -l` all clean.

Not fixed in this milestone (pre-existing, identified during testing, out
of scope for the GUI work): a slow/racy file write into `/watch` can have
its `Create` event fire before the file is fully sized, so the rubbish
filter misclassifies a good file as empty; because `Seen()` blocks
reprocessing of any previously-recorded path, this is a permanent
misclassification without a manual `/review` retry (which now exists as
of this milestone, so it's at least recoverable, just not automatic).
Also out of scope: files queued in `scrape` state (code extracted, but no
source enabled yet) have no retry path from `/review`, since that page
only lists review/failed states — enabling a source later doesn't
automatically drain that queue. **Fixed in Milestone 4a** via
`Pipeline.DrainQueued`.

## Milestone 4a — Studio adapters + scrape-queue drain ✅ done

**Goal:** a second working source (fallback becomes real, not theoretical)
plus the M3 known-gap where enabling a source never drained files already
sitting in `scrape` state.

- Live-probed all three originally planned no-proxy studio/distributor
  sources before writing any selector code (`s1.go`'s methodology).
  **Two of three are not viable without a proxy, contradicting this
  roadmap's original assumption that studio-direct sources "work from any
  IP, no proxy":**
  - `ideapocket.com` — confirmed working, same CMS family as S1 (near-
    identical selectors; div-based `.th`/`.td` instead of table cells, but
    the same class-selector queries carry over unchanged). Implemented as
    `internal/scraper/ideapocket`.
  - SOD Prime (`ec.sod.co.jp`) — every path returns HTTP 403 regardless of
    headers/User-Agent; the 403 lives on the nginx front door itself
    (`/prime/info.php`), not a Cloudflare JS challenge. `ec.sod.co.jp` is
    DMM-family, and this doc's own DMM/FANZA research already flagged that
    family as Japan-only geo-blocked — this looks like the same
    restriction, not a bot-check a proxy-less adapter could work around.
  - MGStage (`mgstage.com`) — HTTP 403 from CloudFront directly
    ("Request blocked"), the textbook CloudFront geo-restriction response,
    not a bot challenge. The `adc=1` age-cookie assumption in the original
    plan was never the blocker.
  - Decision: `sodprime`/`mgstage` are **dropped from the roadmap**, not
    deferred. An M4b-style Cloudflare-bypass proxy doesn't fix a
    country-level geo-block — that would need a Japan-routed egress
    specifically, and S1 + IdeaPocket + the M4b aggregators already give
    solid fallback coverage without it. Removed from
    `config.Default().Sources` (`internal/config/config.go`) accordingly.
- `Pipeline.DrainQueued` (`internal/pipeline/pipeline.go`) reprocesses
  every file in `scrape` state, clearing its stale record first (same
  pattern as `/review`'s retry) so `Seen()` doesn't block it. Called from
  `/setup/sources` in a goroutine using `context.Background()` (not the
  request context, which is cancelled the moment the handler returns) so
  the Post/Redirect/Get response isn't blocked by live scrapes.
- `internal/scraper/registry` gained an `ideapocket` case.

**Verify:** ran the built binary against `testbed/`:
- `go build ./...`, `go vet ./...`, `gofmt -l .` all clean.
- Dropped `IPZZ-877.mp4` with only `ideapocket` enabled → organised into
  `IPZZ-877 (2026)/` with a fully-populated NFO (title, plot, runtime,
  release date, genres, actress, studio) and cover/fanart images.
- Enabled `s1` + `ideapocket`, dropped `IPX-001.mp4` (not in S1's catalogue)
  → resolved correctly via `ideapocket` (confirmed by the NFO's `<studio>`
  tag), proving fallback ordering works.
- Dropped `ZZZZ-999.mp4` (well-formed, no source has it) with both sources
  enabled → routed to `review/_unmatched/` with reason `all sources failed
  for code ZZZZ-999`.
- Disabled all sources, dropped `IPZZ-878.mp4` → sat in `scrape` state in
  `watch/`. Re-enabled `ideapocket` via `/setup/sources` → the file was
  picked up and reprocessed automatically within seconds, no restart, no
  manual `/review` retry — the M3 known-gap is closed.

## Milestone 4b — Proxy infra + aggregators ✅ done

**Goal:** resilience via the aggregator sites, now that M4a proved the
fallback mechanics work end-to-end on a real second source.

- Live-probed all three planned aggregators before writing any selector
  code. **Findings changed the milestone's scope twice, each time checked
  with the user before proceeding:**
  - `javbus.com` — no proxy or cookie needed. Every request (found or not)
    gets an HTTP 302 to an age-verification interstitial regardless of
    cookies sent, but the 302 response body already contains the real page
    HTML (or a real "404 Page Not Found!" page for a bad code) — the
    adapter just disables redirect-following and parses whichever body
    comes back. Implemented as `internal/scraper/javbus`.
  - `javdb.com` — resolves directly, no Cloudflare challenge and no age
    gate in this environment, contradicting the original
    `docs/research/source-test-results.md` assumption that it needed a
    proxy. Two-request lookup (search page to find the matching result
    card, then the detail page for the metadata panel). Implemented as
    `internal/scraper/javdb`.
  - `javlibrary.com` — genuinely Cloudflare-challenged (`Cf-Mitigated:
    challenge`, active Turnstile-style JS challenge), and there was no
    working proxy in this environment to verify real selectors against.
    **Decision: ship the proxy plumbing this milestone regardless, defer
    only the adapter itself** — `internal/scraper/registry` has no
    `javlibrary` case yet; it falls through to the existing "source
    enabled but no adapter implemented" warning log, ready to add once a
    proxy is available to probe through.
  - Net effect: since only `javlibrary` needs it, cookie-persistence
    machinery (`cookies_dir`) was **not** built — only the `proxy_url`
    config field, its GUI field, and `deploy/cf-worker/worker.js` shipped,
    ready for whenever the adapter is added.
- `internal/scraper/registry` gained `javbus`/`javdb` cases.
- `/setup/sources` (`internal/httpserver/setup.go`) gained a Proxy URL
  field, wired to `config.Scraping.ProxyURL`, saved via the same
  copy-on-write `cfgStore.Update` hot-reload path as every other setting.
- `deploy/cf-worker/worker.js`: minimal Cloudflare Worker pass-through
  forwarder — HappySorter calls it as `<worker-url>/?url=<encoded target>`
  and it fetches the target from Cloudflare's own edge, for whenever a
  Cloudflare-gated source needs it.
- Bug found and fixed during E2E verification: `organiser.download()`
  (`internal/organiser/organiser.go`) sent no `User-Agent`/`Referer` at
  all, so JavBus's cover-image CDN (hotlink-protected on `Referer`, not
  just User-Agent) 403'd every download. Fixed by adding both headers,
  with the `Referer` derived from the image URL's own scheme+host.
- Two data-correctness bugs found and fixed in `internal/scraper/javbus`
  during E2E verification:
  - The release-date parser did `TrimPrefix(text, label+":")`, but `label`
    (from the `.header` span) already includes the trailing colon in the
    raw HTML — so the prefix never matched and `<premiered>` ended up
    holding the raw Chinese-labelled string, with `<year>` never parsed
    out of it (folder named `SSIS-001 (Unknown)` instead of `(2021)`).
    Fixed by trimming `label` alone.
  - The genre selector (`.genre a`) also matched a second, unrelated block
    of hover-card spans that JavBus reuses the `genre` CSS class for,
    further down the same info column, wrapping the actress links — so
    both actress names leaked into `<genre>` on top of their correct
    `<actor>` entries. Real genre tags are wrapped in a `<label>` (around
    the tag's own checkbox input); the actress hover-spans aren't. Fixed
    by scoping to `.genre label a`.

**Verify:** ran the built binary against `testbed/`:
- `go build ./...`, `go vet ./...`, `gofmt -l .` all clean.
- Dropped `SSIS-001.mp4` with only `javbus` enabled → organised into
  `SSIS-001 (2021)/` with a fully-populated NFO (correct title, runtime,
  `2021-02-18` premiered date, `2021` year, correct 8-entry genre list, no
  actress-name duplicates, studio, director) and cover/fanart/backdrop
  images (184.4K each, downloaded successfully past the CDN's
  Referer-hotlink check).
- Same code, only `javdb` enabled → resolved independently via a
  different data source (`2021-02-19` premiered date — a one-day
  discrepancy between the two sites' own data, not a bug — correct
  6-entry genre list, no actress duplication, cover/fanart images).
- Enabled `s1` + `ideapocket` + `javbus` + `javdb` together, dropped
  `SSIS-001.mp4` again → resolved cleanly with all four adapters wired
  into the same priority-ordered fallback chain, no crash, correct output.
- Dropped `ABC-999.mp4` (well-formed, no source has it) with all four
  sources enabled → every adapter tried and missed, routed to
  `review/_unmatched/` with reason `all sources failed for code ABC-999`.

## Milestone 5 — Hardening & release

**Goal:** ship a v1.0.0 image.

- Non-root container, `read_only` FS, `no-new-privileges`.
- Multi-arch build (`linux/amd64,linux/arm64`) pushed to GHCR.
- Placeholder-poster generation, error-path polish, backup/restore doc pass.
- README badges, versioned tag, `LICENSE`.

**Verify:** fresh-NAS install from README quickstart succeeds end-to-end;
success criteria in `SPEC.md § 7` all pass.

## Dependency order

```
M0 ──▶ M1 ──▶ M2 ──▶ M3 ──▶ M4a ──▶ M4b ──▶ M5
skeleton  triage  1 scraper  GUI    2nd source  aggregators  release
                  +organise         +queue drain +proxy infra (done)
```

Each milestone is a mergeable PR. M2 is the "does the core idea work" proof
point; everything after is breadth and resilience.
