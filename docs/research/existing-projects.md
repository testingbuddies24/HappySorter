# Research: existing OSS projects

> Source: HappySorter research agent #2 (run 2026-07-13)
> Status: complete

## Why this document exists

Before building HappySorter from scratch, we want to know what already exists,
so we can borrow good ideas, avoid duplicating effort, and understand why
existing solutions didn't take over the niche.

## 1. Active / notable projects (initial survey)

| Project                         | Lang   | Framework              | Features                       | Docker?   |
|---------------------------------|--------|------------------------|--------------------------------|-----------|
| `javlibrary-scraper` (forks)    | Python | requests + BeautifulSoup | CLI scraper for JavLibrary   | No        |
| `JAVLib-scraper`                | Node   | puppeteer/playwright    | Web scraper                  | No        |
| `JAVManager`                    | Python | Flask                  | Web UI + SQLite               | Some      |
| `AV Organizer`                  | Go     | single binary           | CLI + web                     | Yes       |
| `jav-scraper`                   | Python | Scrapy                 | Multi-site scraper            | No        |
| **Jellyfin.Plugin.Jav***        | C#     | Jellyfin plugin        | Metadata agent in Jellyfin    | Via Jellyfin |

## 2. What's most active

- **Python `javlibrary-scraper` forks** — multiple active forks; sporadic maintenance; no clear leader.
- **Jellyfin JAV plugin** — actively maintained; provides a metadata agent that scrapes JavLibrary inside Jellyfin's native UI.
- **Go `AV Organizer`** — single-binary, low-RAM, has Docker images.

## 3. Why none of them have "won"

| Gap                                              | Who fills it           |
|--------------------------------------------------|------------------------|
| No lightweight all-in-one Docker image (<100 MB) | **HappySorter target** |
| No unified multi-site scraper with fallback      | **HappySorter target** |
| No CLI-first tool + setup GUI for NAS            | **HappySorter target** |
| Most scrapers don't store images locally         | **HappySorter target** |
| No actress deduplication across studios          | **HappySorter target** |

## 4. Lessons we steal

| From                          | Lesson                                                  |
|-------------------------------|---------------------------------------------------------|
| Jellyfin JAV plugin           | Tight integration with Jellyfin's metadata layout wins user trust |
| AV Organizer (Go)             | Single binary + Docker image = ideal NAS citizen         |
| JAVManager (Flask + SQLite)   | Keep dependencies boring; SQLite is enough               |
| All scrapers                  | Site HTML changes break scrapers — adapters must be isolated and easy to swap |

## 5. Lessons we avoid

| From                          | Mistake                                                  |
|-------------------------------|----------------------------------------------------------|
| javlibrary-scraper forks      | No multi-site fallback → single point of failure        |
| Most scrapers                 | Reference CDN image URLs → rot in months                |
| Most desktop tools            | Windows-only → excludes Linux NAS users                 |
| Most CLI tools                | No GUI → setup barrier for non-technical users          |

## 6. Decisions for HappySorter (provisional)

1. **Adapters-per-site** pattern — each scraper is its own package implementing `Lookup(code) (*Metadata, error)`.
2. **Multi-source with ordered fallback** — driven by user-configured priority list in the GUI.
3. **Local image storage** — every cover/fanart downloaded to the per-item folder, never hotlinked.
4. **Jellyfin-compatible NFO** — Kodi movie NFO format; what Jellyfin reads natively.
5. **One container** — web UI + scraper + watcher + DB all in one image.
6. **Multi-arch Docker** — `linux/amd64,linux/arm64` for Synology DS220+ etc.
7. **No scraping of the original Blogger site** — it's just a download page; nothing to aggregate from there.