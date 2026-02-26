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
├── build.sh             # Cross-platform build script
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

type Segment string

const (
    SegmentEQ Segment = "EQ" // Equity
    SegmentFO Segment = "FO" // Futures and Options
)

type FinancialYear struct {
    Label     string    // e.g., "FY2023-24"
    StartDate time.Time // April 1
    EndDate   time.Time // March 31 of next year
}

type DownloadResult struct {
    Filename    string
    RecordCount int
    FY          FinancialYear
    Segment     Segment
}

type Broker interface {
    Name() string
    Login(username, password, authCode string) error
    NavigateToTradeBook() error

    // DownloadTradesForFY downloads CSVs for one FY across the given segments.
    // Single navigation: sets date range once, then iterates segments via dropdown.
    // Returns one DownloadResult per segment. RecordCount=0 means no trades.
    DownloadTradesForFY(fy FinancialYear, downloadDir string, accountNumber string, segments []Segment) ([]*DownloadResult, error)

    GetAccountNumber() (string, error)
    Close() error
}
```

## Key Logic

### Financial Year Handling

- Indian financial year: April 1 to March 31
- Current FY calculation: If month >= April, FY starts this year; else FY started last year
- FY label format: `FY2023-24` for April 2023 to March 2024

### Multi-Segment Download Logic

A **single backward-scanning FY loop** downloads both EQ and FO per financial year:

1. For each FY (starting from current, going backward):
   a. Determine which segments still need downloading (skip already-downloaded, skip segments that hit boundary)
   b. Navigate to tradebook (fresh page), open date picker, set date range
   c. **Commit search**: click search on default EQ segment to lock dates into Vue state
   d. Download EQ CSV from the commit search results
   e. Switch segment dropdown to FO, click search again, download FO CSV
2. Each segment tracks its own boundary independently — FO can have trades in FYs where EQ doesn't, and vice versa

**Commit search** (step c) is critical: without it, switching the segment dropdown triggers a Vue auto-search using default/stale dates instead of the custom dates set in step b. This caused a production bug where FO downloads received wrong-FY data.

### Boundary Detection

- All segments (EQ, FO) are checked for every FY — no per-segment boundary tracking
- If ANY segment has data in a FY (downloaded or freshly found), the consecutive-empty counter resets
- Stop when ALL segments have zero records for **2 consecutive FYs** (historical boundary)
- No user prompts — fully automatic
- Already-downloaded FYs count as "has data" for boundary detection

### Subsequent Run Logic

1. Scan `~/Downloads` for existing CSVs matching each segment
2. Parse filenames to determine which FYs are already downloaded
3. Skip those — only download missing FYs
4. Always re-download current FY (ongoing year may have new trades)

### CSV Naming Convention

```
<accountnumber>_<segment>_<fromdate>_<todate>.csv
```

Examples:
- Equity: `ZX1234_EQ_20230401_20240331.csv`
- F&O: `ZX1234_FO_20230401_20240331.csv`

- Segment: `EQ` (Equity) or `FO` (Futures and Options)
- Dates in `YYYYMMDD` format
- Account number from broker session
- Enables idempotent checks by filename parsing
- Backward compatible: old-format files without segment (e.g., `ZX1234_20230401_20240331.csv`) are treated as EQ

## Dependencies

```go
// go.mod essentials
require (
    github.com/go-rod/rod v0.116.x      // Browser automation
    github.com/joho/godotenv v1.5.x     // .env file loading
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

If `.env` does not exist when the bot starts, the flow is:

1. Show broker menu and read selection (terminal only — browser not open yet):
   ```
   Select your broker:
     1. Zerodha
     2. Groww

   If your broker is not listed, email support@bharosaclub.com with your broker name to request it be added. We will confirm once it is available.

   Enter number:
   ```
2. **Browser opens immediately after broker is selected**
3. Prompt: `Username:` — read from stdin (browser already visible in background)
4. Prompt: `Password:` — read with plain `bufio.ReadString` (visible input)
5. Write all values to `.env` automatically
6. `Login()` runs: Rod navigates to login page and types username + password
7. Prompt: `Enter auth code (TOTP / SMS OTP / mobile app code):` — read from stdin
8. Rod types the code and submits — proceeds with downloads

This ensures the browser is always open before any credentials are typed, giving a consistent experience across all machines.

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
5. Install dependencies: `go get github.com/go-rod/rod github.com/joho/godotenv`

### Phase 2: Core Interface

1. Create `brokers/broker.go` with the `Broker` interface and supporting types
2. Implement `FinancialYear` helper functions:
   - `CurrentFY() FinancialYear`
   - `PreviousFY(fy FinancialYear) FinancialYear`
   - `ParseFYFromFilename(filename string) (*FinancialYear, string, Segment, error)`

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
- 2FA field (second step): `input[type='number'][maxlength='6']` — matches all Zerodha 2FA methods
  - `id="userid"` (reused from username field), `maxlength="6"`
  - The `label` attribute varies by method: `External TOTP` for authenticator apps, different value for mobile app code/SMS OTP — do NOT rely on `label`
  - **TOTP**: auto-submits on 6th digit — use `rod.Try()` around `MustInput` to handle context cancel mid-navigation
  - **Mobile app code / SMS OTP**: does NOT auto-submit — click `button[type='submit']` after input; wrap in `rod.Try()` so it's harmless if TOTP already navigated away

**Login button on console landing page (if present)**
- Selector: `button.btn-blue`
- Must set up `MustWaitNavigation()` BEFORE clicking — button navigates to Kite login page

**Trade book page**
- Segment dropdown: `select` — native HTML `<select>` element, only one on the page
  - Values: `EQ` (Equity, default), `FO` (Futures and Options), `CDS`, `COM`, `MF`, `EQX`, `MFX`
  - Switched via JS: `sel.value = 'FO'; sel.dispatchEvent(new Event('change', {bubbles: true}))` — Rod's `MustSelect` matches by text not value
  - **Must reset to EQ after fresh navigation** — SPA remembers last-selected segment across `MustNavigate` calls
- Date range input (opens picker): `div.three input`
- Clear date selection: `span.mx-clear-wrapper`
- Preset buttons (visible after opening picker):
  - Current FY: `button:nth-of-type(4)` (aria-label "current FY")
  - Previous FY: `button:nth-of-type(3)` (aria-label "prev. FY")
- Search/filter button: `div.one span`
- CSV download link: `div.table-section a:nth-of-type(2)` (aria-label "CSV")

**Date picker — manual calendar navigation**

The date picker is a vue2-datepicker range picker with left (From) and right (To) calendar panes.

Left pane selector: `div.mx-range-wrapper > div:nth-of-type(1)`
Right pane selector: `div.mx-range-wrapper > div:nth-of-type(2)`

For each pane:
1. Click year label: `{pane} a.mx-current-year` — opens year picker panel
2. Year panel: `{pane} .mx-panel-year span` — shows ~10 years; click matching text
3. If target year not visible:
   - Forward: `{pane} .mx-calendar-header a.mx-icon-next-year`
   - Backward: `{pane} .mx-calendar-header a.mx-icon-last-year`
4. Month panel: `{pane} .mx-panel-month span:nth-of-type(N)` where N = month number (1=Jan, 4=Apr)
5. Day cell: `td[title="YYYY-MM-DD"]` — unambiguous, use `date.Format("2006-01-02")`

### Known Behaviors (verified in production)

- Console landing page may or may not show "Login with Kite" button — code handles both with a 5s timeout
- TOTP auto-submits on 6th digit entry causing navigation mid-`MustInput` — wrap with `rod.Try()`
- Mobile app code does NOT auto-submit — always click `button[type='submit']` after typing; wrap in `rod.Try()` (no-op if TOTP already navigated)
- `WaitNavigation()` must be set up BEFORE any click that triggers navigation
- CSV link (`div.table-section a:nth-of-type(2)`) is absent when no trades exist — treat as RecordCount=0
- **Rod saves downloaded files using the download GUID as filename**, NOT `SuggestedFilename`. Use `info.GUID` for the rename source path. `WaitDownload()` blocks until the file is fully written — no `.crdownload` polling needed.
- Date picker opened via JS: `() => document.querySelector('.mx-input-wrapper').click()` — SVG elements and their children do not work with `rod.MustClick()`
- Year label selector: `{pane} a.mx-current-year` (not `a:nth-of-type(6)` as originally documented)
- Day selector: `td[title="YYYY-MM-DD"]` — most reliable, use `date.Format("2006-01-02")`
- **Fresh tradebook navigation per FY**: `DownloadTradesForFY` navigates to the tradebook URL before each download — Zerodha's SPA caches previous search results, so reusing the same page causes stale CSV links to be downloaded
- **Search result detection (Race + WaitRequestIdle)**: After clicking search, `WaitRequestIdle(3s)` ensures the API request completes, then `Race()` detects either the CSV link (`div.table-section a:nth-of-type(2)`) or "Report's empty" text (`ElementR("div", "[Rr]eport's empty")`). This dual positive-signal approach replaces the old CSV-timeout method, which returned false negatives when the search didn't trigger. Set up the idle listener BEFORE clicking search (Rod requirement). Note: Rod's `ElementR` regex runs in the browser (JavaScript) — use JS-compatible syntax, not Go's `(?i)`.
- **Segment dropdown persists across navigations**: Zerodha's SPA remembers the last-selected segment even after `MustNavigate`. If FO was selected for the previous FY, the next fresh navigation still shows FO. Code explicitly resets to EQ after every fresh navigation, before opening the date picker.
- 3s delay between FY downloads to avoid Zerodha rate limiting (reduced from 5s since fresh navigation adds its own delay)
- Zerodha supports data from 2013-04-01 onwards (`not-before` attribute on datepicker)
- **Post-login success detection**: Primary: wait for `a[href*="tradebook"]` (30s timeout) — only appears in the authenticated sidebar. Fallback: if that element isn't present (account type variation), check `page.Info().URL` contains `console.zerodha.com` — being on console confirms login succeeded. Raw URL polling during the redirect chain is unreliable; only check URL after 2FA is submitted.
- **Segment dropdown**: Native `<select>` element on tradebook page. Switched via JS `dispatchEvent('change')` because Rod's `MustSelect` matches by option text (regex), not by value — `"FO"` wouldn't match `"Futures & Options"`. EQ must be explicitly set after fresh navigation (SPA remembers last selection). Segment is switched AFTER the commit search (dates must be committed to Vue state first).
- **Commit search pattern**: After setting dates in the date picker, always click the search button once (with default EQ segment) before switching segments. This commits the date range to Vue's internal state. Without this, the dropdown `change` event triggers a Vue auto-search with default/stale dates. Discovered via production bug: FO FY2023-24 download received FY2024-25 data because dates weren't committed.
- **Single-loop multi-segment download**: One backward FY loop downloads both EQ and FO per FY in a single navigation. Each segment tracks its own historical boundary independently. F&O activity can exist without equity trades (e.g., covered calls).
- **Year picker bidirectional navigation**: The year panel shows ~10 years. If the target year is ahead of the visible range, navigate forward (`a.mx-icon-next-year`); if behind, navigate backward (`a.mx-icon-last-year`). Up to 10 attempts. Previous code only navigated backward, which failed when the FO segment dropdown caused the year panel to show an older decade.
- **Stale DOM race fix (FO hang root cause)**: After switching from EQ to FO, existing CSV links are marked `data-stale="true"`. `waitForSearchResults` uses `:not([data-stale])` CSS selector for non-EQ segments so the Race waits for Vue to re-render fresh results. Without this, the Race matched stale EQ links immediately, clicking them didn't trigger a download event, and `WaitDownload` blocked forever.
- **Calendar date picker timeouts**: All `MustElement`/`MustElements` calls in `selectCalendarDate` use `page.Timeout(10s)` with error returns. Selector misses produce clear error messages instead of indefinite hangs.
- **Critical JS evals with null-checks**: 4 JS `Eval` calls (segment dropdown reset, date picker open, commit search click, segment search click) include `if (!el) throw new Error(...)` null guards with Go error returns. Non-critical evals (body click, debug logging) are left as `MustEval`.
- **Orphaned GUID cleanup**: When `os.Rename` fails after download, the GUID-named temp file is removed to prevent littering ~/Downloads.
- **Error-resilient boundary detection**: When `DownloadTradesForFY` returns an error, 0-record results are synthesized for all requested segments so `consecutiveEmpty` increments and boundary detection continues correctly.
- **CSV click/href via page-level JS**: All interactions with the CSV download link use `Page.Eval` with `document.querySelector(selector)` instead of `Element.MustEval` or `Element.MustClick`. This avoids three Rod quirks: (1) `Element.Eval` passes element as `this`, not as function arg — arrow functions `(el) => el.click()` get `el=undefined`; (2) `Element.MustClick` retries forever on obscured elements; (3) `Timeout(N).Element()` attaches deadline to returned element — subsequent `MustClick`/`MustEval` inherit and panic on expiry. Click errors return 0-record result gracefully.
- **Simplified boundary detection**: No per-segment boundary tracking or user prompts. All segments are checked for every FY. Stop when ALL segments have 0 records for 2 consecutive FYs. No hard floor. Handles accounts where FO data exists only in older FYs (e.g., a few F&O trades years ago, recent activity is EQ-only).
- **Account-scoped idempotency**: `GetDownloadedFYs` filters by `accountNumber` — CSV files from other accounts in `~/Downloads` are ignored. Without this, running for multiple accounts caused cross-account FY skipping.
- **CSV download retry on timeout**: The Zerodha JS click handler intermittently fails to trigger a download (observed on FO segments). The code retries the click once (max 2 attempts, 30s timeout each) before returning 0 records. Logs show `(attempt 1/2) — retrying click` on first timeout.

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

# Build binary for current platform
go build -o broker-trade-sync

# Build all platform binaries into ~/Downloads
bash build.sh

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

All phases complete and verified in production:
- Phase 1 ✅ Project setup
- Phase 2 ✅ Core interface (brokers/broker.go)
- Phase 3 ✅ Zerodha implementation — full end-to-end verified
- Phase 4 ✅ Main CLI — flags, first-run setup, download loop, summary
- Phase 5 ✅ Download manager — idempotency, FY scanning, boundary detection
- Phase 6 ✅ Polish — --verbose, --broker, Ctrl+C
- build.sh ✅ Cross-platform build script (mac-m1, mac-intel, windows.exe → ~/Downloads)
- ✅ Consistent first-run flow: browser always opens before credential prompts (all machines)
- ✅ All three Zerodha 2FA methods supported: TOTP (auto-submit), SMS OTP (explicit submit), mobile app code (explicit submit)
- ✅ Stale search results fixed: fresh navigation + commit search + WaitRequestIdle + Race detection
- ✅ Multi-segment support: single FY loop downloads EQ+FO per FY with independent boundary detection
- ✅ CSV naming includes segment: `ACCOUNT_EQ_FROM_TO.csv` / `ACCOUNT_FO_FROM_TO.csv`
- ✅ Backward compatible: old-format files (no segment) treated as EQ
- ✅ F&O segment dropdown verified on live tradebook page (FY2020-21 through FY2024-25)
- ✅ Commit search pattern prevents stale-date bug when switching segments
- ✅ Year picker bidirectional navigation (forward + backward through decades)
- ✅ Segment dropdown reset to EQ after fresh navigation (SPA persists last selection)
- ✅ Race-based result detection: CSV link vs "Report's empty" text (replaces CSV-timeout)
- ✅ Verified on three accounts: BT2632 (user's account, EQ+FO), CI8364 (wife's account, EQ only, light trader), ZY7393 (third-party, heavy FO trader)
- ✅ Robustness hardening: stale DOM race fix, calendar timeouts, GUID cleanup, error-resilient boundary detection, CSV error logging, JS null-checks
- ✅ Page-level JS for CSV click/href (avoids Element.Eval `this` binding quirk, MustClick hang, and timeout context inheritance)
- ✅ Simplified boundary detection: all segments checked every FY, stop when all empty for 2 consecutive FYs, no prompts
- ✅ Account-scoped idempotency: `GetDownloadedFYs` filters by account number — prevents cross-account CSV files from causing FY skips
- ✅ CSV download retry: on download timeout (JS click handler intermittently fails), retries click once before giving up
- ✅ Full end-to-end clean run verified on ZY7393: 17 files (9 EQ + 8 FO), FY2017-18 through FY2025-26

**Not yet tested (requires live run):**
- Subsequent run after N days: should re-download current FY only, skip all prior FYs. Logic is implemented and correct — `foundActiveFY=true` is set when skipping already-downloaded FYs, ensuring the historical boundary is correctly detected.

## Troubleshooting

- **Login fails**: Check credentials in `.env`; use `--reset` to re-enter
- **2FA code not accepted**: Enter the code in the terminal (not the browser); for TOTP make sure your authenticator app is time-synced; for SMS OTP check your registered mobile number; for mobile app code open the Zerodha app and use the code shown there
- **Download hangs**: Run with `--headless=false` to see browser state
- **Rate limited by Zerodha**: Wait a few minutes and re-run — already-downloaded FYs are skipped
- **Want to change broker/credentials**: Run with `--reset` to clear `.env` and re-trigger setup
