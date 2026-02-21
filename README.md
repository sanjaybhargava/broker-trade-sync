# broker-trade-sync

A Go CLI bot that automates downloading trade history CSVs from broker web consoles using browser automation (Chrome via Rod).

## Features

- Interactive first-run setup — no manual config file editing
- Login with runtime TOTP/OTP prompt (auth code never stored)
- Downloads trade CSVs organized by financial year (Indian FY: Apr 1 - Mar 31)
- Idempotent — re-running never duplicates downloads, skips already-downloaded FYs
- Adapter pattern — supports multiple brokers from one codebase

## Supported Brokers

- [x] Zerodha

## Quick Start

### Prerequisites

- Go 1.21+
- Chrome browser installed

### Install & Run

```bash
git clone <repo-url>
cd broker-trade-sync
go mod download
go run .
```

On first run, the bot will interactively ask for your broker, username, and password, then save them to `.env`. On every run, it prompts for your TOTP/OTP at login time — auth codes are never stored.

### Commands

```bash
# Normal run
go run .

# Run with visible browser (useful for debugging)
go run . --headless=false

# Clear saved credentials and re-run setup
go run . --reset

# Enable verbose logging
go run . --verbose
```

## How It Works

### First Run
1. Prompts for broker, username, password — saves to `.env`
2. Prompts for TOTP/OTP at login time (not stored)
3. Starts from current financial year, goes backward FY by FY
4. Downloads CSV for each FY with trades
5. Stops when it finds a FY with no trading activity

### Subsequent Runs
1. Loads credentials silently from `.env`
2. Prompts for TOTP/OTP at login
3. Scans `downloads/` to find already-downloaded FYs
4. Downloads only missing FYs
5. Always re-downloads current FY (ongoing year may have new trades)

## CSV Naming Convention

```
<account_number>_<from_date>_<to_date>.csv
```

Example: `ZX1234_20230401_20240331.csv`

Dates are in `YYYYMMDD` format. This enables idempotent checks — filenames are parsed to detect existing downloads.

## Project Structure

```
broker-trade-sync/
├── brokers/
│   ├── broker.go        # Broker interface, FY helpers, registry
│   └── zerodha.go       # Zerodha implementation
├── main.go              # CLI entry point, download loop, summary
├── downloads/           # Downloaded CSVs (gitignored)
├── .env                 # Saved credentials (gitignored)
├── .env.example         # Credential template
├── CLAUDE.md            # Developer/AI assistant guide
└── README.md
```

## Security

- Credentials stored in `.env` (gitignored, never committed)
- TOTP/OTP prompted at runtime and never stored anywhere
- All data stays on your machine — no external services involved

## Adding a New Broker

1. Create `brokers/<brokername>.go` using `package brokers`
2. Implement the `Broker` interface (see `brokers/broker.go`)
3. Register it in an `init()` function: `RegisterBroker("name", constructor)`
4. Add broker-specific env vars to `.env.example`

See `brokers/zerodha.go` as a reference implementation.

## Troubleshooting

- **Login fails** — check credentials in `.env`; run `go run . --reset` to re-enter
- **Wrong TOTP** — make sure your authenticator app is time-synced
- **Download hangs** — run with `--headless=false` to watch the browser
- **No records found** — your account may not have trades in recent FYs; the bot will ask before checking further back
