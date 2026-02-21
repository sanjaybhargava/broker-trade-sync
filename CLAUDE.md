# CLAUDE.md - broker-trade-sync

## Project Overview

**broker-trade-sync** is a Go CLI bot that automates downloading trade history CSVs from broker web consoles. It uses Rod (browser automation) to log in, navigate to trade books, and download CSVs organized by financial year. Starting with Zerodha, the architecture supports adding multiple brokers.

## Architecture

### Adapter Pattern

The project uses the **adapter pattern** to support multiple brokers:

- `brokers/broker.go` defines a common `Broker` interface
- Each broker implementation lives in `brokers/<brokername>.go` (e.g., `brokers/zerodha.go`)
- All broker code uses `package brokers`
- The main bot imports the `brokers` package and only interacts with the `Broker` interface
- Adding a new broker = implementing the interface in a new file under `brokers/`

### Folder Structure

```
broker-trade-sync/
├── brokers/
│   ├── broker.go        # Broker interface definition
│   └── zerodha.go       # Zerodha broker implementation
├── main.go              # CLI entry point
├── downloads/           # CSV storage (gitignored)
├── .env                 # Credentials (gitignored)
├── .env.example         # Template for .env
├── README.md
├── CLAUDE.md
├── go.mod
└── go.sum
```

## Broker Interface

Define this interface in `brokers/broker.go`:

```go
package brokers

import "time"

// FinancialYear represents a FY with start and end dates
type FinancialYear struct {
    Label     string    // e.g., "FY2023-24"
    StartDate time.Time // April 1
    EndDate   time.Time // March 31
}

// DownloadResult represents the outcome of a CSV download
type DownloadResult struct {
    Filename    string
    RecordCount int
    FY          FinancialYear
}

// Broker defines the interface all broker implementations must satisfy
type Broker interface {
    // Name returns the broker identifier (e.g., "zerodha")
    Name() string

    // Login opens browser, navigates to login page, authenticates with 2FA
    // Returns error if login fails
    Login(username, password, totpSecret string) error

    // NavigateToTradeBook navigates to the trade history/book section
    NavigateToTradeBook() error

    // DownloadTradesForFY downloads the CSV for a specific financial year
    // Returns the result with filename and record count
    // Returns RecordCount=0 if no trades exist for that FY
    DownloadTradesForFY(fy FinancialYear, downloadDir string, accountNumber string) (*DownloadResult, error)

    // GetAccountNumber extracts the account/client ID from the logged-in session
    GetAccountNumber() (string, error)

    // Close cleans up browser resources
    Close() error
}
```

## Key Logic

### Financial Year Handling

- Indian financial year: April 1 to March 31
- Current FY calculation: If month >= April, FY starts this year; else FY started last year
- FY label format: `FY2023-24` for April 2023 to March 2024

### First Run Download Logic

1. Start from current financial year
2. Attempt download for each FY going backwards
3. If FY has zero records:
   - If no active FY found yet: prompt user "No records found. Check 3 more financial years?"
   - If active FY was already found: STOP (this is the boundary)
4. Continue until zero-record FY marks the historical boundary

### Subsequent Run Logic

1. Scan `downloads/` folder for existing files
2. Parse filenames to determine which FYs are already downloaded
3. Only download missing FYs (between earliest downloaded and current)
4. Re-download current FY if incomplete (ongoing year)

### CSV Naming Convention

```
<accountnumber>_<fromdate>_<todate>.csv
```

Example: `ZX1234_20230401_20240331.csv`

- Dates in `YYYYMMDD` format
- Account number from broker session
- Enables idempotent checks by filename parsing

## Dependencies

```go
// go.mod essentials
require (
    github.com/go-rod/rod v0.116.x      // Browser automation
    github.com/pquerna/otp v1.4.x       // TOTP generation for 2FA
    github.com/joho/godotenv v1.5.x     // .env file loading
)
```

## Environment Variables

`.env` file format:

```env
ZERODHA_USERNAME=your_username
ZERODHA_PASSWORD=your_password
ZERODHA_TOTP_SECRET=your_totp_secret_key
```

The TOTP secret is the base32 secret key from Zerodha's 2FA setup (not the QR code, but the manual entry key).

## Coding Conventions

### Go Standards

- Use `gofmt` for formatting
- Follow effective Go idioms
- Error handling: always check and wrap errors with context
- Use `log` package for output (not fmt.Println for errors)

### Naming

- Package names: lowercase, single word
- Interface names: noun (Broker, not IBroker)
- Implementation structs: `ZerodhaBroker`, `GrowwBroker`, etc.
- Files: lowercase with underscores if needed

### Error Handling

```go
if err != nil {
    return fmt.Errorf("failed to login to %s: %w", b.Name(), err)
}
```

### Browser Automation (Rod)

- Always use explicit waits, not sleeps where possible
- Handle element visibility before interaction
- Clean up browser resources in defer/Close()
- Use headless mode by default, with flag for visible debugging

## Step-by-Step Build Instructions

### Phase 1: Project Setup

1. Initialize Go module: `go mod init broker-trade-sync`
2. Create folder structure as specified
3. Create `.env.example` with placeholder values
4. Add `.gitignore` for `.env`, `downloads/`, and Go build artifacts
5. Install dependencies: `go get github.com/go-rod/rod github.com/pquerna/otp github.com/joho/godotenv`

### Phase 2: Core Interface

1. Create `brokers/broker.go` with the `Broker` interface and supporting types
2. Implement `FinancialYear` helper functions:
   - `CurrentFY() FinancialYear`
   - `PreviousFY(fy FinancialYear) FinancialYear`
   - `ParseFYFromFilename(filename string) (*FinancialYear, string, error)`

### Phase 3: Zerodha Implementation

1. Create `brokers/zerodha.go` implementing the `Broker` interface
2. Implement `Login()`:
   - Navigate to `https://console.zerodha.com/`
   - Enter username, password
   - Generate TOTP using `pquerna/otp` library
   - Submit TOTP for 2FA
   - Wait for dashboard load
3. Implement `NavigateToTradeBook()`:
   - Navigate to trade book/history section
4. Implement `DownloadTradesForFY()`:
   - Set date range filters
   - Trigger CSV download
   - Wait for download completion
   - Rename file to convention
   - Count records in CSV
5. Implement `GetAccountNumber()`:
   - Extract from logged-in session/page
6. Implement `Close()`:
   - Clean browser shutdown

### Phase 4: Main CLI

1. Create `main.go` with:
   - Import the `brokers` package
   - Load `.env` credentials
   - Initialize Zerodha broker via `brokers.NewZerodhaBroker()`
   - Execute login flow
   - Determine download strategy (first run vs subsequent)
   - Execute downloads
   - Print summary report

### Phase 5: Download Manager

1. Implement `downloads/` directory scanning
2. Implement idempotency checks
3. Implement the "check 3 more FYs" prompt logic
4. After each run, print a summary table showing:
   - Each CSV filename downloaded in this session
   - The record count for each file
   - Total files and total records

### Phase 6: Polish

1. Add CLI flags: `--headless`, `--broker`, `--verbose`
2. Add proper logging levels
3. Add graceful interrupt handling (Ctrl+C)
4. Test edge cases: no trades, partial years, network errors

## Testing Guidance

- Use Rod's testing utilities for browser tests
- Mock the Broker interface for unit testing main logic
- Test FY calculation edge cases (March vs April dates)
- Test filename parsing/generation roundtrip

## Security Notes

- NEVER commit `.env` or credentials
- NEVER log passwords or TOTP secrets
- Store TOTP secret securely (it's equivalent to a password)
- Consider adding `.env` to `.gitignore` in Phase 1

## Zerodha-Specific Details

### URLs

- Login: `https://console.zerodha.com/`
- Trade book is accessible after login from console dashboard

### Page Elements (may need updating)

- Login form has username, password fields
- TOTP is entered on second step after password
- Trade book has date range selectors and CSV export button

### Known Behaviors

- Session may timeout; handle re-login if needed
- CSV download may take a few seconds for large date ranges
- Rate limiting possible; add delays between requests if needed

## Adding a New Broker

1. Create `brokers/<brokername>.go` (e.g., `brokers/groww.go`)
2. Use `package brokers` at the top of the file
3. Implement the `Broker` interface with a struct like `GrowwBroker`
4. Add a constructor function like `NewGrowwBroker(headless bool) (*GrowwBroker, error)`
5. Add broker-specific env vars to `.env.example`
6. Register broker in main.go's broker selection logic
7. Document broker-specific details in this file

## Commands Reference

```bash
# Run the bot
go run .

# Build binary
go build -o broker-trade-sync

# Run with visible browser (for debugging)
go run . --headless=false

# Specify broker (future)
go run . --broker=zerodha
```

## Troubleshooting

- **Login fails**: Check credentials in `.env`, verify TOTP secret is correct
- **Download hangs**: Run with `--headless=false` to see browser state
- **No records found**: Verify date range and account has trading history
- **File already exists**: This is expected; bot is idempotent
