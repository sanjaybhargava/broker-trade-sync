package brokers

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
	z.page = z.browser.MustPage("https://console.zerodha.com/")
	z.page.MustWaitLoad()

	z.page.MustElement("input[type='text']").MustInput(username)
	z.page.MustElement("input[type='password']").MustInput(password)
	z.page.MustElement("button[type='submit']").MustClick()

	time.Sleep(2 * time.Second)

	if authCode == "" {
		fmt.Print("Enter auth code (TOTP/SMS/email OTP): ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			authCode = strings.TrimSpace(scanner.Text())
		}
	}

	z.page.MustElement("input[type='text']").MustInput(authCode)
	z.page.MustElement("button[type='submit']").MustClick()

	time.Sleep(3 * time.Second)
	z.page.MustWaitLoad()

	return nil
}

// NavigateToTradeBook navigates to the trade history section
func (z *ZerodhaBroker) NavigateToTradeBook() error {
	z.page.MustNavigate("https://console.zerodha.com/reports/tradebook")
	z.page.MustWaitLoad()
	time.Sleep(2 * time.Second)
	return nil
}

// DownloadTradesForFY downloads the CSV for a specific financial year
func (z *ZerodhaBroker) DownloadTradesForFY(fy FinancialYear, downloadDir string, accountNumber string) (*DownloadResult, error) {
	// TODO: Implement date range selection and CSV download
	targetFilename := GenerateCSVFilename(accountNumber, fy)
	targetPath := filepath.Join(downloadDir, targetFilename)

	recordCount, err := countCSVRecords(targetPath)
	if err != nil {
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

	count := len(records) - 1
	if count < 0 {
		count = 0
	}
	return count, nil
}
