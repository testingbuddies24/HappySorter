# Research: javhelper.blogspot.com

> Source: https://javhelper.blogspot.com/ (visited 2026-07-13)
> Author of original research: HappySorter research agent #1
> Status: complete

## 1. Executive summary — what "javhelper" actually is

The site is **not** a JAV metadata database or library aggregator — it is a
**Blogger download portal for a Windows desktop utility** of the same name.
A single blog post (published July 2015) hosts MEGA.nz download links to a
closed-source .NET application.

The actual "JavHelper" software is a **Windows-only file-renamer/organizer**
for JAV (Japanese Adult Video) collections. It:

1. Recursively scans an input directory for video files (`.avi`, `.mpg`, `.rmvb`, …).
2. Extracts the JAV product **code** (e.g. `ABC-001`, `HEY-067`) from each filename.
3. Queries external JAV-database websites to fetch **cover image + actress metadata**.
4. Renames the file to `[Actress] [Code] [Title].ext`.
5. Moves successfully processed files to `success/`, unprocessed ones to `failure/`.
6. Handles multi-disc releases (CD1 / CD2).

| Property              | Value                                          |
|-----------------------|------------------------------------------------|
| Hosting platform      | Blogger (Blogspot)                             |
| Distribution          | MEGA.nz links                                  |
| Software platform     | Windows XP+                                    |
| Runtime               | .NET Framework 2.0 / 3.5                       |
| Implementation        | Closed-source (likely C# or VB.NET)            |
| Current version       | v1.1.5.8                                       |
| Post date             | 2015-07-11                                     |
| Comments              | 6 (issues: Amazon API expired, actress stats, subtitle vs multi-disc conflict) |

## 2. Why the Blogger site is limited

| Limitation               | Impact on users                                  |
|--------------------------|--------------------------------------------------|
| Single blog post         | No discoverable content; the URL is a download link, nothing more |
| No search/filter UI      | Users can only navigate via Blogger's archive |
| No accounts/favourites   | Nothing personalised; no collection sync across devices |
| No API                   | Power users / scripts can't reach the library |
| Blogger ad injection     | Random ads may appear around the download |
| Third-party hosting      | MEGA links can be removed; Blogger URL could disappear |
| No metadata database     | All knowledge lives in filenames on the user's disk |
| Windows-only executable  | Linux/Mac/NAS users cannot use the tool at all |

## 3. Information architecture today

```
javhelper.blogspot.com
└── /2015/07/javhelper-v1.html          ← single blog post
    ├── title: "JavHelper 批量批次重命名JAV檔名的軟體"
    ├── body: screenshot + usage notes
    ├── sidebar: archive, author, Atom feed
    └── comments: 6 user reports
```

There are no categories, tags, or per-item pages. Page types are limited to
what Blogger itself provides: post, archive, feed.

## 4. Data model the *software* (not the site) operates on

This is the implicit data model the Windows .NET tool consumes and produces:

| Field         | Source                         | Example                  |
|---------------|--------------------------------|--------------------------|
| Code          | parsed from filename           | `SSIS-001`               |
| Title         | scraped from JAV DB site       | Japanese/Chinese string  |
| Actress(es)   | scraped                        | `Riona Hime`             |
| Cover image   | scraped                        | CDN URL → saved locally  |
| Release date  | scraped                        | `2018-04-13`             |
| Duration      | scraped                        | `120 min`                |
| Director      | scraped (sometimes)           | `Takeshi Kogusuri`       |
| Studio/label  | inferred from code prefix      | `S1 / SOFT ON DEMAND`    |
| Genre/tags    | scraped                        | `big tits, blowjob`      |
| Subtitles     | heuristic on filename / scrape | `中字`                   |
| Disc #        | parsed from filename           | `CD1`, `CD2`             |

## 5. Scraping strategy inferred from the original tool

Based on user comments on the blog:

- **Amazon Product Advertising API** — was once used, now expired (per comment).
- **JavLibrary** (`javlibrary.com`) — likely target for code → title/actress lookup.
- **JavBus** (`javbus.com`) — likely target for cover art.
- Possibly **DMM.co.jp** — common Japanese source.

The original tool performs live HTTP scraping on each run; no offline cache.

## 6. UX patterns HappySorter should preserve

What users actually value about javhelper:

1. **Filename-as-query** — the JAV code is everything; users already know it.
2. **Single-pass batch processing** — drop a folder, get it organized.
3. **Local-first** — no upload required, metadata is personal.
4. **Cover art preview** — visual confirmation of the right item.
5. **Disc-aware** — multi-disc releases stay together.

## 7. UX patterns HappySorter should *add*

The Blogger site + Windows tool have no concept of these:

| Feature                          | Why                                          |
|----------------------------------|----------------------------------------------|
| Web UI accessible from phone     | Check your collection from anywhere on LAN   |
| Search / filter / sort           | "Show me everything starring X, released in 2024" |
| Per-user favourites & watched    | Personal library, multiple users on a NAS    |
| Tag & actress auto-complete      | Browse by any axis                           |
| Cover gallery view               | Visual browse by cover                       |
| Audit log / undo                 | Revert accidental renames                    |
| REST API                         | Script / integrate with other tools          |
| Background scanner               | Watch a folder, auto-organize new files      |
| Magnet / torrent reference field | Quick lookup of where to acquire             |
| Import / export JSON             | Migrate from old JavHelper, share definitions|

## 8. Lessons learned from the original blog

These are recurring themes in the 6 user comments:

- External API dependencies die (Amazon).
- Multi-disc handling is tricky (CD1 vs subtitles).
- Actress metadata is incomplete in some databases.
- No way to override a wrong auto-match.

→ HappySorter must support **manual override per item** and **multiple
metadata sources** with fallback.

## 9. Direct URLs to verify in future research

- Main post: `https://javhelper.blogspot.com/2015/07/javhelper-v1.html`
- Archive: `https://javhelper.blogspot.com/2015/`
- Atom feed: `https://javhelper.blogspot.com/feeds/posts/default`
- Likely scrape targets (not the blog itself):
  - `https://www.javlibrary.com/`
  - `https://www.javbus.com/`
  - `https://www.dmm.co.jp/`

## 10. TL;DR

javhelper.blogspot.com is a **download page**, not a product. The "product" is
a 2015-era Windows desktop .NET file-renamer that scrapes third-party JAV
databases and rearranges files on the user's disk.

HappySorter's reason for existing is to **rebuild that workflow as a
modern, self-hosted web app that any device on the home network can use** —
with proper persistence, search, multi-user, and a Docker deployment target
that makes it survive the death of any single blog/MEGA/API.
