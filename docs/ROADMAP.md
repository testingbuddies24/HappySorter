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

## Milestone 1 — Watcher → filter → review (no scraping)

**Goal:** dropped files get triaged into review folders correctly.

- `fsnotify` watcher on `/watch` with polling fallback.
- Rubbish filter (extension allow-list, size floor, sample/junk patterns) → `review/_filter/`.
- Code extractor (normalise + regex) → on miss, `review/_unmatched/`.
- `files` table records every seen file + its state; idempotent on restart.

**Verify:** drop `SSIS-001.mp4` (stays queued, no scraper yet), `notes.txt`
(→ `_filter`), `random.mp4` (→ `_unmatched`). Restart container → no
re-processing. All transitions visible in `/logs`.

## Milestone 2 — First scraper (S1) → organise → NFO

**Goal:** full pipeline end-to-end for a studio-direct code, no proxy.

- Scrape manager + `Adapter` interface + HTTP client factory (§ 4.1).
- `s1` adapter (studio-direct; no Cloudflare — proven in probing).
- Organiser: create `<CODE> (<YEAR>)/`, download poster/fanart, move video.
- NFO writer (Kodi movie schema).
- `metadata_cache` populated; multi-disc codes reuse cache.

**Verify:** drop a real S1 code → within 30 s it lands in
`/library/<CODE> (<YEAR>)/` with `movie.nfo` + `poster.jpg` + `fanart.jpg`;
point Jellyfin at `/library` and confirm it displays title/year/cover/actress.

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
