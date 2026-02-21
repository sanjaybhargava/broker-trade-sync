package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// ZerodhaBroker implements the Broker interface for Zerodha Console
type ZerodhaBroker struct {
	browser       *rod.Browser
	page          *rod.Page
	headless      bool
	accountNumber string
}

// NewZerodhaBroker creates a new Zerodha broker instance
func NewZerodhaBroker(headless bool) (*ZerodhaBroker, error) {
	// Launch browser
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
	// Navigate to Zerodha Console login
	z.page = z.browser.MustPage("https://console.zerodha.com/")

	// Wait for login form to load
	z.page.MustWaitLoad()

	// Enter username
	z.page.MustElement("input[type='text']").MustInput(username)

	// Enter password
	z.page.MustElement("input[type='password']").MustInput(password)

	// Submit login form
	z.page.MustElement("button[type='submit']").MustClick()

	// Wait for auth code page
	time.Sleep(2 * time.Second)

	// Prompt user for auth code if not provided
	if authCode == "" {
		fmt.Print("Enter auth code (TOTP/SMS/email OTP): ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			authCode = strings.TrimSpace(scanner.Text())
		}
	}

	// Enter auth code
	z.page.MustElement("input[type='text']").MustInput(authCode)

	// Submit auth code
	z.page.MustElement("button[type='submit']").MustClick()

	// Wait for dashboard to load
	time.Sleep(3 * time.Second)
	z.page.MustWaitLoad()

	return nil
}

// NavigateToTradeBook navigates to the trade history section
func (z *ZerodhaBroker) NavigateToTradeBook() error {
	// TODO: Navigate to the specific trade book URL or click through menus
	// This will need to be adjusted based on Zerodha Console's actual structure
	z.page.MustNavigate("https://console.zerodha.com/reports/tradebook")
	z.page.MustWaitLoad()
	time.Sleep(2 * time.Second)

	return nil
}

// DownloadTradesForFY downloads the CSV for a specific financial year
func (z *ZerodhaBroker) DownloadTradesForFY(fy FinancialYear, downloadDir string, accountNumber string) (*DownloadResult, error) {
	// TODO: Implement date range selection and CSV download
	// This is a placeholder that needs to be filled based on Zerodha Console's actual UI

	// Set date range in the UI
	// ...

	// Click download CSV button
	// ...

	// Wait for download
	// ...

	// Move/rename downloaded file to match our naming convention
	targetFilename := GenerateCSVFilename(accountNumber, fy)
	targetPath := filepath.Join(downloadDir, targetFilename)

	// Count records in CSV (placeholder - read actual downloaded file)
	recordCount, err := countCSVRecords(targetPath)
	if err != nil {
		// If file doesn't exist, assume no records
		recordCount = 0
	}

	return &DownloadResult{
		Filename:    targetFilename,
		RecordCount: recordCount,
		FY:          fy,
	}, nil
}

// GetAccountNumber extracts the account/client ID from the logged-in session
func (z *ZerodhaBroker) GetAccountNumber() (string, error) {
	if z.accountNumber != "" {
		return z.accountNumber, nil
	}

	// TODO: Extract account number from page
	// This could be from a user profile element, URL, or page content
	// Placeholder - needs actual implementation

	z.accountNumber = "PLACEHOLDER"
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

	// Subtract 1 for header row
	count := len(records) - 1
	if count < 0 {
		count = 0
	}

	return count, nil
}
