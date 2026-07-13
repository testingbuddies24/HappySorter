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

## Milestone 3 — Setup GUI (folders, sources, rename)

**Goal:** everything configurable without editing YAML by hand.

- HTMX pages: `/setup/folders`, `/setup/sources` (enable/reorder/QPS + `proxy_url`), `/setup/rename`.
- `/review` list with retry/delete actions; `/rescan`, `/pause`, `/resume`.
- Config writes persisted to `config.yaml` + hot-reload where safe.

**Verify:** change folders + enable a source in the GUI, drop a file, confirm
it flows using the new config; retry an item from `/review`.

## Milestone 4 — Multi-source fallback + aggregators

**Goal:** resilience — one source dying doesn't stop the pipeline.

- Add `sodprime`, `ideapocket`, `mgstage` (studio/distributor adapters).
- Add aggregators `javbus` (age-cookie), `javdb`, `javlibrary` (proxy-gated).
- Fallback ordering, skip-with-reason logging, cookie persistence under `cookies_dir`.
- `deploy/cf-worker/worker.js` + docs wiring (referenced by DEPLOYMENT § 4a).

**Verify:** disable S1, enable JavBus → same code resolves via fallback;
with no proxy, a Cloudflare source logs `Cloudflare-gated` and is skipped
rather than crashing; with proxy set, it resolves.

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
M0 ──▶ M1 ──▶ M2 ──▶ M3 ──▶ M4 ──▶ M5
skeleton  triage  1 scraper  GUI    fallback  release
                  +organise         +aggregators
```

Each milestone is a mergeable PR. M2 is the "does the core idea work" proof
point; everything after is breadth and resilience.
