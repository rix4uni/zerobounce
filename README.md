## zerobounce

A Go CLI tool to verify email addresses through format validation, MX record checks, and SMTP verification.

## Features
- **Format Validation**: Validates email syntax according to RFC standards
- **MX Record Check**: Verifies domain has Mail Exchange records configured
- **SMTP Verification**: Connects to mail servers to verify email address existence
- **DNS Fallback**: Automatically uses public DNS (Google, Cloudflare) if system DNS fails
- **Catch-All Detection**: Identifies and flags catch-all email servers
- **Multiple Output Formats**: Human-readable, JSON, and CSV output
- **Bulk Processing**: Read emails from file or pipe for batch verification

## Installation

**Using Go:**
```console
go install github.com/rix4uni/zerobounce@latest
```

**Pre-built Binaries:**
```console
wget https://github.com/rix4uni/zerobounce/releases/download/v0.0.2/zerobounce-linux-amd64-0.0.2.tgz
tar -xvzf zerobounce-linux-amd64-0.0.2.tgz
mv zerobounce ~/go/bin/
```

**From Source:**
```console
git clone --depth 1 https://github.com/rix4uni/zerobounce.git
cd zerobounce; go install
```

## Examples
```console
Usage of zerobounce:
      --concurrent int   Number of concurrent email checks (default 10)
      --csv              Output results in CSV format
      --json             Output results in JSON format
      --silent           Silent mode.
      --valid            Print only valid emails.
      --verbose          Show detailed error messages for each check.
      --version          Print the version of the tool and exit.
```

## Usage Examples
Single Email:
```console
echo "user@example.com" | zerobounce
```

Multiple Emails from File:
```console
cat emails.txt | zerobounce
```

## Output
rix4uni@krazeplanet.com [FORMAT:VALID] [MX:FOUND] [SMTP:FAILED] => INVALID
support@hackerone.com [FORMAT:VALID] [MX:FOUND] [SMTP:CATCHALL] => VALID
contact@krazeplanet.com [FORMAT:VALID] [MX:FOUND] [SMTP:PASSED] => VALID
```

JSON Output:
```console
cat emails.txt | zerobounce --json

## Output
{"email":"rix4uni@krazeplanet.com","format":"Valid","mx_records":"Found","smtp_check":"Failed","checked_count":2}
{"email":"support@hackerone.com","format":"Valid","mx_records":"Found","smtp_check":"Passed","checked_count":3}
{"email":"contact@krazeplanet.com","format":"Valid","mx_records":"Found","smtp_check":"Passed","checked_count":3}
```

CSV Output:
```console
cat emails.txt | zerobounce --csv

## Output
email,format,mx_records,smtp_check,checked_count
rix4uni@krazeplanet.com,Valid,Found,Failed,2
support@hackerone.com,Valid,Found,Passed,3
contact@krazeplanet.com,Valid,Found,Passed,3
```
