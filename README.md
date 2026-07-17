# HappySorter

> Self-hosted, Docker-deployable organizer for personal JAV (Japanese Adult Video) media libraries, with first-class [Jellyfin](https://jellyfin.org/) compatibility.

Drop a video file into a watched folder. HappySorter parses the JAV code from the filename, scrapes metadata from multiple sources (with ordered fallback), and lays out the file into a Jellyfin-recognized folder: `<CODE> (<YEAR>)/<CODE> (<YEAR>).mp4` + `poster.jpg` + `fanart.jpg` + `movie.nfo` + `actors/`.

Runs as a single Docker container on a NAS (Synology, QNAP, anything x86_64 / arm64). The setup GUI is a one-time configuration tool — your actual library is browsed in Jellyfin.

## Why

The legacy tool at https://javhelper.blogspot.com/ is a 2015 Windows .NET file-renamer whose backend API has since died. HappySorter is the modern, self-hosted, multi-source equivalent — portable, Docker-native, and not at the mercy of any single website.

## Features

- 📁 **Folder watcher** — drop files in `/watch`, they appear organised in `/library`.
- 🗑️ **Rubbish filter** — junk files (`.url`, `.txt`, samples, trailers) routed to a review folder.
- 🔎 **Multi-source scrape with fallback** — configure JavLibrary, JavBus, JavDB, etc.; if one dies, the next takes over.
- 🎬 **Jellyfin-compatible output** — `movie.nfo` + cover + fanart + per-actress photos, layout Jellyfin reads natively.
- 🐳 **One container** — ~30–80 MB final image, multi-arch (`linux/amd64`, `linux/arm64`), idle RAM ≤ 100 MB.
- 🖥️ **Web GUI for setup** — configure folders, sources, rename template, view logs.
- 🔄 **Crash-safe** — pipeline state in SQLite; container restart resumes from where it left off.

## Quickstart

```bash
docker run -d \
  --name happy-sorter \
  --restart unless-stopped \
  -p 8080:8080 \
  -v $(pwd)/happy-sorter/config:/config \
  -v /path/to/library:/library \
  -v /path/to/watch:/watch \
  ghcr.io/<owner>/happy-sorter:latest
```

Then open `http://localhost:8080` and walk through the setup wizard.

See [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) for the full guide (docker-compose, NAS-specific notes, hardening, backup).

## Documentation

| Document | Purpose |
|---|---|
| [`docs/SPEC.md`](docs/SPEC.md) | Product spec — goals, requirements, success criteria |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Components, data model, scraping flow, config schema |
| [`docs/ROADMAP.md`](docs/ROADMAP.md) | Build plan — milestone-by-milestone vertical slices |
| [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) | Docker / docker-compose / NAS-specific deployment |
| [`docs/research/website-analysis.md`](docs/research/website-analysis.md) | What the original javhelper.blogspot.com actually is |
| [`docs/research/jav-metadata-standards.md`](docs/research/jav-metadata-standards.md) | JAV code format, studios, fields |
| [`docs/research/existing-projects.md`](docs/research/existing-projects.md) | OSS landscape — what already exists and why none of it "won" |
| [`docs/research/stack-recommendations.md`](docs/research/stack-recommendations.md) | Why Go + SQLite + HTMX |
| [`docs/research/source-test-results.md`](docs/research/source-test-results.md) | Live source probing — Cloudflare findings, source priority, mitigations |

## Project status

🏗️ **Milestone 4a complete** — the pipeline now has two working
studio-direct sources (S1, IdeaPocket) with real fallback: files dropped
into `/watch` are triaged (rubbish filter, JAV code extraction), scraped
live with metadata caching (so multi-disc releases skip re-scraping) and
priority-ordered fallback across sources, and organised into a
Jellyfin-recognised `<CODE> (<YEAR>)/` folder with `movie.nfo`,
`poster.jpg`, `fanart.jpg`, and `backdrop.jpg`. A file that would collide
with an already-organised release is left alone and routed to
`review/_duplicate/` instead of being overwritten or auto-renamed. Files
queued for scraping while no source was enabled now drain automatically
the moment a source is turned on — no restart, no manual retry. Everything
is configurable from the web GUI without editing YAML by hand:
`/setup/folders`, `/setup/sources`, `/setup/rename`, a `/review` queue
with retry/delete, `/logs`, and `/rescan`/`/pause`/`/resume` controls —
folder paths, sources, and rename templates all hot-reload without a
restart (only the watch path and server port need one). See
`docs/ROADMAP.md` for what's next (Milestone 4b: aggregator sources +
proxy infrastructure).

For a hands-on sandbox to run the server yourself and drop test files in,
see [`testbed/README.md`](testbed/README.md).

## Repository layout

```
HappySorter/
├── README.md                       ← this file
├── go.mod / go.sum
├── Dockerfile
├── docker-compose.yml
├── cmd/
│   └── server/                     # main entrypoint
├── internal/
│   ├── config/                     # config.yaml load/save + defaults
│   ├── database/                   # SQLite open + embedded migrations
│   ├── fsutil/                     # cross-device-safe file move helpers
│   ├── httpserver/                 # dashboard + /healthz
│   ├── logging/                    # slog JSON (stdout + logs table)
│   ├── nfo/                        # Kodi movie.nfo writer
│   ├── organiser/                  # Jellyfin folder layout + image download
│   ├── pipeline/                   # watcher -> filter -> scrape -> organise
│   ├── scraper/                    # Adapter interface, manager, s1 adapter
│   ├── store/                      # files + metadata_cache tables
│   └── watcher/                    # /watch folder watcher
├── web/                            # (future) HTMX templates + static
├── docs/
│   ├── SPEC.md
│   ├── ARCHITECTURE.md
│   ├── ROADMAP.md
│   ├── DEPLOYMENT.md
│   ├── index.md
│   └── research/
│       ├── website-analysis.md
│       ├── jav-metadata-standards.md
│       ├── existing-projects.md
│       ├── stack-recommendations.md
│       └── source-test-results.md
```

## Legal

HappySorter is a **personal-use tool**. It does not host, index, or redistribute scraped content. You are responsible for ensuring you have the right to possess the files you organise.

Scrape sources ship **disabled** by default — you opt in via the GUI. Scraping is done at low rates (default 1 QPS) and you agree to abide by each source's terms of service.

## License

[MIT](LICENSE) — free to use, modify, and self-host. Provided as-is, with no
warranty; use at your own risk (see [Legal](#legal) above).