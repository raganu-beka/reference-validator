package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const parserSystemPrompt = `You are a citation parser. Extract fields from the reference below. Return ONLY valid JSON, no explanation, no markdown. Schema: {"authors": ["Last, F."], "title": "", "doi": "", "isbn": "", "url": "", "year": "", "type": "article|book|website|unknown"}. If a field is not present, use an empty string or empty array. Normalize DOIs to just the identifier e.g. 10.1000/xyz, strip https://doi.org/. For url, extract any http(s) URL present in the reference that is not a doi.org link; leave empty if the reference has a DOI or ISBN as its primary identifier.`

// parseReference invokes `claude -p` to extract structured fields from a raw
// reference string. The raw text is sent on stdin to the subprocess.
func parseReference(raw, model string) (Reference, error) {
	ref := Reference{Raw: raw}

	args := []string{"-p", "--system-prompt", parserSystemPrompt}
	if model != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.Command("claude", args...)
	cmd.Stdin = strings.NewReader(raw)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var execErr *exec.Error
		if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
			return ref, fmt.Errorf("claude CLI not found: %w", err)
		}
		return ref, fmt.Errorf("claude -p failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	out := strings.TrimSpace(stdout.String())
	jsonStr := extractJSON(out)
	if jsonStr == "" {
		return ref, fmt.Errorf("no JSON found in claude output: %q", out)
	}

	var parsed struct {
		Authors []string `json:"authors"`
		Title   string   `json:"title"`
		DOI     string   `json:"doi"`
		ISBN    string   `json:"isbn"`
		URL     string   `json:"url"`
		Year    string   `json:"year"`
		Type    string   `json:"type"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return ref, fmt.Errorf("malformed JSON from claude: %w (got: %q)", err, jsonStr)
	}

	ref.Authors = parsed.Authors
	ref.Title = parsed.Title
	ref.DOI = normalizeDOI(parsed.DOI)
	ref.ISBN = parsed.ISBN
	ref.URL = strings.TrimSpace(parsed.URL)
	ref.Year = parsed.Year
	ref.Type = parsed.Type
	if ref.Type == "" {
		ref.Type = "unknown"
	}
	return ref, nil
}

// extractJSON finds the first balanced {...} object in the output, tolerating
// stray prose or code fences.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func normalizeDOI(doi string) string {
	d := strings.TrimSpace(doi)
	d = strings.TrimPrefix(d, "https://doi.org/")
	d = strings.TrimPrefix(d, "http://doi.org/")
	d = strings.TrimPrefix(d, "doi:")
	return strings.TrimSpace(d)
}
