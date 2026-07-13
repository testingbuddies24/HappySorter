# Testbed

A persistent local sandbox for running HappySorter yourself, outside of
Docker, without touching a real NAS. Everything under this folder except
this README is gitignored — drop whatever test files you like.

## Layout

- `watch/` — drop test video files here (must be real JAV codes, e.g.
  `SSIS-001.mp4`, and at least 50 MB to pass the rubbish filter — use
  `fsutil -s 60M testfile.mp4` on Windows or `truncate -s 60M testfile.mp4`
  in git-bash to make a fake file of realistic size).
- `library/` — organised output lands here, plus `library/review/_filter/`,
  `library/review/_unmatched/`, `library/review/_duplicate/`.
- `config/config.yaml` — already configured with real Windows paths into
  this folder and the `s1` source enabled. Edit priorities/sources here as
  needed; the running server does not require Docker.

## Running

From the repo root:

```powershell
go build -o bin/happy-sorter.exe ./cmd/server
$env:HAPPYSORTER_CONFIG = "D:/Projects/HappySorter/testbed/config/config.yaml"
$env:HAPPYSORTER_DB = "D:/Projects/HappySorter/testbed/config/happy-sorter.db"
./bin/happy-sorter.exe
```

Then drop files into `testbed/watch/` and watch `library/` fill in. Status
dashboard is at `http://localhost:8080`.

## Duplicate handling

Drop the same code twice (same filename or same code+extension resolving
to the same destination) and the second one will be routed to
`library/review/_duplicate/` untouched, instead of overwriting or
auto-renaming next to the first — the first release is never touched.
