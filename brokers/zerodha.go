package brokers

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

func init() {
	RegisterBroker("zerodha", func(headless bool) (Broker, error) {
		return NewZerodhaBroker(headless)
	})
}

// ZerodhaBroker implements the Broker interface for Zerodha Console
type ZerodhaBroker struct {
	browser       *rod.Browser
	page          *rod.Page
	headless      bool
	accountNumber string
}

// NewZerodhaBroker creates a new Zerodha broker instance
func NewZerodhaBroker(headless bool) (*ZerodhaBroker, error) {
	url := launcher.New().
		Headless(headless).
		MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()
	return &ZerodhaBroker{
		browser:  browser,
		headless: headless,
	}, nil
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

	// After the login button click the page transitions to the TOTP screen.
	// The TOTP input reuses id="userid" but carries aria-label="External TOTP".
	// Wait up to 15 seconds; if it doesn't appear, credentials were likely wrong.
	totpEl, err := z.page.Timeout(15 * time.Second).Element("[label='External TOTP']")
	if err != nil {
		return fmt.Errorf("login failed — check username and password (run with --reset to re-enter credentials)")
	}
	if err := totpEl.WaitVisible(); err != nil {
		return fmt.Errorf("TOTP field did not become visible: %w", err)
	}

	// Prompt for auth code now that the TOTP screen is ready
	if authCode == "" {
		fmt.Print("Enter auth code (TOTP/SMS/email OTP): ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			authCode = strings.TrimSpace(scanner.Text())
		}
	}

	// Set up the navigation wait BEFORE triggering it.
	// Zerodha auto-submits the TOTP form when 6 digits are entered,
	// which navigates away while MustInput is still finishing — that's expected.
	// rod.Try catches the resulting context-cancelled panic so we can proceed.
	waitNav := z.page.MustWaitNavigation()
	rod.Try(func() { totpEl.MustInput(authCode) })
	waitNav()

	return nil
}

// NavigateToTradeBook navigates to the trade history section
func (z *ZerodhaBroker) NavigateToTradeBook() error {
	z.page.MustNavigate("https://console.zerodha.com/reports/tradebook")
	z.page.MustWaitLoad()
	time.Sleep(2 * time.Second)
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

	// Clear any existing date selection before setting a new one
	if clearBtn, err := z.page.Timeout(1 * time.Second).Element("span.mx-clear-wrapper"); err == nil {
		clearBtn.MustClick()
		time.Sleep(300 * time.Millisecond)
	}

	// Open the date range picker
	z.page.MustElement("div.three input").MustClick()
	time.Sleep(500 * time.Millisecond)

	// Use preset buttons for current/prev FY; manually navigate calendar for older FYs
	prevFY := PreviousFY(currentFY)
	switch fy.Label {
	case currentFY.Label:
		z.page.MustElement("button:nth-of-type(4)").MustClick() // "current FY"
	case prevFY.Label:
		z.page.MustElement("button:nth-of-type(3)").MustClick() // "prev. FY"
	default:
		if err := z.selectCalendarDate(1, fy.StartDate); err != nil {
			return nil, fmt.Errorf("selecting start date: %w", err)
		}
		if err := z.selectCalendarDate(2, fy.EndDate); err != nil {
			return nil, fmt.Errorf("selecting end date: %w", err)
		}
	}
	time.Sleep(300 * time.Millisecond)

	// Click the search/filter button
	z.page.MustElement("div.one span").MustClick()
	time.Sleep(2 * time.Second)

	// Check for the CSV download link — absent when there are no trades
	csvEl, err := z.page.Timeout(5 * time.Second).Element("div.table-section a:nth-of-type(2)")
	if err != nil {
		return &DownloadResult{Filename: targetFilename, RecordCount: 0, FY: fy}, nil
	}

	// Intercept the download before clicking so we know the filename
	wait := z.browser.WaitDownload(downloadDir)
	csvEl.MustClick()
	info := wait()

	// Poll until the file is fully written (no .crdownload temp file)
	downloadedPath := filepath.Join(downloadDir, info.SuggestedFilename)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		_, crErr := os.Stat(downloadedPath + ".crdownload")
		_, fileErr := os.Stat(downloadedPath)
		if os.IsNotExist(crErr) && fileErr == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Rename to our naming convention
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
// to the given date and clicks it. Uses the year picker then month picker for efficiency.
func (z *ZerodhaBroker) selectCalendarDate(pane int, date time.Time) error {
	paneSelector := fmt.Sprintf("div.mx-range-wrapper > div:nth-of-type(%d)", pane)

	// Click the year label in the calendar header to open the year picker
	z.page.MustElement(paneSelector + " a:nth-of-type(6)").MustClick()
	time.Sleep(300 * time.Millisecond)

	// Find and click the target year; navigate to previous decade if not visible
	targetYear := date.Year()
	for attempt := 0; attempt < 5; attempt++ {
		spans := z.page.MustElements("div.mx-calendar-panel-year .mx-panel-year span")
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
			return fmt.Errorf("year %d not found in date picker after navigating back 5 decades", targetYear)
		}
		// Go to previous decade and try again
		z.page.MustElement("div.mx-calendar-panel-year a.mx-icon-last-year").MustClick()
		time.Sleep(300 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)

	// Click the target month (span index = month number: 1=Jan, 4=Apr, 3=Mar, etc.)
	month := int(date.Month())
	z.page.MustElement(fmt.Sprintf(
		"div.mx-calendar-panel-month .mx-panel-month > span:nth-of-type(%d)", month,
	)).MustClick()
	time.Sleep(300 * time.Millisecond)

	// Click the target day within the current-month cells of this pane
	day := date.Day()
	cells := z.page.MustElements(paneSelector + " table tbody td.cur-month")
	for _, cell := range cells {
		d, _ := strconv.Atoi(strings.TrimSpace(cell.MustText()))
		if d == day {
			cell.MustClick()
			return nil
		}
	}
	return fmt.Errorf("day %d not found in calendar for %s", day, date.Format("2006-01"))
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
