# HappySorter — Deployment guide

> Status: **Draft v1** (2026-07-13)
> See also: [`SPEC.md`](SPEC.md), [`ARCHITECTURE.md`](ARCHITECTURE.md)

## 1. Prerequisites

- A NAS (Synology DSM 7.x, QNAP QTS, or any x86_64 / arm64 Linux host).
- Docker installed (Container Manager on DSM; Container Station on QTS).
- ~200 MB free disk for the Docker image.
- A folder you want HappySorter to watch (e.g. `/volume1/data/watch`).
- A folder where the organised library should live (e.g. `/volume1/data/jav`).
- Jellyfin (or any Kodi-compatible media server) optionally pointed at the library folder.

## 2. Quickstart — `docker run`

```bash
docker run -d \
  --name happy-sorter \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /volume1/data/happy-sorter/config:/config \
  -v /volume1/data/jav:/library \
  -v /volume1/data/watch:/watch \
  ghcr.io/testingbuddies24/happy-sorter:latest
```

What this does:
- `-p 8080:8080` — exposes the setup GUI at `http://<nas-ip>:8080`.
- `/config` — holds `config.yaml` and `happy-sorter.db` (DB).
- `/library` — the output Jellyfin-compatible library (writable).
- `/watch` — the input drop folder. **Must be writable**: HappySorter moves
  rubbish, unmatched, and duplicate files out of it into `review/_filter/`,
  `review/_unmatched/`, and `review/_duplicate/` under `/library` (F2/F6),
  and moves matched files into the organised library (F5). A read-only
  mount would break this.

Open `http://<nas-ip>:8080` and follow the setup wizard.

## 3. docker-compose.yml

```yaml
services:
  happy-sorter:
    image: ghcr.io/testingbuddies24/happy-sorter:latest
    container_name: happy-sorter
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./config:/config                       # DB + config.yaml
      - /volume1/data/jav:/library             # organised library output
      - /volume1/data/watch:/watch              # drop folder (writable — see § 2)
    environment:
      - TZ=Asia/Hong_Kong                      # match your NAS timezone
    # Hardening (§ 10) — on by default; the app only ever writes under the
    # three volumes above, so the container root filesystem can be read-only.
    read_only: true
    security_opt:
      - no-new-privileges:true
    user: "1000:1000"
```

Run:
```bash
docker compose up -d
docker compose logs -f happy-sorter
```

## 4. First-run setup (web GUI)

1. Open `http://<nas-ip>:8080` → Status dashboard.
2. Go to **Setup → Folders** → confirm `/watch`, `/library`, `/library/review/_filter`, `/library/review/_unmatched`, `/library/review/_duplicate`. Save.
3. Go to **Setup → Sources** → enable at least one scrape source, set its priority (1 = tried first). Save.
   - Start with the **studio-direct sources** (`s1`, `ideapocket`) — they work from any IP with no proxy.
   - The **aggregators** `javbus` and `javdb` also work from any IP with no proxy — despite the name, `§ 4a` below is not needed for either of them. `javlibrary` is listed in config but has no adapter yet (still blocked on a genuine Cloudflare challenge; see `docs/ROADMAP.md` M4b).
4. (Optional) Go to **Setup → Rename** → tweak folder/file templates. Save.
5. Drop a test file (e.g. `SSIS-001.mp4`) into the `/watch` folder.
6. Watch the dashboard — within ~30 s the file should be in `/library/SSIS-001 (2018)/` with cover, fanart, nfo.

If nothing happens, check **Logs** in the GUI.

## 4a. Optional: Cloudflare Worker proxy (no shipped source needs this yet)

No currently-shipped source needs a proxy: `javbus` and `javdb` both resolve
directly. This section is here for `javlibrary`, which is genuinely
Cloudflare-gated but has no adapter yet (`docs/ROADMAP.md` M4b) — set this up
only once that adapter ships and your IP gets a Cloudflare challenge from it.
This mirrors the approach used by the widely-used Emby/Jellyfin JavScraper
plugin. The Cloudflare Worker free tier (100k requests/day) is far more than a
personal library needs.

1. Sign in at <https://workers.cloudflare.com> and create a Worker.
2. Paste the pass-through forwarder from `deploy/cf-worker/worker.js` (fetches
   `?url=<target>` with Cloudflare's own egress IP and returns the response
   untouched).
3. Deploy → copy the `https://<name>.<subdomain>.workers.dev` URL.
4. In HappySorter: **Setup → Sources → Proxy URL**, paste that URL, Save.
   (Equivalent config key: `scraping.proxy_url` in `config.yaml`.)

A standard HTTP or SOCKS5 proxy URL works in the same field if you already
run one. Leave the field empty to go direct.

> **Note on age gates.** JavBus shows an age-verification redirect, but it
> turned out to be cosmetic — the redirect response body already has the real
> page, so HappySorter reads it directly with no consent POST or cookie
> needed.

## 5. Folder map (recommended)

```
/volume1/data/
├── watch/                       ← drop new videos here
│   ├── (incoming SSIS-001.mp4)
│   └── (incoming HEY-067.mp4)
├── jav/                         ← HappySorter output (= Jellyfin library)
│   ├── SSIS-001 (2018)/
│   │   ├── SSIS-001 (2018).mp4
│   │   ├── poster.jpg
│   │   ├── fanart.jpg
│   │   ├── movie.nfo
│   │   └── actors/
│   ├── HEY-067 (2022)/
│   │   └── ...
│   └── review/
│       ├── _filter/             ← files the rubbish filter rejected
│       └── _unmatched/          ← files where no JAV code was found
└── happy-sorter/
    └── config/
        ├── config.yaml
        └── happy-sorter.db
```

Jellyfin library config:
- Content type: Movies
- Folders: `/volume1/data/jav` (NOT including `review/`)
- Preferred metadata language: Japanese (or your preference)
- Enable real-time monitoring if you want fresh additions to appear instantly

## 6. NAS-specific notes

### Synology DSM 7.x

- Open **Container Manager** → **Project** → **Create** → paste the docker-compose.yml.
- For `/watch` to be visible, the host path must be under `/volume1/...` (DSM's main volume) or another defined volume.
- File watcher (`fsnotify`) works on Btrfs and ext4 inside `/volume1/`.
- If you drop files via SMB into `/watch`, inotify events may be delayed by 1–5 s; this is normal.

### QNAP QTS / QuTS hero

- Use **Container Station** → create the container from the image.
- Volume paths must be under `/share/...` or a defined volume.
- For arm64 QNAP NAS (e.g. TS-253D), confirm the image tag supports arm64:
  `docker manifest inspect ghcr.io/testingbuddies24/happy-sorter:latest | grep arm64`.

### NFS / SMB shares as `/watch`

- `fsnotify` does not reliably receive inotify events on remote-mounted filesystems.
- HappySorter falls back to a 60-second polling scan of `/watch` when no events arrive.
- This is automatic; no configuration needed.

### Low-RAM NAS (≤ 1 GB total)

- HappySorter's idle RAM is ~30–80 MB.
- Set Docker memory limit to 256 MB if you want a hard ceiling.
- Disable Jellyfin's realtime monitoring if RAM is tight.

## 7. Backup

Minimum to back up:
- `./config/` (contains the SQLite DB; restoring this restores HappySorter's memory of what's already processed).

The library folder (`/library`) is just files — back it up however you back up the rest of your NAS media.

Restore procedure:
1. `docker compose down`
2. Restore `./config/` from backup
3. `docker compose up -d`
4. HappySorter resumes from where the DB says it was.

## 8. Updates

```bash
docker compose pull happy-sorter
docker compose up -d
```

The DB schema migrates automatically. Old library folders are untouched.

To pin a version:
```yaml
image: ghcr.io/testingbuddies24/happy-sorter:1.0.0
```

## 9. Troubleshooting

| Symptom                                | Cause                                       | Fix |
|----------------------------------------|---------------------------------------------|-----|
| GUI doesn't load                       | Port 8080 blocked; wrong NAS IP             | Check `docker ps`, NAS firewall |
| Files dropped, nothing happens         | Watcher paused; source not enabled          | Resume in dashboard; enable source in Setup → Sources |
| All files end up in `review/_unmatched/` | Source site is down; code regex too strict | Check Logs; try a different source |
| File ends up in `review/_duplicate/`   | A release for that code already exists in the library | Compare the two files by hand, then delete one; the existing library release was left untouched |
| Logs show `Cloudflare-gated`            | Only relevant once `javlibrary` ships (not yet implemented) | Enable a studio source, or set a proxy (§ 4a) |
| Cover image is a placeholder            | Source returned no cover                   | Try another source; manually drop cover into the folder |
| `permission denied` on `/library`      | UID mismatch between container and NAS      | Set `user: "1000:1000"` in compose and `chown -R 1000:1000 /volume1/data/jav` |
| Container restarts repeatedly          | Crash in pipeline                           | `docker logs happy-sorter` — share the tail |

## 10. Hardening checklist

The image and `docker-compose.yml` in this repo already do the first three:

- [x] Container runs as non-root (UID 1000) — baked into the `Dockerfile`.
- [x] `read_only: true` on the container root FS — the app only ever writes
      under `/config`, `/library`, and `/watch` (all bind-mounted), so this
      is reasoned safe by code inspection. Not yet run under an actual
      read-only container on real hardware — if you hit a startup error
      after enabling this, that's the first thing to report.
- [x] `no-new-privileges:true` security option.

Up to you:

- [ ] GUI bound to LAN IP only (set via reverse proxy, e.g. Caddy).
- [ ] Watch folder permissions scoped to HappySorter's UID only (it needs
      write access to move files out, so `:ro` is not an option — see § 2).
- [ ] Jellyfin and HappySorter on the same trusted VLAN.
- [ ] Backup `./config/` daily.

## 11. Uninstall

```bash
docker compose down --rmi all
rm -rf ./config
# (keep /library and /watch — those are your files)
```