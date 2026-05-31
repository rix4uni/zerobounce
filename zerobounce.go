package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rix4uni/zerobounce/banner"
	"github.com/spf13/pflag"
)

// ANSI color codes for terminal output
var (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
)

func disableColors() {
	colorReset = ""
	colorGreen = ""
	colorRed = ""
	colorYellow = ""
	colorCyan = ""
}

var verbose bool
var stdoutMu sync.Mutex
var fileMu sync.Mutex

func printCheckmark() {
	fmt.Printf("%s✓%s ", colorGreen, colorReset)
}

func printCross() {
	fmt.Printf("%s✗%s ", colorRed, colorReset)
}

func printWarning() {
	fmt.Printf("%s⚠%s ", colorYellow, colorReset)
}

func printResult(result VerifyResult, outputJSON bool, outputCSV bool, checkedCount int, isFirst bool) {
	stdoutMu.Lock()
	defer stdoutMu.Unlock()

	if outputJSON {
		printJSON(result, checkedCount)
		return
	}

	if outputCSV {
		printCSV(result, checkedCount, isFirst)
		return
	}

	// Default human-readable format (single line, tagged)
	var formatTag string
	if result.FormatValid {
		formatTag = fmt.Sprintf("%s[FORMAT:VALID]%s", colorGreen, colorReset)
	} else {
		formatTag = fmt.Sprintf("%s[FORMAT:INVALID]%s", colorRed, colorReset)
	}

	var mxTag string
	if result.MXValid {
		mxTag = fmt.Sprintf("%s[MX:FOUND]%s", colorGreen, colorReset)
	} else {
		mxTag = fmt.Sprintf("%s[MX:NOT FOUND]%s", colorRed, colorReset)
	}

	var smtpTag string
	if result.IsCatchAll {
		smtpTag = fmt.Sprintf("%s[SMTP:CATCHALL]%s", colorGreen, colorReset)
	} else if result.IsBlocked {
		smtpTag = fmt.Sprintf("%s[SMTP:BLOCKED]%s", colorYellow, colorReset)
	} else if result.SMTPValid {
		smtpTag = fmt.Sprintf("%s[SMTP:PASSED]%s", colorGreen, colorReset)
	} else {
		smtpTag = fmt.Sprintf("%s[SMTP:FAILED]%s", colorYellow, colorReset)
	}

	overallValid := result.FormatValid && result.MXValid && result.SMTPValid
	var overallTag string
	if overallValid {
		overallTag = fmt.Sprintf("%sVALID%s", colorGreen, colorReset)
	} else {
		overallTag = fmt.Sprintf("%sINVALID%s", colorRed, colorReset)
	}

	fmt.Printf("%s %s %s %s => %s\n", result.Email, formatTag, mxTag, smtpTag, overallTag)

	if verbose {
		if result.FormatError != "" {
			fmt.Printf("  [DEBUG] Format error: %s\n", result.FormatError)
		}
		if result.MXError != "" {
			fmt.Printf("  [DEBUG] MX error: %s\n", result.MXError)
		}
		if result.SMTPError != "" {
			fmt.Printf("  [DEBUG] SMTP error: %s\n", result.SMTPError)
		}
		if result.EmailExists {
			fmt.Printf("  [DEBUG] EmailExists: true\n")
		} else {
			fmt.Printf("  [DEBUG] EmailExists: false\n")
		}
	}
}

func getFormatStatus(result VerifyResult) string {
	if result.FormatValid {
		return "Valid"
	}
	return "Invalid"
}

func getMXStatus(result VerifyResult) string {
	if result.MXValid {
		return "Found"
	}
	return "Not found"
}

func getSMTPStatus(result VerifyResult) string {
	if result.SMTPValid {
		return "Passed"
	}
	return "Failed"
}

func printJSON(result VerifyResult, checkedCount int) {
	// Use a struct to ensure field order (checked_count will be first)
	type JSONOutput struct {
		Email        string `json:"email"`
		Format       string `json:"format"`
		MXRecords    string `json:"mx_records"`
		SMTPCheck    string `json:"smtp_check"`
		CheckedCount int    `json:"checked_count"`
	}

	output := JSONOutput{
		Email:        result.Email,
		Format:       getFormatStatus(result),
		MXRecords:    getMXStatus(result),
		SMTPCheck:    getSMTPStatus(result),
		CheckedCount: checkedCount,
	}

	jsonData, err := json.Marshal(output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		return
	}
	fmt.Println(string(jsonData))
}

func printCSV(result VerifyResult, checkedCount int, isFirst bool) {
	writer := csv.NewWriter(os.Stdout)
	defer writer.Flush()

	// Write header only for first record
	if isFirst {
		header := []string{"email", "format", "mx_records", "smtp_check", "checked_count"}
		if err := writer.Write(header); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing CSV header: %v\n", err)
			return
		}
	}

	record := []string{
		result.Email,
		getFormatStatus(result),
		getMXStatus(result),
		getSMTPStatus(result),
		fmt.Sprintf("%d", checkedCount),
	}
	if err := writer.Write(record); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing CSV: %v\n", err)
	}
}

// isValidFormat checks if the email address has a valid format
func isValidFormat(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

// lookupMXWithFallback tries to lookup MX records using multiple DNS servers
func lookupMXWithFallback(domain string) ([]*net.MX, error) {
	// List of public DNS servers to try as fallback
	dnsServers := []string{
		"8.8.8.8:53", // Google DNS
		"1.1.1.1:53", // Cloudflare DNS
		"8.8.4.4:53", // Google DNS secondary
		"1.0.0.1:53", // Cloudflare DNS secondary
	}

	// First, try system DNS
	mxRecords, err := net.LookupMX(domain)
	if err == nil && len(mxRecords) > 0 {
		return mxRecords, nil
	}

	// If system DNS fails, try public DNS servers
	for _, dnsServer := range dnsServers {
		var mxErr error
		mxRecords, mxErr = lookupMXWithCustomDNS(domain, dnsServer)
		if mxErr == nil && len(mxRecords) > 0 {
			return mxRecords, nil
		}
		err = mxErr // Update outer err for accurate error reporting
	}

	// If all DNS lookups failed, return the original error or last error
	if err != nil {
		return nil, fmt.Errorf("DNS lookup failed on all servers: %v", err)
	}

	return nil, fmt.Errorf("no MX records found on any DNS server")
}

// lookupMXWithCustomDNS performs MX lookup using a specific DNS server
func lookupMXWithCustomDNS(domain, dnsServer string) ([]*net.MX, error) {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: 5 * time.Second,
			}
			return d.DialContext(ctx, "udp", dnsServer)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mxRecords, err := resolver.LookupMX(ctx, domain)
	if err != nil {
		return nil, err
	}

	return mxRecords, nil
}

// lookupIPWithCustomDNS performs IP lookup using a specific DNS server
func lookupIPWithCustomDNS(domain, dnsServer string) ([]net.IP, error) {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: 5 * time.Second,
			}
			return d.DialContext(ctx, "udp", dnsServer)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ips, err := resolver.LookupIPAddr(ctx, domain)
	if err != nil {
		return nil, err
	}

	result := make([]net.IP, len(ips))
	for i, ip := range ips {
		result[i] = ip.IP
	}

	return result, nil
}

// lookupIPWithFallback tries to lookup IP addresses using multiple DNS servers
func lookupIPWithFallback(domain string) ([]net.IP, error) {
	// List of public DNS servers to try as fallback
	dnsServers := []string{
		"8.8.8.8:53", // Google DNS
		"1.1.1.1:53", // Cloudflare DNS
		"8.8.4.4:53", // Google DNS secondary
		"1.0.0.1:53", // Cloudflare DNS secondary
	}

	// First, try system DNS
	ips, err := net.LookupIP(domain)
	if err == nil && len(ips) > 0 {
		return ips, nil
	}

	// If system DNS fails, try public DNS servers
	for _, dnsServer := range dnsServers {
		var ipErr error
		ips, ipErr = lookupIPWithCustomDNS(domain, dnsServer)
		if ipErr == nil && len(ips) > 0 {
			return ips, nil
		}
		err = ipErr // Update outer err for accurate error reporting
	}

	// If all DNS lookups failed, return the original error or last error
	if err != nil {
		return nil, fmt.Errorf("DNS lookup failed on all servers: %v", err)
	}

	return nil, fmt.Errorf("no IP records found on any DNS server")
}

// hasMXRecord checks if the domain has MX (Mail Exchange) records
// Returns: (valid, mxRecords, errorMessage)
func hasMXRecord(email string) (bool, []*net.MX, string) {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false, nil, "invalid email format"
	}

	domain := parts[1]
	mxRecords, err := lookupMXWithFallback(domain)
	if err != nil {
		// Also check if domain has A records (some domains use A records instead of MX)
		ips, aErr := lookupIPWithFallback(domain)
		if aErr == nil && len(ips) > 0 {
			return true, nil, "domain exists (A record found, no MX records)"
		}
		return false, nil, fmt.Sprintf("domain not found or unreachable: %v", err)
	}

	if len(mxRecords) == 0 {
		// Check A records as fallback
		ips, aErr := lookupIPWithFallback(domain)
		if aErr == nil && len(ips) > 0 {
			return true, nil, "domain exists (A record found, no MX records)"
		}
		return false, nil, "no MX records found and domain does not resolve"
	}

	return true, mxRecords, ""
}

// getHostname returns a proper hostname for SMTP HELO command
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		// Fallback to a generic hostname
		return "mail.example.com"
	}

	// If hostname doesn't contain a dot, make it FQDN-like
	if !strings.Contains(hostname, ".") {
		return hostname + ".local"
	}

	return hostname
}

// isBlockedError checks if the SMTP error is due to IP/reputation blocking
func isBlockedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	blockedIndicators := []string{
		"blocked",
		"reputation",
		"proofpoint",
		"dynamic reputation",
		"client ",
		"service unavailable",
	}
	for _, indicator := range blockedIndicators {
		if strings.Contains(errStr, indicator) {
			return true
		}
	}
	return false
}

// isEmailNotFoundError checks if the SMTP error indicates the email doesn't exist
// Common codes: 550 (mailbox unavailable), 551 (user not local)
func isEmailNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Rejections about the sender/HELO/connection are NOT "user doesn't exist"
	senderIndicators := []string{
		"sender address rejected",
		"helo command rejected",
		"host not found",
		"nullmx",
		"domain does not accept",
	}
	for _, indicator := range senderIndicators {
		if strings.Contains(errStr, indicator) {
			return false
		}
	}

	// Check for common "user doesn't exist" error codes and messages
	notFoundIndicators := []string{
		"user unknown",
		"no such user",
		"user not found",
		"invalid recipient",
		"no mailbox here",
		"recipient address rejected",
		"undeliverable address",
	}

	for _, indicator := range notFoundIndicators {
		if strings.Contains(errStr, indicator) {
			return true
		}
	}

	return false
}

// smtpCheck attempts to verify the email by connecting to the mail server
// and checking if the email address is accepted (without actually sending)
// mxRecords: Pre-found MX records to use (to avoid duplicate lookups)
// Returns: (isValid, errorMessage, emailExists)
// emailExists: true if email exists, false if doesn't exist, unknown if can't determine
func smtpCheck(email string, mxRecords []*net.MX) (bool, string, bool) {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false, "invalid email format", false
	}

	if len(mxRecords) == 0 {
		return false, "no MX records provided for SMTP check", false
	}

	// Use the first MX record (highest priority)
	mxHost := strings.TrimSuffix(mxRecords[0].Host, ".")

	// Set a timeout for the connection
	timeout := 10 * time.Second

	// Connect to the SMTP server
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(mxHost, "25"), timeout)
	if err != nil {
		return false, fmt.Sprintf("connection failed: %v", err), false
	}
	defer conn.Close()

	// Set overall deadline for all SMTP operations to prevent indefinite hangs
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return false, fmt.Sprintf("failed to set connection deadline: %v", err), false
	}

	// Create SMTP client
	client, err := smtp.NewClient(conn, mxHost)
	if err != nil {
		return false, fmt.Sprintf("SMTP client creation failed: %v", err), false
	}
	defer client.Close()

	// Get proper hostname for HELO
	heloHostname := getHostname()

	// Say hello (EHLO) with proper hostname
	if err := client.Hello(heloHostname); err != nil {
		return false, fmt.Sprintf("HELO failed: %v", err), false
	}

	// Set a fake sender using the target email's own domain so it resolves and isn't rejected
	fakeSender := fmt.Sprintf("verify@%s", parts[1])
	if err := client.Mail(fakeSender); err != nil {
		return false, fmt.Sprintf("MAIL FROM failed: %v", err), false
	}

	// Check if the recipient is accepted
	err = client.Rcpt(email)
	if err != nil {
		// Check if server blocked us due to IP/reputation
		if isBlockedError(err) {
			return true, fmt.Sprintf("server blocked verification: %v", err), true
		}
		// Check if this is a "user doesn't exist" error
		if isEmailNotFoundError(err) {
			return false, fmt.Sprintf("email does not exist: %v", err), false
		}
		// Other errors (greylisting, etc.)
		return false, fmt.Sprintf("RCPT TO failed: %v (may be blocked by server)", err), false
	}

	// Check if this is a catch-all server by testing a random non-existent email
	// If the server accepts a random email, it's likely a catch-all
	if len(parts) == 2 {
		domain := parts[1]
		// Generate a random test email that definitely doesn't exist
		testEmail := fmt.Sprintf("nonexistent-test-%d@%s", time.Now().UnixNano(), domain)

		// Test if the random email is also accepted
		testErr := client.Rcpt(testEmail)
		if testErr == nil {
			// Server accepts random emails - it's a catch-all, cannot verify email existence
			return true, "server appears to accept all emails (catch-all)", true
		}
	}

	// RCPT TO succeeded and server is not catch-all - email exists
	return true, "", true
}

// VerifyResult holds the result of email verification
type VerifyResult struct {
	Email       string
	IsValid     bool
	FormatValid bool
	FormatError string
	MXValid     bool
	MXError     string
	SMTPValid   bool
	SMTPError   string
	EmailExists bool // true if confirmed exists, false if confirmed doesn't exist, unknown if can't determine
	IsCatchAll  bool // true if server is catch-all (accepts all emails)
	IsBlocked   bool // true if server blocked verification (IP/reputation)
	Issues      []string
}

// VerifyEmail performs comprehensive email verification
// An email is considered VALID only if ALL THREE checks pass:
// 1. Format validation
// 2. MX records exist
// 3. SMTP check confirms email exists
func VerifyEmail(email string) VerifyResult {
	result := VerifyResult{
		Email:   email,
		IsValid: false,
		Issues:  []string{},
	}

	// Step 1: Format validation
	result.FormatValid = isValidFormat(email)
	if !result.FormatValid {
		result.FormatError = "invalid email format"
		result.Issues = append(result.Issues, result.FormatError)
		return result
	}

	// Step 2: MX record check
	var mxRecords []*net.MX
	result.MXValid, mxRecords, result.MXError = hasMXRecord(email)
	if !result.MXValid {
		result.Issues = append(result.Issues, result.MXError)
		return result
	}

	// If we have MX records, use them for SMTP check
	// If domain only has A records (no MX), we need to fetch MX records now
	if len(mxRecords) == 0 {
		// Domain has A records but no MX, try to get MX records for SMTP check
		parts := strings.Split(email, "@")
		if len(parts) != 2 {
			result.Issues = append(result.Issues, "invalid email format")
			return result
		}

		domain := parts[1]
		var err error
		mxRecords, err = lookupMXWithFallback(domain)
		if err != nil || len(mxRecords) == 0 {
			result.Issues = append(result.Issues, "could not retrieve MX records for SMTP check")
			return result
		}
	}

	// Step 3: SMTP check (this is the most reliable way to verify email existence)
	result.SMTPValid, result.SMTPError, result.EmailExists = smtpCheck(email, mxRecords)

	// Handle catch-all: server accepts all emails, treat as valid
	if result.SMTPValid && strings.Contains(result.SMTPError, "catch-all") {
		result.IsCatchAll = true
		result.IsValid = true
		result.EmailExists = true
		return result
	}

	// Handle IP/reputation blocks: server blocked us, treat as valid
	if result.SMTPValid && strings.Contains(result.SMTPError, "blocked") {
		result.IsBlocked = true
		result.IsValid = true
		result.EmailExists = true
		return result
	}

	if !result.SMTPValid {
		// Check if we got a definitive "email doesn't exist" response
		if result.EmailExists == false && strings.Contains(strings.ToLower(result.SMTPError), "does not exist") {
			// Email confirmed to not exist
			result.IsValid = false
			result.Issues = append(result.Issues, result.SMTPError)
			return result
		}

		// SMTP check failed - email cannot be verified as valid
		result.IsValid = false
		result.Issues = append(result.Issues, fmt.Sprintf("SMTP check failed: %s", result.SMTPError))
		return result
	}

	// All three checks passed - email is VALID and confirmed to exist
	result.IsValid = true
	result.EmailExists = true
	return result
}

func main() {
	// Parse command-line flags
	var outputJSON, outputCSV, silent, version, valid, noColor bool
	var concurrent int
	var outputFile string
	pflag.BoolVar(&outputJSON, "json", false, "Output results in JSON format")
	pflag.BoolVar(&outputCSV, "csv", false, "Output results in CSV format")
	pflag.BoolVar(&silent, "silent", false, "Silent mode.")
	pflag.BoolVar(&version, "version", false, "Print the version of the tool and exit.")
	pflag.BoolVar(&verbose, "verbose", false, "Show detailed error messages for each check.")
	pflag.IntVar(&concurrent, "concurrent", 10, "Number of concurrent email checks")
	pflag.BoolVar(&valid, "valid", false, "Print only valid emails")
	pflag.BoolVar(&noColor, "nc", false, "Disable color output")
	pflag.StringVar(&outputFile, "output", "", "Append only valid emails to the specified file")
	pflag.Parse()

	if noColor {
		disableColors()
	}

	if !silent {
		banner.PrintBanner()
	}

	// Print version and exit if -version flag is provided
	if version {
		banner.PrintBanner()
		banner.PrintVersion()
		return
	}

	// Read all emails from stdin first
	scanner := bufio.NewScanner(os.Stdin)
	var emails []string
	for scanner.Scan() {
		email := strings.TrimSpace(scanner.Text())
		if email == "" {
			continue
		}
		emails = append(emails, email)
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}

	// If no emails were processed, show usage
	if len(emails) == 0 {
		fmt.Println("Error: No email addresses provided")
		os.Exit(1)
	}

	// Open output file for appending if --output is set
	var outFile *os.File
	if outputFile != "" {
		var err error
		outFile, err = os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening output file: %v\n", err)
			os.Exit(1)
		}
		defer outFile.Close()
	}

	// Print CSV header before any concurrent output
	if outputCSV {
		stdoutMu.Lock()
		writer := csv.NewWriter(os.Stdout)
		header := []string{"email", "format", "mx_records", "smtp_check", "checked_count"}
		writer.Write(header)
		writer.Flush()
		stdoutMu.Unlock()
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrent)

	for _, email := range emails {
		wg.Add(1)
		sem <- struct{}{} // acquire semaphore

		go func(e string) {
			defer wg.Done()
			defer func() { <-sem }() // release semaphore

			result := VerifyEmail(e)

			// Calculate checked_count
			checkedCount := 0
			if result.FormatValid {
				checkedCount++
				if result.MXValid {
					checkedCount++
					if result.SMTPValid {
						checkedCount++
					}
				}
			}

			// Skip if --valid is set and email is not valid
			if valid && !result.IsValid {
				return
			}

			// Append valid email to output file if --output is set
			if outFile != nil && result.IsValid {
				fileMu.Lock()
				fmt.Fprintf(outFile, "%s\n", result.Email)
				fileMu.Unlock()
			}

			printResult(result, outputJSON, outputCSV, checkedCount, false)
		}(email)
	}

	wg.Wait()

	// Always exit with 0 to prevent shell from showing exit status
	os.Exit(0)
}
