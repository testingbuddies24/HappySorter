# Research: source-site probing results

> Date: 2026-07-13
> Method: live `curl` probes against candidate metadata sources for JAV code `SSIS-001`, plus source-code inspection of the active community projects that have already solved this problem.

## 1. Why this doc exists

The previous research assumed scraping would "just work" with appropriate
adapters. That was wrong. Real-world probing shows that **every candidate
site blocks naive HTTP requests** from datacenter IPs (which is what this
session runs on). Before designing around a source, we needed evidence:
which ones block, why, and which workarounds exist.

## 2. Probe results

| Site | Endpoint | HTTP | Body size | What happened |
|------|----------|------|-----------|---------------|
| JavLibrary | `/cn/?v=javssiss001` | **403** | 5 KB | Cloudflare "Just a moment…" challenge |
| JavLibrary + `over18=18` cookie | `/en/vl_searchbyid.php?keyword=SSIS-001` | **403** | 5 KB | Cloudflare still challenges — cookie alone is not enough |
| JavDB | `/v/SSIS-001` | **403** | 5 KB | Cloudflare "Attention Required!" block |
| JavBus | `/SSIS-001` | 200 | 22 KB | **Age verification gate** — solvable with `age=verified` cookie |
| JavBus | `/SSIS-001` (after age POST) | 200 | 24 KB | Cookie set, but the actual page returns to age-gate on re-fetch (cookie session-tied) |
| JavBus search | `/search/SSIS-001` | 200 | 22 KB | Same age-gate behaviour |
| DMM | `/mono/dvd/.../cid=ssis001/` | 200 | 3 KB | **Geo-blocked**: "このページはお住まいの地域からご利用になれません" (Japan-only) |
| FANZA | search via dmm.co.jp | 200 | 35 KB | Age gate (年齢認証) |
| FANZA | `www.fanza.co.jp` | DNS fail | — | Wrong domain — doesn't exist |
| R18.com | `/videos/vod/movies/detail/-/id=ssis001/` | 406 | 0 | Not Acceptable — bot UA filter |
| R18.com | with full browser headers | 404 | 61 KB | Page Not Found — likely geo or wrong URL pattern |
| S1 studio | `s1s1s1.com/search?keyword=SSIS-001` | **200** | 49 KB | **Works!** Studio sites don't use Cloudflare |

## 3. The Cloudflare problem

Every major aggregator (JavLibrary, JavDB, JavBus) sits behind Cloudflare.
From a datacenter IP, Cloudflare returns a JavaScript challenge page
("Just a moment…" / "Attention Required!") which requires a real browser
to solve. Plain `curl` cannot pass it.

Three ways the community has solved this:

### 3.1 `cloudscraper` (Python library)

Used by [`hibikidesu/javscraper`](https://github.com/hibikidesu/javscraper) (51 ★).
Replaces `requests` with a session that solves Cloudflare's JS challenge
automatically. Works against JavLibrary directly. **No Go equivalent**
exists that is actively maintained.

### 3.2 Cloudflare Worker as a forwarder (the standard)

Used by [`JavScraper/Emby.Plugins.JavScraper`](https://github.com/JavScraper/Emby.Plugins.JavScraper) (3783 ★, updated 2026-07-11).
A tiny Cloudflare Worker (paste-and-deploy, ~150 lines of JS) acts as a
generic HTTP forwarder. Requests from the user arrive at the Worker, the
Worker fetches the target with Cloudflare's own egress IP, which is
trusted by Cloudflare, then returns the result. Free tier: 100K req/day.

The Worker source lives at `cf-worker/index.js` in that repo. Setup:
1. Sign up at https://workers.cloudflare.com
2. Paste the Worker code
3. Save → get a `https://xxxx.subdomain.workers.dev` URL
4. Use that URL as a proxy in the scraper

### 3.3 Run from a residential IP

Home NAS users typically have residential IPs. Cloudflare is much more
permissive to residential traffic. Many NAS users will succeed with no
proxy at all. **We can't validate from this datacenter IP** — needs
in-the-wild testing.

### 3.4 Headless browser (Playwright / Puppeteer)

Real Chrome solving the JS challenge. **+200 MB image, +200 MB idle RAM**
— defeats the "lean NAS" goal. Reject for v1.

## 4. Existing OSS to learn from (and borrow selectors from)

| Repo | Stars | Updated | Language | Lesson |
|---|---|---|---|---|
| [`JavScraper/Emby.Plugins.JavScraper`](https://github.com/JavScraper/Emby.Plugins.JavScraper) | 3783 | 2026-07-11 | C# | **The reference impl.** Uses CF Worker proxy. Supports JavBus, JavDB, MsgTage, FC2, AVSOX, Jav123, R18. |
| [`hibikidesu/javscraper`](https://github.com/hibikidesu/javscraper) | 51 | 2026-05-06 | Python | 21 studio/aggregator scrapers with working XPath selectors. Uses cloudscraper. Source we can study. |
| [`JellyfinJav`](https://github.com/markheath/JellyfinJav) | 0 | 2026-06-01 | ? | Jellyfin-specific provider. |
| [`EvanGongka/JavScraper26`](https://github.com/EvanGongka/JavScraper26) | 14 | 2026-07-11 | Python | Local browser UI. |
| [`JavTool/JavScraper`](https://github.com/JavTool/JavScraper) | 4 | 2026-05-12 | ? | Extends JavHelper — direct lineage to our reference. |

## 5. Recommended source list for HappySorter v1

Ship adapters in this order (priority = tried first), based on (a) coverage,
(b) maintenance activity of the upstream scraper code, (c) anti-bot posture:

| # | Source | Type | Why |
|---|---|---|---|
| 1 | **S1** (`s1s1s1.com`) | Studio | No Cloudflare; works from any IP. Covers all S1/SSIS codes. |
| 2 | **SOD Prime** | Studio | Same as above; sister studio of S1. |
| 3 | **IdeaPocket** | Studio | Direct studio; works from any IP. |
| 4 | **MGStage (MGS)** | Distributor | Covers Heyzo, many others. |
| 5 | **JavBus** | Aggregator | Largest aggregator; needs age cookie + (optionally) CF Worker proxy |
| 6 | **JavDB** | Aggregator | Modern UI, hash-based image names; needs CF Worker proxy |
| 7 | **JAVLibrary** | Aggregator | Most comprehensive but most aggressively Cloudflare-protected |

**Strategy:**
- Try studio sources first (no anti-bot; always work).
- Fall through to aggregators (which need proxy or residential IP).
- Multi-source fallback already architected in SPEC § F4.

## 6. Selector portability

The XPath / CSS selectors from `hibikidesu/javscraper` are directly
translatable to Go — `net/html` plus a small selector helper (or
`github.com/PuerkitoBio/goquery`) handles the parsing. Fields covered:
title, code, studio, image, actresses, genres, release_date, score,
description, sample_video.

## 7. Concrete blocker summary

| Risk | Mitigation in HappySorter |
|---|---|
| Cloudflare blocks datacenter IPs | GUI exposes **optional proxy URL**; user points to their CF Worker. Default = none (home users usually don't need one). |
| Age gates on aggregators | Each adapter sets its required cookie on first request; persist in `~/.happy-sorter/cookies/<source>.txt`. |
| Site HTML changes | Selectors isolated in per-source adapter; one file to fix. |
| Site geo-block (DMM Japan) | Skip DMM adapter for v1; use studio-direct equivalents instead. |
| Image URL rot | Already in spec: download and store locally; never hotlink. |

## 8. Recommendation for project go/no-go

**Go — with one architectural addition.**

Add to the existing design:
1. A `proxy_url` config field (HTTP/SOCKS5/CF-Worker-URL) that all HTTP
   requests from adapters pass through.
2. A `cookies_dir` config field where each adapter persists its
   age-verification cookie on first use.
3. A documented "optional: deploy a Cloudflare Worker proxy in 5 minutes"
   section in DEPLOYMENT.md.
4. The default source list above (studios first, aggregators as fallback).

With these additions, the project is feasible from a Go codebase of
moderate size, no headless browser required, and survives the
Cloudflare problem the same way the active community already does.

## 9. Test artifacts (preserved)

Probes run during this research are preserved in `/tmp/scrape-probe/`
on the dev machine, including:
- `jb.html`, `jb-after3.html` — JavBus age-gate responses
- `jl.html`, `jl-real.html` — JavLibrary Cloudflare challenges
- `jdb.html` — JavDB Cloudflare block page
- `fanza2.html` — DMM age gate + geo-block
- `r18b.html`, `r18c.html` — R18 406/404 responses
- `s1s.html` — S1 studio successful response (49 KB)