package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorDim    = "\033[2m"
)

type status int

const (
	statusOK status = iota
	statusWarn
	statusFail
)

func classify(r ValidationResult, strict bool) status {
	if len(r.Errors) > 0 {
		return statusFail
	}
	if !r.ParseOK {
		return statusFail
	}
	isURL := r.Ref.DOI == "" && r.Ref.ISBN == "" && r.Ref.URL != ""
	if !r.IDFound {
		if r.Ref.DOI == "" && r.Ref.ISBN == "" && r.Ref.URL == "" {
			return statusFail
		}
		if isURL {
			// Broken link is a real failure, not a soft warning.
			return statusFail
		}
		return statusWarn
	}
	if isURL {
		if (!r.TitleMatch && r.Ref.Title != "") || len(r.Warnings) > 0 {
			if strict {
				return statusFail
			}
			return statusWarn
		}
		return statusOK
	}
	if !r.TitleMatch || !r.AuthorMatch || len(r.Warnings) > 0 {
		if strict {
			return statusFail
		}
		return statusWarn
	}
	return statusOK
}

func writeJSON(w io.Writer, results []ValidationResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

func writeResult(w io.Writer, r ValidationResult, i int, numbered, strict, useColor bool) {
	s := classify(r, strict)
	symbol, color := symbolFor(s)
	reset := colorReset
	if !useColor {
		color = ""
		reset = ""
	}

	header := formatHeader(r.Ref)
	if numbered {
		fmt.Fprintf(w, "%s[%d] %s%s  %s\n", color, i+1, symbol, reset, header)
	} else {
		fmt.Fprintf(w, "%s%s%s  %s\n", color, symbol, reset, header)
	}

	for _, line := range detailLines(r) {
		fmt.Fprintf(w, "        %s\n", line)
	}
	fmt.Fprintln(w)
}

func writeSummary(w io.Writer, results []ValidationResult, strict bool) {
	var ok, warn, fail int
	for _, r := range results {
		switch classify(r, strict) {
		case statusOK:
			ok++
		case statusWarn:
			warn++
		case statusFail:
			fail++
		}
	}
	fmt.Fprintf(w, "%d checked — %d valid, %d warning, %d failed\n", len(results), ok, warn, fail)
}

func symbolFor(s status) (string, string) {
	switch s {
	case statusOK:
		return "✓", colorGreen
	case statusWarn:
		return "⚠", colorYellow
	default:
		return "✗", colorRed
	}
}

func formatHeader(r Reference) string {
	author := "Unknown"
	if len(r.Authors) > 0 {
		first := r.Authors[0]
		if comma := strings.Index(first, ","); comma >= 0 {
			first = strings.TrimSpace(first[:comma])
		}
		if len(r.Authors) > 1 {
			author = first + " et al."
		} else {
			author = first
		}
	}
	year := r.Year
	if year == "" {
		year = "n.d."
	}
	title := r.Title
	if title == "" {
		title = "???"
	}
	return fmt.Sprintf("%s (%s) — %q", author, year, title)
}

func detailLines(r ValidationResult) []string {
	var lines []string

	for _, e := range r.Errors {
		lines = append(lines, "error: "+e)
	}

	switch {
	case r.Ref.DOI != "":
		switch {
		case !r.IDFound:
			lines = append(lines, fmt.Sprintf("DOI %s → not found in Crossref", r.Ref.DOI))
		case !r.TitleMatch && !r.AuthorMatch:
			lines = append(lines, fmt.Sprintf("DOI %s → found, but title and author mismatch", r.Ref.DOI))
		case !r.TitleMatch:
			lines = append(lines, fmt.Sprintf("DOI %s → found, but title mismatch", r.Ref.DOI))
		case !r.AuthorMatch:
			lines = append(lines, fmt.Sprintf("DOI %s → found, but author name mismatch", r.Ref.DOI))
		default:
			lines = append(lines, fmt.Sprintf("DOI %s → confirmed via Crossref", r.Ref.DOI))
		}
	case r.Ref.ISBN != "":
		switch {
		case !r.IDFound:
			lines = append(lines, fmt.Sprintf("ISBN %s → not found in Open Library", r.Ref.ISBN))
		case !r.TitleMatch && !r.AuthorMatch:
			lines = append(lines, fmt.Sprintf("ISBN %s → found, but title and author mismatch", r.Ref.ISBN))
		case !r.TitleMatch:
			lines = append(lines, fmt.Sprintf("ISBN %s → found, but title mismatch", r.Ref.ISBN))
		case !r.AuthorMatch:
			lines = append(lines, fmt.Sprintf("ISBN %s → found, but author name mismatch", r.Ref.ISBN))
		default:
			lines = append(lines, fmt.Sprintf("ISBN %s → confirmed via Open Library", r.Ref.ISBN))
		}
	case r.Ref.URL != "":
		switch {
		case !r.IDFound:
			lines = append(lines, fmt.Sprintf("URL %s → unreachable", r.Ref.URL))
		case r.Ref.Title == "":
			lines = append(lines, fmt.Sprintf("URL %s → reachable (no title to compare)", r.Ref.URL))
		case !r.TitleMatch:
			lines = append(lines, fmt.Sprintf("URL %s → reachable, but page title mismatch", r.Ref.URL))
		default:
			lines = append(lines, fmt.Sprintf("URL %s → reachable and title matches", r.Ref.URL))
		}
	default:
		lines = append(lines, "No DOI, ISBN, or URL found. Could not validate.")
	}

	for _, wmsg := range r.Warnings {
		lines = append(lines, "warning: "+wmsg)
	}
	return lines
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
