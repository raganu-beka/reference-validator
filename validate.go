package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
)

const userAgent = "reference-validator/1.0 (mailto:user@example.com)"

var httpClient = &http.Client{Timeout: 10 * time.Second}

func validateDOI(doi string, ref Reference) (idFound, titleMatch, authorMatch bool, err error) {
	url := "https://api.crossref.org/works/" + doi
	body, err := httpGet(url)
	if err != nil {
		return false, false, false, fmt.Errorf("crossref lookup failed: %w", err)
	}

	var resp struct {
		Message struct {
			Title  []string `json:"title"`
			Author []struct {
				Family string `json:"family"`
				Given  string `json:"given"`
			} `json:"author"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return true, false, false, fmt.Errorf("decode crossref response: %w", err)
	}

	apiTitle := ""
	if len(resp.Message.Title) > 0 {
		apiTitle = resp.Message.Title[0]
	}
	apiAuthors := make([]string, 0, len(resp.Message.Author))
	for _, a := range resp.Message.Author {
		apiAuthors = append(apiAuthors, a.Family)
	}

	return true, titlesMatch(ref.Title, apiTitle), authorsMatch(ref.Authors, apiAuthors), nil
}

func validateISBN(isbn string, ref Reference) (idFound, titleMatch, authorMatch bool, err error) {
	clean := stripISBN(isbn)
	url := "https://openlibrary.org/isbn/" + clean + ".json"
	body, err := httpGet(url)
	if err != nil {
		return false, false, false, fmt.Errorf("openlibrary lookup failed: %w", err)
	}

	var resp struct {
		Title   string `json:"title"`
		Authors []struct {
			Key string `json:"key"`
		} `json:"authors"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return true, false, false, fmt.Errorf("decode openlibrary response: %w", err)
	}

	apiAuthors := make([]string, 0, len(resp.Authors))
	for _, a := range resp.Authors {
		time.Sleep(500 * time.Millisecond)
		name, aerr := fetchOpenLibraryAuthor(a.Key)
		if aerr == nil && name != "" {
			apiAuthors = append(apiAuthors, name)
		}
	}

	return true, titlesMatch(ref.Title, resp.Title), authorsMatch(ref.Authors, apiAuthors), nil
}

func fetchOpenLibraryAuthor(key string) (string, error) {
	body, err := httpGet("https://openlibrary.org" + key + ".json")
	if err != nil {
		return "", err
	}
	var resp struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	return resp.Name, nil
}

func httpGet(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("not found (404)")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return body, nil
}

func stripISBN(isbn string) string {
	var b strings.Builder
	for _, r := range isbn {
		if unicode.IsDigit(r) || r == 'X' || r == 'x' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normalize(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevSpace = false
		case unicode.IsSpace(r):
			if !prevSpace && b.Len() > 0 {
				b.WriteRune(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func titlesMatch(a, b string) bool {
	na, nb := normalize(a), normalize(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}

	return strings.Contains(na, nb) || strings.Contains(nb, na)
}

func authorsMatch(parsed, api []string) bool {
	if len(parsed) == 0 || len(api) == 0 {
		return false
	}
	apiNorm := make([]string, 0, len(api))
	for _, a := range api {
		apiNorm = append(apiNorm, normalize(a))
	}
	for _, p := range parsed {
		last := lastName(p)
		if last == "" {
			continue
		}
		for _, a := range apiNorm {
			if strings.Contains(a, last) {
				return true
			}
		}
	}
	return false
}

func lastName(author string) string {
	a := strings.TrimSpace(author)
	if a == "" {
		return ""
	}
	if i := strings.Index(a, ","); i >= 0 {
		return normalize(a[:i])
	}
	parts := strings.Fields(a)
	if len(parts) == 0 {
		return ""
	}
	return normalize(parts[len(parts)-1])
}
