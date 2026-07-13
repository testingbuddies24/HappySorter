# HappySorter documentation

> Quick navigation for the docs tree.

## Top-level

- [README.md](../README.md) — project elevator pitch, quickstart, status
- [SPEC.md](SPEC.md) — what we're building; goals, requirements, success criteria
- [ARCHITECTURE.md](ARCHITECTURE.md) — how it's built; components, data model, scraping flow
- [ROADMAP.md](ROADMAP.md) — how we'll build it; milestone-by-milestone vertical slices
- [DEPLOYMENT.md](DEPLOYMENT.md) — running it; Docker / docker-compose / NAS-specific

## Research

The research docs capture *why* the project is shaped the way it is. Read these before changing a design decision.

- [research/website-analysis.md](research/website-analysis.md) — what the reference site javhelper.blogspot.com actually is
- [research/jav-metadata-standards.md](research/jav-metadata-standards.md) — JAV code format, studios, metadata fields
- [research/existing-projects.md](research/existing-projects.md) — OSS landscape; what already exists, what gaps we fill
- [research/stack-recommendations.md](research/stack-recommendations.md) — why Go + SQLite + HTMX
- [research/source-test-results.md](research/source-test-results.md) — live probe results; Cloudflare findings, source priority, mitigations

## Reading order for new contributors

1. `README.md` — orientation
2. `SPEC.md` — what's being built
3. `ARCHITECTURE.md` — how it's shaped
4. `research/` — context and rationale
5. `DEPLOYMENT.md` — when you're ready to run it

## Decisions log

Decisions are inlined in the relevant doc (e.g. "Why SQLite over Postgres" lives in `stack-recommendations.md`). When a decision changes, update the doc and link from here.

| Decision                                  | Where it lives                              |
|-------------------------------------------|---------------------------------------------|
| Stack: Go + SQLite + HTMX                 | `research/stack-recommendations.md`         |
| Jellyfin folder layout                    | `SPEC.md` § F5 + `ARCHITECTURE.md` § 5      |
| GUI is setup-only, not a library browser  | `user-requirements` memory + `SPEC.md` § F7 |
| Multi-source fallback                     | `SPEC.md` § F4 + `ARCHITECTURE.md` § 4      |
| SQLite over Postgres                      | `research/stack-recommendations.md` § 2     |