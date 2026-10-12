# Cappuccino ☕️ 
`Cappuccino` is a CLI Tool for monitor the system activity (include the CPU Active, GPU Active etc.) 
It can shows live CPU (E/P Cluster and pre-core), GPU, ANE(Apple Neural Engine), RAM information with UI.
It reads data from macOS's built-in `powermetrics` and renders it with the [`termui`](https://github.com/gizak/termui)
 package 

![platform](https://img.shields.io/badge/platform-macOS%20Apple%20Silicon-black)
![go](https://img.shields.io/badge/go-1.27%2B-blue)
![license](https://img.shields.io/badge/license-MIT-green)

<video width="800" controls>
  <source src="assets/demo.mp4" type="video/mp4">
  
</video>

## Features

- **CPU** — E-core / P-core utilization and frequency, with an optional
  per-core breakdown.
- **GPU** — utilization and current frequency.
- **ANE** — estimated utilization and power draw.
- **Memory** — used/total RAM and swap.
- **Power** — live CPU / GPU power charts with rolling average, peak and a
  thermal-throttle indicator.
- **Interactive TUI** — a bottom hint bar, toggleable help overlay and an
  in-app REPL for changing settings on the fly.
- **Lightweight** — a single self-contained binary, no background services.


## Requirements

- macOS on Apple Silicon (M1 / M2 / M3 or newer).
- **Root privileges.** `powermetrics` only runs as the superuser, so Cappuccino
  must be started with `sudo`.
- Go 1.27 or newer **only if you build from source**.


## Installation

### From source

```sh
git clone https://github.com/starxsky/Cappuccino.git
cd Cappuccino
go build -o Cappuccino ./cmd/cappuccino
sudo ./Cappuccino
```

### With `go install`

```sh
go install github.com/starxsky/Cappuccino@latest
sudo "$(go env GOPATH)/bin/Cappuccino"
```

> Tip: add an alias to your shell profile so you only ever type `sudo Cappuccino`.
> Some hints in the app refer to `cappu` as a shorthand for that alias.


## Usage

```sh
sudo Cappuccino [flags]
```

```text
Flags:
  -a, --avg int         Interval for averaged values (seconds) (default 30)
  -c, --color int       Choose display color (0-8) (default 2)
  -h, --help            help for Cappuccino
  -i, --interval int    Display interval and sampling interval for powermetrics (seconds) (default 1)
      --max-count int   Max show count to restart powermetrics (default 0 = unlimited)
      --show-cores      Show per-core usage
```


Examples:

```sh
sudo Cappuccino                 # default 1s sampling
sudo Cappuccino -i 2 -a 60      # sample every 2s, average over 60s
sudo Cappuccino --show-cores    # include per-core gauges
sudo Cappuccino --color 5       # magenta theme
```

## Keyboard shortcuts

| Key            | Action                                   |
| -------------- | ---------------------------------------- |
| `q` / `Ctrl-C` | Quit                                     |
| `h` / `?`      | Toggle the help overlay                  |
| `:`            | Enter command mode (REPL)                |
| `+` / `-`      | Increase / decrease the sampling interval|
| `c`            | Cycle the bar color                      |
| `Esc`          | Close the help overlay / cancel a command|

## Command mode (REPL)

Press `:` to open the command prompt at the bottom of the screen, type a
command and press `Enter`. `Esc` cancels.

| Command                | Description                                        |
| ---------------------- | -------------------------------------------------- |
| `help`                 | Show the help overlay                              |
| `quit` / `exit`        | Quit                                               |
| `interval <seconds>`   | Set the sampling/display interval (restarts powermetrics) |
| `avg <seconds>`        | Set the averaging window                           |
| `color <0-8>`          | Set the bar color                                  |
| `cores <on\|off>`      | Toggle the per-core layout                         |
| `maxcount <n>`         | Restart powermetrics every `n` samples (`0` = off) |
| `clear`                | Clear the current status message                   |

## Supported SoCs

CPU/GPU max power and bandwidth tables are built in for:

`M1`, `M1 Pro`, `M1 Max`, `M1 Ultra`,
`M2`, `M2 Pro`, `M2 Max`, `M2 Ultra`,
`M3`, `M3 Pro`, `M3 Max`, `M3 Ultra`.

SoCs without a dedicated table fall back to conservative defaults, so the tool
still works — only the percentage scales may be less precise.

## How it implement ? 

Cappuccino spawns `powermetrics` in the bacground and catch its plist output to a temporary file : 

```text
powermetrics --samplers cpu_power,gpu_power,thermal -f plist -i <interval_ms> -o /tmp/cappuccino_powermetrics<timecode>
```
Each sample is a NUL-spearated plist document. Cappuccino parses the newset one and normalizes it into `internal/metrics` structs, and renders the result with tool `termui`. Mem statistics are read separately from `vm_stat` and `sysctl`. 

# Contributing 
Any Issues and pull requests are welcome, but please make sure you have executed below commands before you opening a PR : 

```sh 
gofmt -w . 
go vet ./... 
go build ./... 
```

Please keep changes focused and include a short description of what you changed
and why.

## Acknowledgements

- Inspired by [asitop](https://github.com/tlkh/asitop).
- Terminal UI powered by [termui](https://github.com/gizak/termui).
- CLI parsing by [cobra](https://github.com/spf13/cobra).
- Plist decoding by [howett.net/plist](https://howett.net/plist).