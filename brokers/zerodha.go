package brokers

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

func init() {
	RegisterBroker("zerodha", func(headless bool, verbose bool) (Broker, error) {
		return NewZerodhaBroker(headless, verbose)
	})
}

// ZerodhaBroker implements the Broker interface for Zerodha Console
type ZerodhaBroker struct {
	browser       *rod.Browser
	page          *rod.Page
	headless      bool
	verbose       bool
	accountNumber string
}

// NewZerodhaBroker creates a new Zerodha broker instance
func NewZerodhaBroker(headless bool, verbose bool) (*ZerodhaBroker, error) {
	url := launcher.New().
		Headless(headless).
		MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()
	return &ZerodhaBroker{
		browser:  browser,
		headless: headless,
		verbose:  verbose,
	}, nil
}

// debugLog prints only when verbose mode is enabled
func (z *ZerodhaBroker) debugLog(format string, args ...interface{}) {
	if z.verbose {
		log.Printf(format, args...)
	}
}

// Name returns the broker identifier
func (z *ZerodhaBroker) Name() string {
	return "zerodha"
}

// Login opens browser, navigates to Zerodha Console, and authenticates.
// It prompts the user at runtime for the auth code (TOTP/SMS/email OTP).
func (z *ZerodhaBroker) Login(username, password, authCode string) error {
	// In Zerodha the username is the client/account ID
	z.accountNumber = username

	// Navigate to Zerodha Console landing page
	z.page = z.browser.MustPage("https://console.zerodha.com/")

	// The landing page may show a "Login with Kite" button that navigates to the
	// kite login page. If present, wait for the navigation it triggers before
	// proceeding — otherwise we race ahead before the login form is loaded.
	if loginBtn, err := z.page.Timeout(5 * time.Second).Element("button.btn-blue"); err == nil {
		waitConsoleNav := z.page.MustWaitNavigation()
		loginBtn.MustClick()
		waitConsoleNav()
	}

	// Now on the kite login page. Use a timeout so we get a clear error instead
	// of an indefinite hang if the selector is wrong or navigation failed.
	userEl, err := z.page.Timeout(15 * time.Second).Element("#userid")
	if err != nil {
		return fmt.Errorf("could not find login form — check that browser reached kite login page: %w", err)
	}
	userEl.MustWaitVisible().MustInput(username)
	z.page.MustElement("#password").MustInput(password)
	z.page.MustElement("button[type='submit']").MustClick()

	// After submitting credentials the page transitions to a 2FA screen.
	// Zerodha uses different label values depending on the user's 2FA method
	// (e.g. "External TOTP", mobile app code, etc.) so we wait for the common
	// structural signature: a 6-digit numeric input that reuses id="userid".
	authEl, err := z.page.Timeout(15 * time.Second).Element(`input[type='number'][maxlength='6']`)
	if err != nil {
		return fmt.Errorf("login failed — check username and password (run with --reset to re-enter credentials)")
	}
	if err := authEl.WaitVisible(); err != nil {
		return fmt.Errorf("2FA field did not become visible: %w", err)
	}

	// Prompt for auth code now that the 2FA screen is ready
	if authCode == "" {
		fmt.Print("Enter auth code (TOTP / mobile app code): ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			authCode = strings.TrimSpace(scanner.Text())
		}
		if authCode == "" {
			return fmt.Errorf("no auth code entered — re-run and type your code when prompted")
		}
	}

	// Re-fetch the element just before typing — the reference can go stale
	// while the user is typing their auth code. Click to ensure focus.
	authEl = z.page.MustElement(`input[type='number'][maxlength='6']`)
	authEl.MustClick()

	// Type the code. TOTP auto-submits on the 6th digit, navigating the page
	// away mid-input — rod.Try catches the resulting context-cancelled panic.
	// Mobile app code does NOT auto-submit, so we also click the submit button
	// afterwards; rod.Try makes that a no-op if TOTP already navigated away.
	rod.Try(func() { authEl.MustInput(authCode) })
	rod.Try(func() { z.page.MustElement("button[type='submit']").MustClick() })

	// Wait for the authenticated console dashboard. Try the tradebook sidebar
	// link first (reliable for most accounts). If it doesn't appear within 30s,
	// fall back to checking the URL — if we're on console.zerodha.com the login
	// succeeded even if the tradebook link isn't present in this account's sidebar.
	if _, err := z.page.Timeout(30 * time.Second).Element(`a[href*="tradebook"]`); err != nil {
		if info, infoErr := z.page.Info(); infoErr == nil && strings.Contains(info.URL, "console.zerodha.com") {
			return nil
		}
		return fmt.Errorf("dashboard did not load after 2FA — code may be wrong or expired (run with --reset to re-enter credentials)")
	}
	return nil
}

// NavigateToTradeBook navigates to the trade history section
func (z *ZerodhaBroker) NavigateToTradeBook() error {
	z.page.MustNavigate("https://console.zerodha.com/reports/tradebook")
	// Wait for the calendar icon — confirms the Vue datepicker has mounted.
	// Skipping MustWaitLoad(): the tradebook is a SPA where the load event is
	// unreliable, and the icon appearing is the real readiness signal.
	if _, err := z.page.Timeout(20 * time.Second).Element("svg.mx-calendar-icon"); err != nil {
		return fmt.Errorf("tradebook date picker did not appear: %w", err)
	}
	return nil
}

// DownloadTradesForFY downloads the CSV for a specific financial year.
// Returns RecordCount=0 if no trades exist for that FY.
func (z *ZerodhaBroker) DownloadTradesForFY(fy FinancialYear, downloadDir string, accountNumber string) (*DownloadResult, error) {
	targetFilename := GenerateCSVFilename(accountNumber, fy)
	targetPath := filepath.Join(downloadDir, targetFilename)

	// Skip if already downloaded — except current FY which may be incomplete
	currentFY := CurrentFY()
	if fy.Label != currentFY.Label {
		if _, err := os.Stat(targetPath); err == nil {
			recordCount, _ := countCSVRecords(targetPath)
			return &DownloadResult{Filename: targetFilename, RecordCount: recordCount, FY: fy}, nil
		}
	}

	// Brief pause between FY downloads to avoid Zerodha rate limiting.
	time.Sleep(5 * time.Second)

	// Open the date picker via JS — Rod's MustClick() hangs on SVG elements
	// because it can't resolve their click geometry reliably.
	z.debugLog("clicking .mx-input-wrapper to open date picker")
	z.page.MustEval(`() => document.querySelector('.mx-input-wrapper').click()`)

	z.debugLog("waiting for date picker popup")
	if err := z.page.MustElement(".mx-datepicker-popup").WaitVisible(); err != nil {
		return nil, fmt.Errorf("date picker did not open: %w", err)
	}
	z.debugLog("date picker open")

	// Use today as end date for current FY — the FY end (Mar 31) may be in the
	// future and the calendar won't allow clicking future dates.
	startDate := fy.StartDate
	endDate := fy.EndDate
	if endDate.After(time.Now()) {
		endDate = time.Now()
	}

	z.debugLog("selecting start date %s", startDate.Format("2006-01-02"))
	if err := z.selectCalendarDate(1, startDate); err != nil {
		return nil, fmt.Errorf("selecting start date: %w", err)
	}
	z.debugLog("selecting end date %s", endDate.Format("2006-01-02"))
	if err := z.selectCalendarDate(2, endDate); err != nil {
		return nil, fmt.Errorf("selecting end date: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	z.debugLog("clicking search button")
	z.page.MustElement("div.one span").MustClick()
	time.Sleep(2 * time.Second)

	// Check for the CSV download link — absent when there are no trades
	csvEl, err := z.page.Timeout(5 * time.Second).Element("div.table-section a:nth-of-type(2)")
	if err != nil {
		return &DownloadResult{Filename: targetFilename, RecordCount: 0, FY: fy}, nil
	}

	// Intercept the download before clicking. Rod saves the file using the
	// download GUID as filename (not the suggested filename).
	wait := z.browser.WaitDownload(downloadDir)
	csvEl.MustClick()
	info := wait() // blocks until download is complete and file is written

	// Rod writes the file as <GUID> in downloadDir — rename to our convention
	downloadedPath := filepath.Join(downloadDir, info.GUID)
	if err := os.Rename(downloadedPath, targetPath); err != nil {
		return nil, fmt.Errorf("renaming downloaded file: %w", err)
	}

	recordCount, _ := countCSVRecords(targetPath)
	if recordCount == 0 {
		os.Remove(targetPath) // don't keep empty CSV files
	}
	return &DownloadResult{Filename: targetFilename, RecordCount: recordCount, FY: fy}, nil
}

// selectCalendarDate navigates the vue2-datepicker calendar pane (1=left/From, 2=right/To)
// to the given date and clicks it. Flow: click year label → pick year → pick month → pick day.
func (z *ZerodhaBroker) selectCalendarDate(pane int, date time.Time) error {
	paneSelector := fmt.Sprintf("div.mx-range-wrapper > div:nth-of-type(%d)", pane)

	z.debugLog("pane %d: selecting year %d", pane, date.Year())
	z.page.MustElement(paneSelector + " a.mx-current-year").MustClick()
	time.Sleep(300 * time.Millisecond)

	// Find and click the target year span; navigate decade if not visible
	targetYear := date.Year()
	for attempt := 0; attempt < 5; attempt++ {
		spans := z.page.MustElements(paneSelector + " .mx-panel-year span")
		found := false
		for _, span := range spans {
			y, _ := strconv.Atoi(strings.TrimSpace(span.MustText()))
			if y == targetYear {
				span.MustClick()
				found = true
				break
			}
		}
		if found {
			break
		}
		if attempt == 4 {
			return fmt.Errorf("year %d not found in date picker", targetYear)
		}
		z.page.MustElement(paneSelector + " .mx-calendar-header a.mx-icon-last-year").MustClick()
		time.Sleep(300 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)

	// Click the target month span (1=Jan, 4=Apr, 3=Mar, etc.)
	month := int(date.Month())
	z.page.MustElement(fmt.Sprintf(paneSelector+" .mx-panel-month span:nth-of-type(%d)", month)).MustClick()
	time.Sleep(300 * time.Millisecond)

	// Click the day cell by its title attribute (format: YYYY-MM-DD) — unambiguous
	z.page.MustElement(fmt.Sprintf(`td[title="%s"]`, date.Format("2006-01-02"))).MustClick()
	return nil
}

// GetAccountNumber returns the client ID, which for Zerodha is the username
func (z *ZerodhaBroker) GetAccountNumber() (string, error) {
	return z.accountNumber, nil
}

// Close cleans up browser resources
func (z *ZerodhaBroker) Close() error {
	if z.browser != nil {
		return z.browser.Close()
	}
	return nil
}

// countCSVRecords counts the number of data rows in a CSV file
func countCSVRecords(filePath string) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return 0, err
	}

	count := len(records) - 1
	if count < 0 {
		count = 0
	}
	return count, nil
}
