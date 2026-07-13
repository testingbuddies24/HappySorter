# HappySorter — Product Specification

> Status: **Draft v1** (2026-07-13)
> Owner: project owner
> See also: [`ARCHITECTURE.md`](ARCHITECTURE.md), [`DEPLOYMENT.md`](DEPLOYMENT.md), [`research/`](research/)

## 1. Problem statement

Users who collect Japanese Adult Video (JAV) media need a way to:

1. Drop a folder of unsorted video files (often with cryptic filenames).
2. Have those files automatically organised into a library that any media
   player (Jellyfin, Kodi, Plex) can recognise.
3. Get rich metadata (cover, actress, studio, release date, plot, etc.)
   attached to each item without manual work.

The existing solutions either:
- Are Windows-only desktop tools (the legacy JavHelper .NET app).
- Depend on a single external API that can (and has) disappeared.
- Have no self-hosted, Docker-deployable equivalent.
- Don't store images locally, so cover art rots within months.
- Provide no web UI for non-technical users.

## 2. Target user

A technically-comfortable hobbyist running a home NAS (Synology or QNAP)
who:

- Maintains a personal JAV media collection on the NAS.
- Runs Jellyfin (or Kodi/Plex) on the same NAS to browse and play it.
- Wants the "drop folder and walk away" workflow that classic file-renamers
  promised, but in a modern, self-hosted, multi-source, Docker-native form.

Out of scope: shared/multi-user libraries, cloud sync, public sharing,
streaming to non-LAN clients. This is a single-household appliance.

## 3. Goals

| # | Goal                                                                                |
|---|-------------------------------------------------------------------------------------|
| G1 | Drop a video file in a watched folder → it ends up in a Jellyfin-recognised layout, no human intervention. |
| G2 | Multi-source metadata with ordered fallback — never fail because one site died.     |
| G3 | Single Docker container, runs on x86_64 and ARM NAS, idle RAM ≤ 100 MB.            |
| G4 | One-time setup via a clean web GUI; Jellyfin is the library UI, not HappySorter.    |
| G5 | All metadata + images stored locally; works fully offline once scraped.             |
| G6 | Rubbish / unmatched files routed to a review folder; never silently lost.          |

## 4. Non-goals

- Not a streaming server (Jellyfin does that).
- Not a public metadata service / aggregator.
- Not a torrent client or indexer.
- Not a metadata editor — once scraped, edits are made in Jellyfin's own UI.
- Not a multi-tenant SaaS — single household, single user.

## 5. Functional requirements

### F1. Folder watcher

The system shall:

- Watch a user-configured input folder (default `/watch`).
- Detect new video files (`.mp4`, `.mkv`, `.avi`, `.wmv`, `.mov`, `.flv`, `.rmvb`, `.ts`).
- Enqueue each new file for processing.
- Be idempotent: re-running on the same folder does not duplicate work.
- Use `fsnotify` where available; fall back to polling on filesystems that
  don't deliver inotify events (NFS, SMB shares).
- Be pausable via the web GUI.

### F2. Rubbish filter

Files matching any of the following shall be moved to `review/_filter/`:

- Extension not in the video-extension allow-list.
- File size < 50 MB (likely a sample, trailer, or accidentally-downloaded `.url`/`.txt`/`.jpg`).
- Filename matches sample/teaser patterns: `sample`, `trailer`, `preview`, `字幕` (when standalone), `.url`, `.txt`, `.html`, `.part`, `.torrent`.
- Empty files.

The user reviews `review/_filter/` periodically and deletes what's truly junk.

### F3. Code extraction

For each non-rubbish video file:

- Normalise the filename (strip release-group suffixes like `-CH`, `-UC`, `-JP`, `HD`, `FHD`).
- Match against regex `^([A-Z0-9]{2,5})-?(\d{2,5})$` (case-insensitive).
- If no match → move to `review/_unmatched/`.
- If matched → enqueue for metadata scrape with the extracted code.

### F4. Multi-source metadata scrape

- Each configured source implements `Lookup(code) (*Metadata, error)`.
- Sources are tried in user-configured priority order.
- **Default source order is studio-direct first, aggregators as fallback**
  (see `research/source-test-results.md`): `s1`, `sodprime`, `ideapocket`,
  `mgstage`, then `javbus`, `javdb`, `javlibrary`. Studio sites are not
  Cloudflare-gated and resolve reliably; aggregators cover the long tail.
- A "failure" that triggers fallback: HTTP error, code-not-found, missing required fields (title or cover image), context timeout, Cloudflare challenge on a source with no proxy configured.
- Per-source rate limit (QPS) configurable; default 1 QPS.
- **Cloudflare handling:** an optional `proxy_url` (HTTP/SOCKS5 or a
  Cloudflare Worker forwarder) routes requests for aggregator sources.
  When empty, Cloudflare-gated sources are skipped with a clear log reason;
  studio sources still work. Most residential/home-NAS IPs need no proxy.
- **Age-gate handling:** sources behind an age-consent wall (e.g. JavBus)
  have their consent cookie POSTed once and persisted under `cookies_dir`,
  so subsequent requests pass through automatically.
- Cached results (code → metadata) live in SQLite so re-scrape on retry isn't required.
- Sources ship disabled by default; user enables them in the GUI and is
  reminded of the legal/ToS context.

### F5. Jellyfin folder layout

For each successfully-scraped item:

```
<Library Root>/
└── <CODE> (<YEAR>)/
    ├── <CODE> (<YEAR>).<ext>      ← renamed video file
    ├── poster.jpg                   ← cover image (downloaded locally)
    ├── fanart.jpg                   ← backdrop image (downloaded locally)
    ├── backdrop.jpg                 ← alias of fanart (Jellyfin-friendly)
    ├── thumb.jpg                    ← small thumbnail
    ├── movie.nfo                    ← Kodi movie NFO (Jellyfin reads natively)
    └── actors/
        └── <actress>.jpg            ← per-actress photo (when available)
```

- Cover image always downloaded; never hotlinked.
- Multiple actresses → one sub-image per name.
- If scrape returns no cover image, generate a placeholder (`poster.jpg`
  with code rendered on it) and continue.

### F6. Review folder

Two sibling folders, both visible in the GUI's home screen:

- `review/_filter/` — files the rubbish filter rejected.
- `review/_unmatched/` — files where the code couldn't be parsed.

User can:
- List contents of each.
- Manually rename a file to inject a code, then click "Retry".
- Delete files (with confirm).
- Empty the whole folder (with confirm).

### F7. Web GUI (setup + logs only)

Pages:

| Page                  | Purpose                                                       |
|-----------------------|---------------------------------------------------------------|
| `/`                   | Status dashboard — counts, recent activity, pause/resume      |
| `/setup/folders`      | Configure source/output/review folder paths                  |
| `/setup/sources`      | Enable/disable scrape sources, set priority, set QPS limit   |
| `/setup/rename`       | Configure rename template (folder/file naming)                |
| `/logs`               | Tail-style log viewer; filter by level; export               |
| `/review`             | List of files in `_filter` and `_unmatched`, with retry/delete actions |
| `/rescan`             | Trigger a full re-scan of the source folder                   |

The GUI is **not** a library browser. The library is browsed in Jellyfin.

### F8. Logging

- Structured JSON logs to stdout (captured by Docker).
- Mirror to SQLite (`logs` table) for the web viewer.
- Default log level INFO; DEBUG toggleable via env.
- Never log full magnet URIs (only btih hash prefix).

### F9. Configuration

- All config in a single `config.yaml` mounted at `/config/config.yaml`.
- Hot-reload where safe (log level, source priority); restart required for
  port change.
- First run: if `/config/config.yaml` missing, generate defaults and start
  with the GUI on port 8080.

## 6. Non-functional requirements

| Concern          | Target                                                        |
|------------------|---------------------------------------------------------------|
| Image size       | Final Docker image ≤ 100 MB compressed                        |
| Idle RAM         | ≤ 100 MB (Go process + SQLite)                                |
| Cold start       | Container ready to serve GUI in < 3 s                         |
| Scrape latency   | First source response within 10 s on a typical NAS             |
| Throughput       | Process 1 new file every ~5 s during bulk import              |
| Backup           | `cp` of `/config/` (DB) + `/library/` (media) is sufficient   |
| Crash recovery   | Watcher state restored from DB on restart                    |
| Update           | Pull new image, restart container; config & media untouched  |
| Architectures    | `linux/amd64`, `linux/arm64` (Synology DS220+, QNAP TBS-453) |
| License          | MIT (or user's preference — open to input)                    |

## 7. Success criteria

The project is "done for v1" when:

1. `docker run` command in README deploys the app on a fresh NAS.
2. Web GUI at `http://nas:8080` lets a user configure folders in < 5 minutes.
3. Dropping `SSIS-001.mp4` into the watched folder results, within 30 s,
   in `Library/SSIS-001 (2018)/` containing the renamed file, poster.jpg,
   fanart.jpg, movie.nfo, and actors/.
4. Jellyfin, pointed at `Library/`, recognises the new movie and displays
   cover, title, year, actress, plot.
5. Disabling source A and enabling source B in the GUI causes the same
   code to be scraped successfully on retry.
6. A `.txt` file dropped into the watched folder ends up in
   `review/_filter/`, not the library.
7. A file named `random.mp4` (no JAV code) ends up in `review/_unmatched/`.
8. Container survives restart; watcher resumes without re-processing.

## 8. Legal / ethical posture

- HappySorter is a **personal-use** tool.
- It does not host, index, or redistribute scraped content.
- Users are responsible for ensuring they have the right to possess the
  files they organise.
- Scraping is done against public sites at low rates; sources ship disabled
  by default; user opts in.
- README must include a clear legal disclaimer.

## 9. Out of scope for v1

- Multi-user authentication.
- Per-item manual metadata editing inside HappySorter.
- Cloud sync / remote access.
- Mobile app.
- Real-time dashboard widgets / charts.
- Translation of Japanese titles to user language.
- Automatic deletion of source files after success (configurable on/off,
  default off).
- Built-in Jellyfin setup wizard (separate concern).