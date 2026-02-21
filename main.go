package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	"broker-trade-sync/brokers"
)

var (
	headless = flag.Bool("headless", true, "Run browser in headless mode")
	verbose  = flag.Bool("verbose", false, "Enable verbose logging")
	reset    = flag.Bool("reset", false, "Clear saved credentials and re-run setup")
)

const envFile = ".env"

func main() {
	flag.Parse()

	// Handle --reset: delete .env to force first-run setup
	if *reset {
		if err := os.Remove(envFile); err != nil && !os.IsNotExist(err) {
			log.Fatalf("Failed to remove .env: %v", err)
		}
		log.Println("Credentials cleared. Starting fresh setup...")
	}

	// First-run setup if .env doesn't exist
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		if err := runFirstRunSetup(); err != nil {
			log.Fatalf("Setup failed: %v", err)
		}
	}

	// Load .env silently
	if err := godotenv.Load(envFile); err != nil {
		log.Fatalf("Failed to load .env: %v", err)
	}

	brokerName := os.Getenv("BROKER")
	username := os.Getenv(strings.ToUpper(brokerName) + "_USERNAME")
	password := os.Getenv(strings.ToUpper(brokerName) + "_PASSWORD")

	if brokerName == "" || username == "" || password == "" {
		log.Fatal("Missing credentials in .env. Run with --reset to reconfigure.")
	}

	// Use the user's Downloads folder — familiar on both macOS and Windows
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Could not determine home directory: %v", err)
	}
	downloadDir := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		log.Fatalf("Failed to create downloads directory: %v", err)
	}

	// Initialize broker
	broker, err := brokers.NewBroker(brokerName, *headless)
	if err != nil {
		log.Fatalf("Failed to initialize broker: %v", err)
	}
	defer broker.Close()

	// Handle Ctrl+C: close browser and exit cleanly
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nInterrupted. Closing browser...")
		broker.Close()
		os.Exit(1)
	}()

	// Login (broker prompts for auth code at runtime)
	log.Printf("Logging into %s...", broker.Name())
	if err := broker.Login(username, password, ""); err != nil {
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
	downloadedFYs, err := brokers.GetDownloadedFYs(downloadDir)
	if err != nil {
		log.Fatalf("Failed to scan downloads: %v", err)
	}

	if *verbose {
		log.Printf("Found %d existing downloads", len(downloadedFYs))
	}

	// Download logic
	var results []*brokers.DownloadResult
	currentFY := brokers.CurrentFY()
	fy := currentFY
	foundActiveFY := false
	consecutiveEmptyFYs := 0
	maxEmptyFYsToCheck := 3

	for {
		// Skip already downloaded FYs (except current FY which may be incomplete)
		if _, exists := downloadedFYs[fy.Label]; exists && fy.Label != currentFY.Label {
			log.Printf("Skipping %s (already downloaded)", fy.Label)
			fy = brokers.PreviousFY(fy)
			continue
		}

		log.Printf("Downloading trades for %s...", fy.Label)
		result, err := broker.DownloadTradesForFY(fy, downloadDir, accountNumber)
		if err != nil {
			log.Printf("Error downloading %s: %v", fy.Label, err)
			fy = brokers.PreviousFY(fy)
			continue
		}

		if result.RecordCount == 0 {
			consecutiveEmptyFYs++
			if !foundActiveFY {
				if consecutiveEmptyFYs >= maxEmptyFYsToCheck {
					if !promptContinue("No records found. Check 3 more financial years?") {
						log.Println("Stopping search")
						break
					}
					consecutiveEmptyFYs = 0
				}
			} else {
				log.Printf("No records in %s. Reached historical boundary.", fy.Label)
				break
			}
		} else {
			foundActiveFY = true
			consecutiveEmptyFYs = 0
			results = append(results, result)
			log.Printf("Downloaded %s: %d records", result.Filename, result.RecordCount)
		}

		fy = brokers.PreviousFY(fy)
	}

	printSummary(results)
}

func runFirstRunSetup() error {
	reader := bufio.NewReader(os.Stdin)

	// Show broker menu
	available := brokers.ListBrokers()
	sort.Strings(available)

	fmt.Println("\nSelect your broker:")
	for i, name := range available {
		fmt.Printf("  %d. %s\n", i+1, strings.ToUpper(name[:1])+name[1:])
	}
	fmt.Println()
	fmt.Println("If your broker is not listed, email support@bharosaclub.com with your broker name to request it be added. We will confirm once it is available.")
	fmt.Println()
	fmt.Print("Enter number: ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > len(available) {
		return fmt.Errorf("invalid selection: %s", input)
	}
	brokerName := available[choice-1]

	// Prompt username
	fmt.Print("Username: ")
	username, _ := reader.ReadString('\n')
	username = strings.TrimSpace(username)

	// Prompt password (visible input)
	fmt.Print("Password: ")
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	// Write .env
	envContent := fmt.Sprintf("BROKER=%s\n%s_USERNAME=%s\n%s_PASSWORD=%s\n",
		brokerName,
		strings.ToUpper(brokerName), username,
		strings.ToUpper(brokerName), password,
	)
	if err := os.WriteFile(envFile, []byte(envContent), 0600); err != nil {
		return fmt.Errorf("failed to write .env: %w", err)
	}

	fmt.Printf("Setup complete. Credentials saved to %s\n\n", envFile)
	return nil
}

func promptContinue(message string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [y/N]: ", message)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

func printSummary(results []*brokers.DownloadResult) {
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
