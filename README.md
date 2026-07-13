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
  -v /path/to/watch:/watch:ro \
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

🚧 **Pre-implementation.** Documentation is in place; code is not yet written. See `docs/SPEC.md` for the v1 scope.

## Repository layout

```
HappySorter/
├── README.md                       ← this file
├── docs/
│   ├── SPEC.md
│   ├── ARCHITECTURE.md
│   ├── DEPLOYMENT.md
│   ├── index.md
│   └── research/
│       ├── website-analysis.md
│       ├── jav-metadata-standards.md
│       ├── existing-projects.md
│       └── stack-recommendations.md
├── (future) cmd/                   # main entrypoint
├── (future) internal/              # pipeline / scraper / db / http
├── (future) web/                   # templates + static
├── (future) migrations/
├── (future) Dockerfile
└── (future) docker-compose.yml
```

## Legal

HappySorter is a **personal-use tool**. It does not host, index, or redistribute scraped content. You are responsible for ensuring you have the right to possess the files you organise.

Scrape sources ship **disabled** by default — you opt in via the GUI. Scraping is done at low rates (default 1 QPS) and you agree to abide by each source's terms of service.

## License

TBD (likely MIT).