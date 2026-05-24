package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func main() {
	var (
		filePath string
		model    string
		asJSON   bool
		strict   bool
	)
	flag.StringVar(&filePath, "file", "", "read references from file instead of stdin")
	flag.StringVar(&model, "model", "", "Claude model to use for parsing (default: claude CLI default)")
	flag.BoolVar(&asJSON, "json", false, "output JSON")
	flag.BoolVar(&strict, "strict", false, "treat warnings as failures (affects exit code)")
	flag.Parse()

	if _, err := exec.LookPath("claude"); err != nil {
		fmt.Fprintln(os.Stderr, "reference-validator requires the Claude CLI. Install it with: npm install -g @anthropic-ai/claude-code")
		os.Exit(2)
	}

	var src io.Reader = os.Stdin
	if filePath != "" {
		f, err := os.Open(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open %s: %v\n", filePath, err)
			os.Exit(2)
		}
		defer f.Close()
		src = f
	}

	raw, err := io.ReadAll(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read input: %v\n", err)
		os.Exit(2)
	}

	refs := splitReferences(string(raw))
	if len(refs) == 0 {
		fmt.Fprintln(os.Stderr, "no references found in input")
		os.Exit(1)
	}

	results := make([]ValidationResult, 0, len(refs))
	for i, r := range refs {
		results = append(results, processOne(r, model))
		if i < len(refs)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	if err := writeReport(os.Stdout, results, asJSON, strict); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(2)
	}

	// Exit nonzero if anything failed (or warned in strict mode).
	for _, r := range results {
		if classify(r, strict) == statusFail {
			os.Exit(1)
		}
	}
}

func processOne(raw, model string) ValidationResult {
	res := ValidationResult{
		Ref:      Reference{Raw: raw},
		Warnings: []string{},
		Errors:   []string{},
	}

	ref, err := parseReference(raw, model)
	if err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("parse failed: %v", err))
		return res
	}
	res.Ref = ref
	res.ParseOK = true

	switch {
	case ref.DOI != "":
		idFound, tm, am, vErr := validateDOI(ref.DOI, ref)
		if vErr != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Could not reach validation API: %v", vErr))
			return res
		}
		res.IDFound, res.TitleMatch, res.AuthorMatch = idFound, tm, am
	case ref.ISBN != "":
		idFound, tm, am, vErr := validateISBN(ref.ISBN, ref)
		if vErr != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Could not reach validation API: %v", vErr))
			return res
		}
		res.IDFound, res.TitleMatch, res.AuthorMatch = idFound, tm, am
	default:
		// No identifier — handled in report.
	}
	return res
}

// numberedPrefix matches list markers like "1.", "1)", "[1]", "(1)" at the
// start of a (trimmed) line.
var numberedPrefix = regexp.MustCompile(`^(?:\[\d+\]|\(\d+\)|\d+[.)])\s+`)

// splitReferences accepts the entire input and returns each reference as a
// single trimmed string. It splits on blank lines, and additionally on lines
// that begin with a numbered list marker — even when no blank line separates
// them.
func splitReferences(input string) []string {
	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var refs []string
	var cur strings.Builder

	flush := func() {
		s := strings.TrimSpace(cur.String())
		if s != "" {
			refs = append(refs, s)
		}
		cur.Reset()
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			flush()
			continue
		}

		// A numbered marker starts a new reference (unless current is empty).
		if numberedPrefix.MatchString(trimmed) && cur.Len() > 0 {
			flush()
		}

		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		// Strip the leading numbering for cleanliness.
		cur.WriteString(numberedPrefix.ReplaceAllString(trimmed, ""))
	}
	flush()
	return refs
}
