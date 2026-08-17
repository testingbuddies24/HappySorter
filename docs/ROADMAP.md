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

- `fsnotify` watcher on `/download` with polling fallback (60s) and an initial
  full scan on startup, so nothing dropped while offline is missed.
- Rubbish filter (extension allow-list, 50MB size floor, junk-extension and
  junk-substring patterns) → `review/_filter/`.
- Code extractor (normalise + regex `^([A-Z0-9]{2,5})-?(\d{2,5})$`, release-suffix
  stripping) → on miss, `review/_unmatched/`.
- Cross-device-safe move helper (rename first, copy+rename+remove fallback for
  when `/download` and `/sorted` are separate volumes).
- `files` table records every seen file + its state; `Seen()` lookup makes
  processing idempotent across restarts, regardless of which of the three
  detection paths (startup scan, fsnotify, poll) re-emits a path.
- `/healthz`'s `queue_size` now reports the live count of files in `scrape`
  state (extracted, awaiting Milestone 2's scraper).

**Verify:** dropped `SSIS-001.mp4` (51MB) → stayed in `/download`, `files` row
`state=scrape, code=SSIS-001`; `notes.txt` → moved to `review/_filter/`,
`state=review_filter, reason="junk extension .txt"`; `random.mp4` (51MB, no
code) → moved to `review/_unmatched/`, `state=review_unmatched, reason="no
JAV code found in filename"`. Restarted the process with `SSIS-001.mp4`
still in place → no new log entries, no duplicate `files` rows, `queue_size`
unchanged. `go build`, `go vet`, `gofmt -l` all clean. (Verified by running
the binary directly against a scratch watch/sorted tree; Docker image
build still unverified in this environment, per M0's note.)

Also fixed while building this milestone: `docker-compose.yml`, `README.md`,
and `DEPLOYMENT.md` previously mounted `/download` read-only (`:ro`), which
directly contradicted this milestone's requirement to move files out of
`/download` — corrected to a writable mount in all three places.

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
consistent with M0/M1's caveat) against a scratch watch/sorted tree with
`s1` enabled in config:
- Real code `SSIS-001.mp4` (60MB) → scraped live from `s1s1s1.com`,
  organised within ~2s into `/sorted/SSIS-001 (2021)/` containing the
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
  `/sorted` and confirming it renders the metadata (no Jellyfin install
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
of scope for the GUI work): a slow/racy file write into `/download` can have
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

## Milestone 5 — Hardening & release 🔶 in progress

**Goal:** ship a v1.0.0 image.

- Non-root container, `read_only` FS, `no-new-privileges` — done, but
  *reasoned* safe rather than run-verified (no Docker on this dev machine,
  consistent with M0–M4b). The `Dockerfile` already ran as UID 1000 (since
  M0); `docker-compose.yml` now turns on `read_only: true`,
  `no-new-privileges:true`, and `user: "1000:1000"` by default rather than
  as a commented-out "optional hardening" suggestion. Reasoned safe by
  grepping every filesystem write in the codebase
  (`os.Create`/`os.WriteFile`/`os.MkdirAll`/`os.CreateTemp` across
  `config`, `fsutil`, `nfo`, `organiser`) — all of them land under
  `/config`, `/sorted`, or `/download`, the three bind-mounted volumes. One
  caveat grep can't rule out: the `modernc.org/sqlite` driver could in
  principle want scratch space outside `/config` in some mode; this hasn't
  been exercised under an actual read-only container yet, so treat this as
  "should work" until someone runs it on real hardware.
- Placeholder-poster generation — done. `SPEC.md` F5 required this but it
  was never implemented. The old behavior was inconsistent: an empty
  `CoverURL` organised the file with no `poster.jpg` at all (no error), but
  a *non-empty* URL that failed to download returned a hard error, routing
  the whole file to `review/_unmatched/` as Failed even though the metadata
  scrape itself had succeeded. New `internal/organiser/placeholder.go`
  (`golang.org/x/image` — `basicfont` + `font.Drawer`) renders a plain
  400×600 JPEG with the code text centered on it; `Organise` now falls back
  to it in both cases (empty URL or failed download) instead of leaving the
  poster missing or failing the item. Fanart got the same non-fatal
  treatment (a missing/failed fanart just means no
  `fanart.jpg`/`backdrop.jpg`, not a failed item). Note the tradeoff: a
  *transient* download failure now writes a placeholder and marks the item
  `done` permanently — the metadata cache means the real cover is never
  retried later, whereas the old Failed-state was retryable via `/review`.
  Judged worth it (a placeholder beats an item stuck in review over a
  network blip), but it is a real behavior change, not pure polish.
  Verified: unit test decodes the generated JPEG and checks dimensions;
  live testbed run with `s1` confirms the untouched happy path still pulls
  the real 500 KB cover (not a placeholder) when the source has one. The
  failure→placeholder branch itself was not exercised end-to-end (only unit
  + happy-path tested) — the logic is small and `os.Create` safely
  overwrites any partial download, so this was judged low-risk rather than
  blocking on it.
- README badges + `LICENSE` — done. `LICENSE` (MIT) already existed since
  an earlier milestone. Added CI/Release GitHub Actions badges (below) plus
  a static license badge to `README.md`.
- Error-path polish — the cover/fanart change above *is* the error-path
  polish this bullet meant; no other unhandled-error paths were found worth
  chasing at this size.
- Backup/restore doc pass — `DEPLOYMENT.md § 7` already documented this
  accurately as of M4b; nothing stale to correct.
- Multi-arch build (`linux/amd64,linux/arm64`) pushed to GHCR — **prepped,
  not executed.** Added `.github/workflows/release.yml`: builds
  `linux/amd64,linux/arm64` via `docker buildx` + QEMU and pushes to
  `ghcr.io/testingbuddies24/happy-sorter` on any `v*.*.*` tag push. Also
  added `.github/workflows/ci.yml` (`go build`/`go vet`/`gofmt -l` on every
  push/PR to `main`) since there was no CI at all before this. Neither
  workflow has been triggered yet — pushing a version tag is a deliberate,
  externally-visible release action (publishes a public image), left for
  an explicit go-ahead rather than done in-line with everything else here.
- Versioned tag — **not cut yet**, same reasoning as above.

**Verify:** fresh-NAS install from README quickstart succeeds end-to-end;
success criteria in `SPEC.md § 7` all pass. Blocked on cutting the release
tag (see above) — everything else in this milestone is done and pushed.

### Addendum — production hardening: recognizer, FC2, SQLite, watcher

Prompted by a real NAS deployment log review (Aug 14–17, 3.5 days): 32 files
failed code extraction and needed manual renaming (the reported pain point),
alongside 3,036 `SQLITE_BUSY` errors, 2,567 watcher-channel-full drops, and
1,440 stale-path warnings.

- **`internal/pipeline/code.go` rewritten.** The old extractor required the
  *whole* normalised filename to match `^[A-Z0-9]{2,5}-?\d{2,5}$` after a
  bounded suffix-stripping loop — any unrecognised trailing noise (a site
  domain, `_CH`/`_4K`, a digit-bearing multi-part tail) failed the match
  outright. The new `ExtractCode` tokenizes on any non-alphanumeric run,
  blanks out dotted-domain tokens (`NYAP2P.COM`, `HHD800.COM`) as a whole
  unit first (done as a substring replace *before* tokenizing — tokenizing
  itself splits on `.`, so a post-split per-token domain check would never
  see the dot), cuts a `site@` watermark prefix, and scans tokens for the
  first prefix+number pair or glued match. This tolerates surrounding noise
  instead of requiring an exact whole-name match. Also added: an `FC2(-PPV)?-#######`
  branch normalising to `FC2-PPV-#######`; multi-part support (`ExtractCode`
  now returns `(code, part, ok)` — a third `-1`/`-2`/`-CD1` token, digit-
  terminated so it can't be confused with a quality marker like `4K` or a
  Chinese-sub marker like `CH`); and a one-level parent-folder fallback when
  the filename itself yields nothing (rescues watermark-tool output sitting
  in a folder named after the real code).

  **Conscious behaviour reversals** (previously-asserted "must not match"
  test cases that now correctly extract a code): `HHD800.COM-DASS-996-AI`
  and `[HHD800.COM] DASS-996` now yield `DASS-996` — that IS the real
  release, and rejecting it was the reported pain. `DASS-996-CD1` now
  yields `DASS-996` + part `CD1` (multi-part is a shipped goal, not a
  guard). `CAWD-991 (1)` now yields `CAWD-991` + part `1` — a **known
  trade-off**: a browser-style re-download suffixed `(1)` now organises as
  a second "part" beside the original instead of routing to
  `TBC/_duplicate`. Judged acceptable (fewer files stuck in review beats
  the rare case of a true re-download landing as `-1`), and an
  identically-named re-download still hits `DuplicateError` normally.

- **`internal/organiser/organiser.go`** gained a `part` parameter. Multi-
  part files share one release folder (`FTKD-040 (2026)/` holding both
  `FTKD-040 (2026)-1.mp4` and `-2.mp4`), with sidecars written once by
  whichever part creates the folder. Duplicate detection now branches on
  `part`: part-less files keep the original folder-exists check; part-
  carrying files check their own destination filename instead, so a true
  re-drop of the same part still correctly lands in `TBC/_duplicate`
  (verified in the testbed: `dal-011/uncoded-video.mp4` organised via the
  parent-folder fallback, then the real `masex.tv@dal-011.mp4` for the same
  release correctly hit `DuplicateError`).

- **FC2 scraping is plumbing-only — every source is externally dead as of
  2026-08-17.** Live-probed before writing any code (this project's
  standing methodology, per M4a/M4b): `fc2.javbus.com` has **no DNS record
  at all**, confirmed against both Google's and Cloudflare's public
  resolvers directly against `javbus.com`'s own authoritative nameservers —
  not a geo-block, the subdomain appears down or retired. `javdb.com`'s FC2
  search results resolve to a distinct `FC2-#######` label (no `PPV`) that
  cleanly misses the exact-match adapter, and FC2 detail pages 302 to a
  login wall regardless (regular codes are unaffected). `fc2ppvdb.com`
  returns Cloudflare 526 (invalid origin cert). `fc2club.net` serves a JS
  anti-bot challenge shell. `internal/scraper/javbus/javbus.go` gained an
  FC2 branch (`fc2.javbus.com/<code>`) ready for when the subdomain
  recovers, but its selectors are **unverified** and must be re-probed
  against a live response before trusting output. Net effect: FC2 files now
  extract and normalise their code correctly (confirmed in the testbed —
  `FC2-PPV-4947342` routes to `TBC/_unmatched` with `code` populated and
  reason `scrape failed: all sources failed for code FC2-PPV-4947342`,
  accurate triage instead of the old "no JAV code found in filename").
  Candidate follow-up once a source recovers: a dedicated `fc2ppvdb`
  adapter, or re-probing `fc2club.net/html/FC2-<id>.html` (the endpoint
  shape used by other JAV-tooling FC2 scrapers).

- **`internal/database/database.go`**: `Open` now sets `busy_timeout` and
  `journal_mode=WAL` via DSN `_pragma` params (applied on every pooled
  connection) instead of a one-shot `db.Exec` that only ever touched the
  first connection, and caps the pool at `SetMaxOpenConns(1)` — modernc's
  driver has no single-writer serialisation of its own, and concurrent
  writers (pipeline, HTTP review handlers, and critically the `slog`
  DB-log handler, which meant every `SQLITE_BUSY` error log was itself
  another contending write) were producing the 3,036 production errors.
  A lost `Record` write left no `Seen()` row, so the next poll re-routed
  the same file and `fsutil.UniquePath` minted the `_2`/`_4` duplicate
  copies observed piling up in `TBC/_filter`. Verified in the testbed: 3
  rapid re-drops of the same filename plus a mid-run restart produced zero
  `SQLITE_BUSY` entries and zero `_2`/`_3` files.

- **`internal/watcher/watcher.go`**: event buffer 256 → 1024, and fsnotify
  Create/Write events for one path now debounce (2s trailing-edge,
  collapsing an SMB copy's event flood into one emit) before entering the
  pipeline. Implemented as a plain map of due-times swept by a ticker,
  entirely inside `Run()`'s own select loop — deliberately *not* a
  per-path `time.AfterFunc` goroutine, which would race the channel close
  on shutdown (a timer firing after `ctx.Done()` closes `w.events` panics
  on send). `pipeline.go`'s "stat failed, skipping" WARN (1,440× in the
  log — routine when a duplicate event targets an already-moved path)
  downgraded to Info.

- `internal/pipeline/filter.go`: junk-pattern matching now also checks a
  whitespace-collapsed copy of the filename, and `"苍老师"` was added to
  `junkPatterns` — catches a specific ad/filler clip observed reused
  verbatim (often letter-spaced) across several unrelated torrents in the
  log, which the un-collapsed substring check missed.

**Verify:** `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`
all clean (34 tests, 17 packages, including new
`internal/database/database_test.go`, a rewritten
`internal/pipeline/code_test.go` with all 32 real log filenames as a
regression table, `internal/pipeline/filter_test.go`, and a debounce test in
`internal/watcher/watcher_test.go`). Ran the built binary against
`testbed/` with `javbus`+`javdb` enabled, recreating the real failing
filenames from the log (parent folders included, since several only
resolve via the new fallback): all organised correctly into
`sorted/<CODE> (<year>)/` — including the `FTKD-040` multi-part case (both
`-1`/`-2` files, one shared sidecar set) and the `DAL-011` fallback-then-
duplicate case above. The FC2 file extracted its code and landed in
`_unmatched` with the expected "scrape failed" reason (sources externally
dead, not a code defect). The genuinely-uncoded file and the spaced ad clip
routed to `_unmatched`/`_filter` respectively, unchanged. Stress/restart
test (3× rapid re-drop + mid-run restart) produced no `SQLITE_BUSY` and no
duplicate-suffixed files. A directory created *after* the watcher's initial
scan wasn't picked up by fsnotify until the next 60s poll — pre-existing,
documented behaviour (the poll is the safety net), not a regression.

Out of scope for this pass (explicitly deselected): a manual code-assign UI
action on the review page, and the `javlibrary` adapter.

## Dependency order

```
M0 ──▶ M1 ──▶ M2 ──▶ M3 ──▶ M4a ──▶ M4b ──▶ M5
skeleton  triage  1 scraper  GUI    2nd source  aggregators  release
                  +organise         +queue drain +proxy infra (in progress:
                                     (done)       (done)      tag+GHCR push pending)
```

Each milestone is a mergeable PR. M2 is the "does the core idea work" proof
point; everything after is breadth and resilience.
