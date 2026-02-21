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
├── .env                 # Credentials (gitignored)
├── .env.example         # Template for .env
├── README.md
├── CLAUDE.md
├── go.mod
└── go.sum
```

CSVs are saved to `~/Downloads` (macOS/Windows user Downloads folder), not a project subdirectory.

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

    // Login opens browser, navigates to login page, prompts user for auth code at runtime
    // authCode supports any method: TOTP, SMS OTP, email OTP
    // Returns error if login fails
    Login(username, password, authCode string) error

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
    github.com/joho/godotenv v1.5.x     // .env file loading
    golang.org/x/term v0.x.x            // Hidden password input
)
```

## Environment Variables

`.env` file format:

```env
BROKER=zerodha
ZERODHA_USERNAME=your_username
ZERODHA_PASSWORD=your_password
```

### First Run Setup (no `.env` present)

If `.env` does not exist when the bot starts, enter interactive setup mode:

1. Scan the `brokers/` folder to discover all available broker implementations
2. Display a numbered menu, e.g.:
   ```
   Select your broker:
     1. Zerodha
     2. Groww

   If your broker is not listed, email support@bharosaclub.com with your broker name to request it be added. We will confirm once it is available.

   Enter number:
   ```
3. Read the user's selection and resolve it to the broker identifier
4. Prompt: `Username:` — read username from stdin
5. Prompt: `Password:` — read password with echo disabled (use `golang.org/x/term` or equivalent)
6. Write all values to `.env` automatically
7. Proceed with the normal run

### Subsequent Runs

If `.env` exists, load it silently via `godotenv` and proceed without any prompts.

### `--reset` Flag

When the user passes `--reset`:

1. Delete the existing `.env` file
2. Re-trigger the interactive first-run setup as described above
3. Proceed with a fresh run using the new credentials

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
5. Install dependencies: `go get github.com/go-rod/rod github.com/joho/godotenv golang.org/x/term`

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
   - Prompt user at runtime: `Enter auth code (TOTP/SMS/email OTP):` — read from stdin
   - Submit auth code for 2FA
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
   - If `--reset` flag is set, delete `.env` before loading
   - If `.env` does not exist, run interactive setup: show broker menu, prompt for username and password, then write `.env`
   - Load `.env` silently via `godotenv`
   - Initialize the selected broker (e.g. `brokers.NewZerodhaBroker()`)
   - Execute login flow (broker will prompt for auth code at runtime)
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

1. Add CLI flags: `--headless`, `--broker`, `--verbose`, `--reset`
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
- NEVER log passwords or auth codes
- Auth codes are always prompted at runtime and never stored
- Consider adding `.env` to `.gitignore` in Phase 1

## Zerodha-Specific Details

### URLs

- Login entry point: `https://console.zerodha.com/` — may show a "Login with Kite" button or redirect directly to Kite login
- Kite login (actual form): `https://kite.zerodha.com/connect/login?api_key=console&sess_id=...`
- Trade book: `https://console.zerodha.com/reports/tradebook`

### Account Number

The Zerodha client ID (e.g. `BT2632`) is the same as the username. No separate extraction needed — stored from `Login()` parameters.

### Verified CSS Selectors (from recorded session)

**Login flow (Kite login page)**
- Username field: `#userid`
- Password field: `#password`
- Submit button: `button[type='submit']`
- TOTP field (second step): `[label='External TOTP']` — note: custom `label` attribute, NOT `aria-label`
  - `type="number"`, `id="userid"` (reused), `maxlength="6"`
  - Zerodha **auto-submits** when 6 digits are entered — use `rod.Try()` around `MustInput` to handle context cancel mid-navigation

**Login button on console landing page (if present)**
- Selector: `button.btn-blue`
- Must set up `MustWaitNavigation()` BEFORE clicking — button navigates to Kite login page

**Trade book page**
- Date range input (opens picker): `div.three input`
- Clear date selection: `span.mx-clear-wrapper`
- Preset buttons (visible after opening picker):
  - Current FY: `button:nth-of-type(4)` (aria-label "current FY")
  - Previous FY: `button:nth-of-type(3)` (aria-label "prev. FY")
- Search/filter button: `div.one span`
- CSV download link: `div.table-section a:nth-of-type(2)` (aria-label "CSV")

**Date picker — manual calendar navigation (for FYs older than prev FY)**

The date picker is a vue2-datepicker range picker with left (From) and right (To) calendar panes.

Left pane selector: `div.mx-range-wrapper > div:nth-of-type(1)`
Right pane selector: `div.mx-range-wrapper > div:nth-of-type(2)`

For each pane:
1. Click year label link: `{pane} a:nth-of-type(6)` — opens year picker panel
2. Year panel: `div.mx-calendar-panel-year .mx-panel-year span` — shows ~10 years; click matching text
3. If target year not visible, click previous decade: `div.mx-calendar-panel-year a.mx-icon-last-year`
4. Month panel (appears after year click): `div.mx-calendar-panel-month .mx-panel-month > span:nth-of-type(N)` where N = month number (1=Jan, 4=Apr, 3=Mar)
5. Day cells: `{pane} table tbody td.cur-month` — iterate and click matching day number

### Known Behaviors (verified in production)

- Console landing page may or may not show "Login with Kite" button — code handles both with a 5s timeout
- TOTP auto-submits on 6th digit entry causing navigation mid-`MustInput` — wrap with `rod.Try()`
- `WaitNavigation()` must be set up BEFORE any click that triggers navigation
- CSV link (`div.table-section a:nth-of-type(2)`) is absent when no trades exist — treat as RecordCount=0
- **Rod saves downloaded files using the download GUID as filename**, NOT `SuggestedFilename`. Use `info.GUID` for the rename source path. `WaitDownload()` blocks until the file is fully written — no `.crdownload` polling needed.
- Date picker opened via JS: `() => document.querySelector('.mx-input-wrapper').click()` — SVG elements and their children do not work with `rod.MustClick()`
- Year label selector: `{pane} a.mx-current-year` (not `a:nth-of-type(6)` as originally documented)
- Day selector: `td[title="YYYY-MM-DD"]` — most reliable, use `date.Format("2006-01-02")`
- 5s delay between FY downloads required to avoid Zerodha rate limiting
- Zerodha supports data from 2013-04-01 onwards (`not-before` attribute on datepicker)

## Adding a New Broker

1. Create `brokers/<brokername>.go` (e.g., `brokers/groww.go`)
2. Use `package brokers` at the top of the file
3. Implement the `Broker` interface with a struct like `GrowwBroker`
4. Add a constructor: `NewGrowwBroker(headless bool, verbose bool) (*GrowwBroker, error)`
5. Register in `init()`: `RegisterBroker("groww", func(headless bool, verbose bool) (Broker, error) { return NewGrowwBroker(headless, verbose) })`
6. Use `z.verbose` / a `debugLog` helper to gate debug-level logs
7. Add broker-specific env vars to `.env.example`
8. Document broker-specific details in this file

## Commands Reference

```bash
# Run the bot (first run: prompts for credentials if no .env)
go run .

# Build binary
go build -o broker-trade-sync

# Run with visible browser (for debugging)
go run . --headless=false

# Show detailed step-by-step logging
go run . --verbose

# Override broker without editing .env
go run . --broker=zerodha

# Clear saved credentials and re-run setup
go run . --reset
```

## Build Status

All phases complete and verified in production (account BT2632):
- Phase 1 ✅ Project setup
- Phase 2 ✅ Core interface (brokers/broker.go)
- Phase 3 ✅ Zerodha implementation — full end-to-end verified
- Phase 4 ✅ Main CLI — flags, first-run setup, download loop, summary
- Phase 5 ✅ Download manager — idempotency, FY scanning, boundary detection
- Phase 6 ✅ Polish — --verbose, --broker, password masking, Ctrl+C

**Not yet tested (requires waiting between runs):**
- Subsequent run after N days: should re-download current FY only, skip all prior FYs. Logic is implemented and correct — `foundActiveFY=true` is set when skipping already-downloaded FYs, ensuring the historical boundary is correctly detected.

## Troubleshooting

- **Login fails**: Check credentials in `.env`; use `--reset` to re-enter
- **TOTP not accepted**: Enter the code in the terminal (not the browser); make sure your authenticator app is time-synced
- **Download hangs**: Run with `--headless=false` to see browser state
- **Rate limited by Zerodha**: Wait a few minutes and re-run — already-downloaded FYs are skipped
- **Want to change broker/credentials**: Run with `--reset` to clear `.env` and re-trigger setup
