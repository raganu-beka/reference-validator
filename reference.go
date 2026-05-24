package main

type Reference struct {
	Raw     string   `json:"raw"`
	Authors []string `json:"authors"`
	Title   string   `json:"title"`
	Year    string   `json:"year"`
	DOI     string   `json:"doi"`
	ISBN    string   `json:"isbn"`
	Type    string   `json:"type"`
}

type ValidationResult struct {
	Ref         Reference `json:"reference"`
	ParseOK     bool      `json:"parse_ok"`
	IDFound     bool      `json:"id_found"`
	TitleMatch  bool      `json:"title_match"`
	AuthorMatch bool      `json:"author_match"`
	Warnings    []string  `json:"warnings"`
	Errors      []string  `json:"errors"`
}
