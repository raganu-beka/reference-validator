# reference-validator

Validates academic references (articles, books, websites) by parsing them with Claude and cross-checking the extracted identifiers against public APIs.

## Requirements

- Go 1.21+
- [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) (`npm install -g @anthropic-ai/claude-code`)

## Build

```sh
go build -o reference-validator .
```

## Usage

```
reference-validator [flags]
```

| Flag | Default | Description |
|---|---|---|
| `-file <path>` | stdin | Read references from a file instead of stdin |
| `-model <name>` | Claude default | Claude model to use for parsing |
| `-json` | false | Output results as JSON |
| `-strict` | false | Treat warnings as failures (affects exit code) |
| `-concurrency <n>` | 5 | Max references processed in parallel; `-1` = all at once |

Exit code is `0` if all references pass, `1` if any fail, `2` for fatal errors.

---

## Examples

### Pipe a list of references

```sh
cat refs.txt | ./reference-validator
```

### Read from a file

```sh
./reference-validator -file refs.txt
```

### JSON output (useful for scripting)

```sh
./reference-validator -file refs.txt -json | jq '.[].reference.doi'
```

### Strict mode — warnings become failures

```sh
./reference-validator -file refs.txt -strict
echo $?   # 1 if any reference has mismatched title/author
```

### Run all references in parallel

```sh
./reference-validator -file refs.txt -concurrency -1
```

### Use a specific Claude model

```sh
./reference-validator -file refs.txt -model claude-opus-4-7
```

---

## Input format

Feed one or more references via stdin or a file. Supported layouts:

**Numbered list** — `[1]`, `(1)`, `1.`, or `1)` prefixes:

```
[1] Smith, J. (2021). A great paper. Nature, 12(3). https://doi.org/10.1038/s123
[2] Doe, J., & Lee, A. (2019). Another study. Journal of Science. doi:10.5678/js.2019
```

**Unnumbered citation-style** (starts with an author name):

```
Smith, J. (2021). A great paper. Nature, 12(3). https://doi.org/10.1038/s123

Knuth, D. E. (1997). The Art of Computer Programming. ISBN 978-0-201-89683-1
```

**Mixed or multi-line** — blank lines always separate references.

---

## How validation works

Each reference goes through three stages: **split → parse → validate**.

### 1. Split

[`splitReferences`](main.go#L156) scans input line by line and splits it into individual reference strings. A new reference starts when:

- A numbered prefix (`[1]`, `1.`, `(1)`, `1)`) is detected, or
- A line looks like the beginning of an author citation (uppercase last name followed by comma or period), or
- A blank line appears.

Multi-line references are joined into a single string before processing.

### 2. Parse (Claude)

[`parseReference`](parse.go#L16) sends the raw reference text to `claude -p` with a strict system prompt asking for JSON only. Claude extracts:

- `authors` — normalized as `["Last, F."]`
- `title`
- `doi` — normalized to bare identifier (strips `https://doi.org/`)
- `isbn`
- `url` — only non-DOI HTTP(S) URLs
- `year`
- `type` — `article | book | website | unknown`

The response is scanned for the first valid JSON object (stray prose or markdown fences are tolerated).

### 3. Validate (external APIs)

[`processOne`](main.go#L107) routes to one of three validators based on what identifier was extracted, in priority order:

| Identifier | API | Checks |
|---|---|---|
| DOI | [Crossref](https://api.crossref.org) | ID exists, title substring match, author last-name match |
| ISBN | [Open Library](https://openlibrary.org) | ID exists, title match, author name match (fetches author records individually) |
| URL | HTTP GET | Reachable (HTTP 2xx), HTML `<title>` tag substring match |
| None | — | Always fails |

**Title matching** ([`titlesMatch`](validate.go#L168)): both strings are normalized (lowercased, punctuation stripped, whitespace collapsed) and the check passes if one is a substring of the other — this tolerates subtitles and minor formatting differences.

**Author matching** ([`authorsMatch`](validate.go#L180)): each parsed author's last name is checked against each API-returned name. If any one parsed author matches any one API author, the check passes (useful for "et al." lists where only the first author appears in the reference).

### 4. Classify and report

[`classify`](report.go#L27) maps a `ValidationResult` to one of three statuses:

| Status | Meaning |
|---|---|
| `✓` OK | ID found, title and author match (or URL reachable with matching title) |
| `⚠` Warn | ID found but title or author mismatch; or API unreachable |
| `✗` Fail | Parse error, no identifier, 404 from API, or broken URL |

With `-strict`, warnings are promoted to failures. The process exits `1` if any result is `statusFail`.

---

## JSON output schema

```json
[
  {
    "reference": {
      "raw": "...",
      "authors": ["Last, F."],
      "title": "...",
      "year": "2021",
      "doi": "10.1000/xyz",
      "isbn": "",
      "url": "",
      "type": "article"
    },
    "parse_ok": true,
    "id_found": true,
    "title_match": true,
    "author_match": true,
    "warnings": [],
    "errors": []
  }
]
```
