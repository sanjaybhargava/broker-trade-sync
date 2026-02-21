package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

var (
	headless = flag.Bool("headless", true, "Run browser in headless mode")
	verbose  = flag.Bool("verbose", false, "Enable verbose logging")
)

func main() {
	flag.Parse()

	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found, using environment variables")
	}

	// Get credentials from environment
	username := os.Getenv("ZERODHA_USERNAME")
	password := os.Getenv("ZERODHA_PASSWORD")
	totpSecret := os.Getenv("ZERODHA_TOTP_SECRET")

	if username == "" || password == "" || totpSecret == "" {
		log.Fatal("Missing required credentials. Please set ZERODHA_USERNAME, ZERODHA_PASSWORD, and ZERODHA_TOTP_SECRET in .env file")
	}

	// Ensure downloads directory exists
	downloadDir := filepath.Join(".", "downloads")
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		log.Fatalf("Failed to create downloads directory: %v", err)
	}

	// Initialize broker
	broker, err := NewZerodhaBroker(*headless)
	if err != nil {
		log.Fatalf("Failed to initialize Zerodha broker: %v", err)
	}
	defer broker.Close()

	// Login
	log.Printf("Logging into %s...", broker.Name())
	if err := broker.Login(username, password, totpSecret); err != nil {
		log.Fatalf("Login failed: %v", err)
	}
	log.Println("Login successful!")

	// Get account number
	accountNumber, err := broker.GetAccountNumber()
	if err != nil {
		log.Fatalf("Failed to get account number: %v", err)
	}
	log.Printf("Account number: %s", accountNumber)

	// Navigate to trade book
	if err := broker.NavigateToTradeBook(); err != nil {
		log.Fatalf("Failed to navigate to trade book: %v", err)
	}

	// Check which FYs are already downloaded
	downloadedFYs, err := GetDownloadedFYs(downloadDir)
	if err != nil {
		log.Fatalf("Failed to scan downloads: %v", err)
	}

	if *verbose {
		log.Printf("Found %d existing downloads", len(downloadedFYs))
	}

	// Download logic
	var results []*DownloadResult
	currentFY := CurrentFY()
	fy := currentFY
	foundActiveFY := false
	consecutiveEmptyFYs := 0
	maxEmptyFYsToCheck := 3

	for {
		// Skip if already downloaded (except current FY which might be incomplete)
		if _, exists := downloadedFYs[fy.Label]; exists && fy.Label != currentFY.Label {
			log.Printf("Skipping %s (already downloaded)", fy.Label)
			fy = PreviousFY(fy)
			continue
		}

		log.Printf("Downloading trades for %s...", fy.Label)
		result, err := broker.DownloadTradesForFY(fy, downloadDir, accountNumber)
		if err != nil {
			log.Printf("Error downloading %s: %v", fy.Label, err)
			fy = PreviousFY(fy)
			continue
		}

		if result.RecordCount == 0 {
			consecutiveEmptyFYs++

			if !foundActiveFY {
				// Haven't found any trades yet
				if consecutiveEmptyFYs >= maxEmptyFYsToCheck {
					if !promptContinue("No records found. Check 3 more financial years?") {
						log.Println("Stopping search")
						break
					}
					consecutiveEmptyFYs = 0
				}
			} else {
				// We had active FYs before, now hit empty - stop condition
				log.Printf("No records in %s. Reached historical boundary.", fy.Label)
				break
			}
		} else {
			foundActiveFY = true
			consecutiveEmptyFYs = 0
			results = append(results, result)
			log.Printf("Downloaded %s: %d records", result.Filename, result.RecordCount)
		}

		fy = PreviousFY(fy)
	}

	// Print summary
	printSummary(results)
}

func promptContinue(message string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [y/N]: ", message)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

func printSummary(results []*DownloadResult) {
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("DOWNLOAD SUMMARY")
	fmt.Println(strings.Repeat("=", 50))

	if len(results) == 0 {
		fmt.Println("No files downloaded in this session.")
		return
	}

	totalRecords := 0
	for _, r := range results {
		fmt.Printf("  %s - %d records\n", r.Filename, r.RecordCount)
		totalRecords += r.RecordCount
	}

	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("Total: %d files, %d records\n", len(results), totalRecords)
}
