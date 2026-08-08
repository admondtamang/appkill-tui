# appkill-tui ⚡

A colorful, keyboard-driven terminal task manager for Linux — like `htop`, but with
process grouping, listening-port visibility, and Docker/Kubernetes awareness built in.

![license](https://img.shields.io/badge/license-MIT-blue.svg)
![Go version](https://img.shields.io/badge/go-1.21%2B-00ADD8.svg)
![PRs welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)

```
╭── appkill ─────────────────────────────────────────────────────────────────╮
│ 💻 CPU [||||············]   9.7%   │ Sort: CPU▼   ⏱ 10s                   │
│ 🧠 RAM [||||||||||······]  57.4%   │ View: [All]                          │
│ 💿 DSK [||||||||||||||||]  90.4%   │ Filter: 🔌 Ports │ 🧩 Apps           │
╰────────────────────────────────────────────────────────────────────────────╯
  / Search...

  PID       NAME                             CPU%    RAM MB    DISK KB  PORTS
──────────────────────────────────────────────────────────────────────────────
  ---   📦💻  ▾ chromium (51)                 55.0   12163.9   11634776  -
  725280 💤💻  ├─ chromium                     5.0     100.0    1352240  -
  45680 💤☸️  grafana                          0.9     162.4          0  -
  ---   📦🐳  ▾ containerd (2)                 0.7     300.1          0  -
  3201  💤💻  pipewire-pulse                   0.5      33.6       2304  -
──────────────────────────────────────────────────────────────────────────────
Nav:  ↑/↓ Move │ / Search │ Enter Expand/Collapse │ Tab Filter   Showing: 1-44 of 423
Act:  x Kill │ +/- Refresh Rate │ q Quit
Sort: s Sort │ S Reverse
```

## Features

- **Live process table** — PID, name, CPU%, RAM, and disk I/O, refreshed on a
  configurable interval (default 10s, adjustable 1–60s with `+`/`-`).
- **Process grouping** — same-named processes (e.g. dozens of `chromium` tabs)
  collapse into one row with aggregated stats; expand into a tree view on demand.
- **Search** — filter the list by name or PID as you type.
- **Sort** — by CPU, RAM, or disk usage, either direction.
- **Kill** — send `SIGTERM` to a single process or an entire process group at once.
- **Listening ports** — each process shows the local ports it's listening on
  (read via `/proc`/`gopsutil`).
- **Runtime detection** — 🐳 Docker, ☸️ Kubernetes, or 💻 host, inferred from
  `/proc/<pid>/cgroup`.
- **Filter tabs** — `All` / `🔌 Ports` / `🧩 Apps`, cycled with `Tab`.
- **System dashboard** — color-coded CPU/RAM/Disk usage bars up top.

## Requirements

- Linux (reads `/proc` directly — not tested on macOS or Windows).
- Go 1.21+ if building from source.

## Install

The app ships as two identical commands, `appkill` and `appkill-tui` — use
whichever you like.

```sh
go install github.com/admondtamang/appkill-tui/cmd/appkill@latest
```

(The GitHub repo is currently named `appkill-tui` — a typo, not yet
renamed — that's what makes the import path above look odd.)

Or build both from source:

```sh
git clone git@github.com:admondtamang/appkill-tui.git
cd appkill-tui
go build -o appkill ./cmd/appkill
go build -o appkill-tui ./cmd/appkill-tui
```

### After installation

`go install` places the binaries in `$(go env GOPATH)/bin` (usually
`~/go/bin`). Make sure that directory is on your `PATH`:

```sh
export PATH="$PATH:$(go env GOPATH)/bin"
```

Then launch the app with either:

```sh
appkill
# or
appkill-tui
```

Some processes (owned by other users) hide their I/O and cgroup info, and
can't be killed, unless you run it with `sudo`.

## Keybindings

| Key         | Action                                    |
|-------------|--------------------------------------------|
| `↑`/`↓`, `j`/`k` | Move selection                        |
| `/`         | Search by name or PID                     |
| `Enter`     | Expand/collapse a process group           |
| `Tab`       | Cycle filter: All → Ports → Apps          |
| `s`         | Cycle sort field: CPU → RAM → Disk        |
| `S`         | Reverse sort direction                    |
| `x`         | Kill selected process (or whole group)    |
| `+` / `-`   | Increase / decrease refresh interval      |
| `q`, `Ctrl+C` | Quit                                    |

## Project layout

| Path                        | Responsibility                                  |
|-----------------------------|--------------------------------------------------|
| `cmd/appkill/main.go`       | Thin entry point for the `appkill` command       |
| `cmd/appkill-tui/main.go`   | Thin entry point for the `appkill-tui` command   |
| `internal/app/app.go`       | Bubble Tea model, keybindings, and rendering     |
| `internal/app/process.go`   | Per-process stats via `gopsutil`                 |
| `internal/app/group.go`     | Grouping, sorting, filtering, and row flattening |
| `internal/app/network.go`   | Listening-port lookup                            |
| `internal/app/runtime.go`   | Docker/Kubernetes/host detection via cgroups     |
| `internal/app/system.go`    | System-wide CPU/RAM/disk usage                   |

Both commands are thin wrappers around `internal/app.Run()` — same
application, two names.

## Contributing

This project is open source and open to contributions of any size — bug
fixes, new features, or just cleanup.

1. Fork the repo and create a branch for your change.
2. Keep changes focused; match the existing code style (`gofmt`, no unrequested
   abstractions).
3. Run `go build ./...` and `go vet ./...` before opening a PR.
4. For anything beyond a small fix, open an issue first to discuss the approach.

## License

[MIT](LICENSE)
