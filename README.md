# zerodha-trade-sync

A Go CLI bot that automates downloading trade history CSVs from broker web consoles using browser automation.

## Features

- Automated login with TOTP-based 2FA (no manual OTP entry)
- Downloads trade CSVs organized by financial year
- Idempotent - re-running never duplicates downloads
- Adapter pattern - supports multiple brokers

## Supported Brokers

- [x] Zerodha

## Quick Start

### Prerequisites

- Go 1.21+
- Chrome browser installed

### Setup

1. Clone the repository:
   ```bash
   git clone <repo-url>
   cd zerodha-trade-sync
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Create `.env` file from template:
   ```bash
   cp .env.example .env
   ```

4. Edit `.env` with your credentials:
   ```env
   ZERODHA_USERNAME=your_username
   ZERODHA_PASSWORD=your_password
   ZERODHA_TOTP_SECRET=your_totp_secret_key
   ```

   > **Note:** The TOTP secret is the base32 key from Zerodha's 2FA setup (the manual entry key, not the QR code).

### Run

```bash
go run .
```

### Options

```bash
# Run with visible browser (for debugging)
go run . --headless=false

# Enable verbose logging
go run . --verbose
```

## How It Works

### First Run
1. Starts from current financial year
2. Goes backward FY by FY
3. Downloads CSV for each year with trades
4. Stops when it hits a year with no trading activity

### Subsequent Runs
1. Checks existing downloads in `downloads/` folder
2. Only downloads missing financial years
3. Re-downloads current FY (might have new trades)

## CSV Naming Convention

```
<account_number>_<from_date>_<to_date>.csv
```

Example: `ZX1234_20230401_20240331.csv`

## Project Structure

```
zerodha-trade-sync/
├── broker.go            # Broker interface
├── zerodha.go           # Zerodha implementation
├── main.go              # CLI entry point
├── downloads/           # CSV storage
├── .env                 # Credentials (gitignored)
├── .env.example         # Credential template
└── CLAUDE.md            # AI assistant guide
```

## Security

- Credentials are stored in `.env` (gitignored)
- TOTP secret is never logged
- No data leaves your machine

## Adding a New Broker

1. Create `<brokername>.go` in root (e.g., `groww.go`)
2. Implement the `Broker` interface
3. Add env vars to `.env.example`
4. Update main.go with broker selection logic

## License

MIT
