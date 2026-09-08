package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// N5: a row that cannot be read used to be skipped, and a query that failed used
// to be ignored, so a list rendered as complete when it was not — an invoice
// missing its payments, a consultation missing a diagnosis, a superbill missing
// a line item. Both patterns were removed across the package; this keeps them
// out, because the failure they produce is silent and would not be noticed.

var (
	droppedRow     = regexp.MustCompile(`\.Scan\([^)]*\) == nil`)
	discardedQuery = regexp.MustCompile(`, _ :?= (?:s\.db|tx)\.Query`)
)

func TestNoRowIsDroppedAndNoQueryErrorIsDiscarded(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatal(err)
		}
		for index, line := range strings.Split(string(source), "\n") {
			if droppedRow.MatchString(line) {
				t.Errorf("%s:%d silently drops a row that could not be read:\n\t%s\n\tuse `if err := rows.Scan(…); err != nil` and report it",
					name, index+1, strings.TrimSpace(line))
			}
			if discardedQuery.MatchString(line) {
				t.Errorf("%s:%d discards a query error, so a failed query renders as an empty list:\n\t%s",
					name, index+1, strings.TrimSpace(line))
			}
		}
	}
}
