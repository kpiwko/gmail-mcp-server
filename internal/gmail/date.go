package gmail

import (
	"fmt"
	"regexp"
	"strconv"
)

// quarterPattern matches "Q1 2026", "q3 2025", etc. — case-insensitive, whole-word.
var quarterPattern = regexp.MustCompile(`(?i)\bQ([1-4])\s+(\d{4})\b`)

// quarterStartMonth returns the first month (1-based) of the given quarter.
func quarterStartMonth(quarter int) int {
	return (quarter-1)*3 + 1
}

// parseQuarterQuery replaces quarter references in a Gmail search query with
// explicit date operators understood by the Gmail API.
//
// Examples:
//
//	"Q1 2026"          → "after:2026/01/01 before:2026/04/01"
//	"Q4 2025"          → "after:2025/10/01 before:2026/01/01"
//	"from:me Q2 2026"  → "from:me after:2026/04/01 before:2026/07/01"
func parseQuarterQuery(query string) string {
	return quarterPattern.ReplaceAllStringFunc(query, func(match string) string {
		sub := quarterPattern.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}

		quarter, err := strconv.Atoi(sub[1])
		if err != nil {
			return match
		}
		year, err := strconv.Atoi(sub[2])
		if err != nil {
			return match
		}

		startMonth := quarterStartMonth(quarter)
		// The end bound is the first day of the *next* quarter (exclusive).
		endMonth := startMonth + 3
		endYear := year
		if endMonth > 12 {
			endMonth = 1
			endYear = year + 1
		}

		return fmt.Sprintf("after:%d/%02d/01 before:%d/%02d/01",
			year, startMonth,
			endYear, endMonth,
		)
	})
}
