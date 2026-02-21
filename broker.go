package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// FinancialYear represents an Indian financial year (April 1 to March 31)
type FinancialYear struct {
	Label     string    // e.g., "FY2023-24"
	StartDate time.Time // April 1
	EndDate   time.Time // March 31 of next year
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

// CurrentFY returns the current Indian financial year
func CurrentFY() FinancialYear {
	now := time.Now()
	year := now.Year()

	// If we're in Jan-Mar, FY started last year
	// If we're in Apr-Dec, FY started this year
	if now.Month() < time.April {
		year--
	}

	startDate := time.Date(year, time.April, 1, 0, 0, 0, 0, time.Local)
	endDate := time.Date(year+1, time.March, 31, 23, 59, 59, 0, time.Local)

	return FinancialYear{
		Label:     fmt.Sprintf("FY%d-%02d", year, (year+1)%100),
		StartDate: startDate,
		EndDate:   endDate,
	}
}

// PreviousFY returns the financial year before the given FY
func PreviousFY(fy FinancialYear) FinancialYear {
	year := fy.StartDate.Year() - 1
	startDate := time.Date(year, time.April, 1, 0, 0, 0, 0, time.Local)
	endDate := time.Date(year+1, time.March, 31, 23, 59, 59, 0, time.Local)

	return FinancialYear{
		Label:     fmt.Sprintf("FY%d-%02d", year, (year+1)%100),
		StartDate: startDate,
		EndDate:   endDate,
	}
}

// FormatDateForFilename formats a date as YYYYMMDD for CSV filenames
func FormatDateForFilename(t time.Time) string {
	return t.Format("20060102")
}

// GenerateCSVFilename creates a filename following the convention: accountnumber_fromdate_todate.csv
func GenerateCSVFilename(accountNumber string, fy FinancialYear) string {
	return fmt.Sprintf("%s_%s_%s.csv",
		accountNumber,
		FormatDateForFilename(fy.StartDate),
		FormatDateForFilename(fy.EndDate),
	)
}

// ParseFYFromFilename extracts the financial year and account number from a CSV filename
// Returns the FY, account number, and any error
func ParseFYFromFilename(filename string) (*FinancialYear, string, error) {
	base := filepath.Base(filename)
	// Pattern: accountnumber_YYYYMMDD_YYYYMMDD.csv
	re := regexp.MustCompile(`^([A-Z0-9]+)_(\d{8})_(\d{8})\.csv$`)
	matches := re.FindStringSubmatch(base)
	if matches == nil {
		return nil, "", fmt.Errorf("filename does not match expected pattern: %s", filename)
	}

	accountNumber := matches[1]
	startStr := matches[2]
	endStr := matches[3]

	startYear, _ := strconv.Atoi(startStr[:4])
	startMonth, _ := strconv.Atoi(startStr[4:6])
	startDay, _ := strconv.Atoi(startStr[6:8])

	endYear, _ := strconv.Atoi(endStr[:4])
	endMonth, _ := strconv.Atoi(endStr[4:6])
	endDay, _ := strconv.Atoi(endStr[6:8])

	startDate := time.Date(startYear, time.Month(startMonth), startDay, 0, 0, 0, 0, time.Local)
	endDate := time.Date(endYear, time.Month(endMonth), endDay, 23, 59, 59, 0, time.Local)

	fy := &FinancialYear{
		Label:     fmt.Sprintf("FY%d-%02d", startYear, (startYear+1)%100),
		StartDate: startDate,
		EndDate:   endDate,
	}

	return fy, accountNumber, nil
}

// GetDownloadedFYs scans the downloads directory and returns a map of already downloaded FYs
// Key is the FY label (e.g., "FY2023-24"), value is the filename
func GetDownloadedFYs(downloadDir string) (map[string]string, error) {
	downloaded := make(map[string]string)

	pattern := filepath.Join(downloadDir, "*.csv")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to scan downloads directory: %w", err)
	}

	for _, file := range files {
		fy, _, err := ParseFYFromFilename(file)
		if err != nil {
			// Skip files that don't match the pattern
			continue
		}
		downloaded[fy.Label] = filepath.Base(file)
	}

	return downloaded, nil
}
