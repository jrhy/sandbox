package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jessevdk/go-flags"
)

// Constants for Plausibility and Formatting
// Max plausible epoch time in seconds (Jan 1, 2049 UTC). Used to filter out large IDs.
const maxEpochSeconds = 2500000000

// Min epoch time (Jan 1, 1970 UTC). All valid timestamps are greater than this.
const minEpochSeconds = 0

// Regex to find a number with at least 10 digits (seconds since 2001),
// optionally followed by a decimal part.
const epochRegex = `(\d{10,}(\.\d+)?)`

// Custom ISO 8601 format for UTC: YYYY-MM-DDTHH:MM:SS.mmmZ
const outputLayout = "2006-01-02T15:04:05.999Z"

func init() {
	funcs["epoch2ts"] = subcommand{
		``,
		"converts epoch timestamps from stdin into ISO-8601 times",
		func(a []string) int {
			o := struct{}{}
			p := flags.NewParser(&o, 0)
			ra, err := p.ParseArgs(a)
			if err != nil {
				if strings.Contains(err.Error(), "unknown flag") {
					return exitSubcommandUsage
				}
				die(fmt.Sprintf("parse: %v", err))
			}
			if len(ra) > 0 {
				return exitSubcommandUsage
			}

			// Compile the regular expression once
			re := regexp.MustCompile(epochRegex)

			// Create a new scanner to read from standard input (os.Stdin)
			scanner := bufio.NewScanner(os.Stdin)

			// Process input line by line
			for scanner.Scan() {
				line := scanner.Text()

				// Find all matches of the epoch time pattern in the current line
				matches := re.FindAllStringSubmatchIndex(line, -1)

				// Build the modified line by replacing timestamps inline
				modifiedLine := line
				offset := 0 // Track how much the line length changes as we make replacements

				// Process matches in forward order
				for _, match := range matches {
					// Extract the full matched number string
					startIndex := match[2] + offset
					endIndex := match[3] + offset
					epochStr := line[match[2]:match[3]]

					// Convert the matched string to a float64
					epochFloat, err := strconv.ParseFloat(epochStr, 64)
					if err != nil {
						continue
					}

					// Separate seconds (integer part) and nanoseconds (fractional part)
					seconds := int64(epochFloat)

					// --- PLAUSIBILITY CHECK ---
					if seconds > maxEpochSeconds || seconds < minEpochSeconds {
						continue
					}

					// Calculate nanoseconds from the fractional part
					nanoseconds := int64((epochFloat - float64(seconds)) * 1e9)

					// Convert the valid epoch time to a time.Time struct in UTC
					t := time.Unix(seconds, nanoseconds).In(time.UTC)

					// Format the time to the desired ISO 8601 string
					isoTime := t.Format(outputLayout)

					// Create the inline replacement string
					replacement := fmt.Sprintf("%s--->%s", epochStr, isoTime)

					// Replace the epoch timestamp inline
					modifiedLine = modifiedLine[:startIndex] + replacement + modifiedLine[endIndex:]

					// Update offset for subsequent replacements
					offset += len(replacement) - len(epochStr)
				}

				// Print the modified line
				fmt.Println(modifiedLine)
			}

			if err := scanner.Err(); err != nil && err != io.EOF {
				fmt.Fprintf(os.Stderr, "Error reading standard input: %v\n", err)
				os.Exit(1)
			}

			return 0
		},
	}
}
