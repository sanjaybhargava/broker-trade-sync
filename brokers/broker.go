package brokers

import (
	"fmt"
	"path/filepath"
	"regexp"
	"time"
)

// Segment represents a tradebook segment (e.g., Equity, F&O)
type Segment string

const (
	SegmentEQ Segment = "EQ" // Equity
	SegmentFO Segment = "FO" // Futures and Options
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
	Segment     Segment
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

	// DownloadTradesForFY downloads CSVs for a specific financial year across the given segments.
	// Navigates once and sets the date range, then iterates segments (switching the dropdown).
	// Returns one result per segment. RecordCount=0 means no trades for that segment.
	DownloadTradesForFY(fy FinancialYear, downloadDir string, accountNumber string, segments []Segment) ([]*DownloadResult, error)

	// GetAccountNumber extracts the account/client ID from the logged-in session
	GetAccountNumber() (string, error)

	// Close cleans up browser resources
	Close() error
}

// registry holds all registered broker constructors
var registry = map[string]func(headless bool, verbose bool) (Broker, error){}

// RegisterBroker adds a broker constructor to the registry
func RegisterBroker(name string, constructor func(headless bool, verbose bool) (Broker, error)) {
	registry[name] = constructor
}

// NewBroker creates a broker instance by name
func NewBroker(name string, headless bool, verbose bool) (Broker, error) {
	constructor, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown broker: %s", name)
	}
	return constructor(headless, verbose)
}

// ListBrokers returns all registered broker names (unsorted — caller should sort if needed).
func ListBrokers() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// CurrentFY returns the current Indian financial year
func CurrentFY() FinancialYear {
	now := time.Now()
	year := now.Year()
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

// GenerateCSVFilename creates a filename following the convention: accountnumber_segment_fromdate_todate.csv
func GenerateCSVFilename(accountNumber string, fy FinancialYear, segment Segment) string {
	return fmt.Sprintf("%s_%s_%s_%s.csv",
		accountNumber,
		string(segment),
		FormatDateForFilename(fy.StartDate),
		FormatDateForFilename(fy.EndDate),
	)
}

// Precompiled filename patterns — avoids recompiling on every call during directory scans.
var (
	// New format: ACCOUNT_SEGMENT_YYYYMMDD_YYYYMMDD.csv (e.g., BT2632_EQ_20230401_20240331.csv)
	reFilenameNew = regexp.MustCompile(`^([A-Z0-9]+)_([A-Z]+)_(\d{8})_(\d{8})\.csv$`)
	// Old format: ACCOUNT_YYYYMMDD_YYYYMMDD.csv (treated as EQ for backward compatibility)
	reFilenameOld = regexp.MustCompile(`^([A-Z0-9]+)_(\d{8})_(\d{8})\.csv$`)
)

// ParseFYFromFilename extracts the financial year, account number, and segment from a CSV filename.
// Supports both new format (ACCOUNT_SEGMENT_FROM_TO.csv) and old format (ACCOUNT_FROM_TO.csv = EQ).
func ParseFYFromFilename(filename string) (*FinancialYear, string, Segment, error) {
	base := filepath.Base(filename)

	if matches := reFilenameNew.FindStringSubmatch(base); matches != nil {
		fy, err := parseDates(matches[3], matches[4])
		if err != nil {
			return nil, "", "", err
		}
		return fy, matches[1], Segment(matches[2]), nil
	}

	if matches := reFilenameOld.FindStringSubmatch(base); matches != nil {
		fy, err := parseDates(matches[2], matches[3])
		if err != nil {
			return nil, "", "", err
		}
		return fy, matches[1], SegmentEQ, nil
	}

	return nil, "", "", fmt.Errorf("filename does not match expected pattern: %s", filename)
}

// parseDates parses YYYYMMDD start/end strings into a FinancialYear.
// The regex already guarantees 8-digit strings, so Atoi failures are guarded but unlikely.
func parseDates(startStr, endStr string) (*FinancialYear, error) {
	startDate, err := time.ParseInLocation("20060102", startStr, time.Local)
	if err != nil {
		return nil, fmt.Errorf("invalid start date %q: %w", startStr, err)
	}
	endDate, err := time.ParseInLocation("20060102", endStr, time.Local)
	if err != nil {
		return nil, fmt.Errorf("invalid end date %q: %w", endStr, err)
	}
	// Set end-of-day for the end date
	endDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 0, time.Local)

	return &FinancialYear{
		Label:     fmt.Sprintf("FY%d-%02d", startDate.Year(), (startDate.Year()+1)%100),
		StartDate: startDate,
		EndDate:   endDate,
	}, nil
}

// GetDownloadedFYs scans the downloads directory and returns a map of already downloaded FYs
// for the specified account and segment. Key is the FY label (e.g., "FY2023-24"), value is the filename.
// Old-format files (no segment) are treated as EQ.
func GetDownloadedFYs(downloadDir string, accountNumber string, segment Segment) (map[string]string, error) {
	downloaded := make(map[string]string)
	pattern := filepath.Join(downloadDir, "*.csv")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to scan downloads directory: %w", err)
	}
	for _, file := range files {
		fy, fileAccount, fileSeg, err := ParseFYFromFilename(file)
		if err != nil {
			continue
		}
		if fileAccount == accountNumber && fileSeg == segment {
			downloaded[fy.Label] = filepath.Base(file)
		}
	}
	return downloaded, nil
}
