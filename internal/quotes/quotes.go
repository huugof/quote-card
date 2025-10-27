package quotes

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gosimple/slug"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"gopkg.in/yaml.v3"
)

var markdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(html.WithUnsafe()),
)

type Quote struct {
	ID            string
	Quote         string
	Name          string
	URL           string
	NormalizedURL string
	ArticleTitle  string
	SourceDomain  string
	ArticleSlug   string
	CreatedAt     *time.Time
	Tags          []string
	BodyHTML      string
	Location      string
}

type LoadResult struct {
	Quotes   []*Quote
	Warnings []string
	Errors   []string
}

type frontMatter struct {
	ID           string   `yaml:"id"`
	Quote        string   `yaml:"quote"`
	Name         string   `yaml:"name"`
	URL          string   `yaml:"url"`
	ArticleTitle string   `yaml:"article_title"`
	SourceDomain string   `yaml:"source_domain"`
	CreatedAt    string   `yaml:"created_at"`
	Tags         []string `yaml:"tags"`
}

func Load(dir string) (*LoadResult, error) {
	entries := []string{}
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			rel, relErr := filepath.Rel(dir, path)
			if relErr != nil {
				return relErr
			}
			entries = append(entries, rel)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	sort.Strings(entries)

	idSet := map[string]struct{}{}
	urlSet := map[string][]string{}
	warnings := []string{}
	errors := []string{}
	quotesOut := []*Quote{}

	for _, rel := range entries {
		absPath := filepath.Join(dir, rel)
		rawBytes, err := os.ReadFile(absPath)
		if err != nil {
			return nil, err
		}

		fm, body, err := parseFrontMatter(string(rawBytes))
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", filepath.ToSlash(rel), err))
			continue
		}

		location := filepath.ToSlash(rel)

		if fm.ID == "" {
			errors = append(errors, fmt.Sprintf("%s: missing required field \"id\".", location))
		} else {
			if _, exists := idSet[fm.ID]; exists {
				errors = append(errors, fmt.Sprintf("%s: duplicate id \"%s\".", location, fm.ID))
			} else {
				idSet[fm.ID] = struct{}{}
			}
		}

		if fm.Quote == "" {
			errors = append(errors, fmt.Sprintf("%s: missing required field \"quote\".", location))
		}
		if fm.Name == "" {
			errors = append(errors, fmt.Sprintf("%s: missing required field \"name\".", location))
		}
		if fm.URL == "" {
			errors = append(errors, fmt.Sprintf("%s: missing required field \"url\".", location))
		}

	normalizedURL, _, normErr := normalizeURL(fm.URL)
	if normErr != nil {
		errors = append(errors, fmt.Sprintf("%s: invalid url \"%s\".", location, fm.URL))
	}

		createdAt := parseTime(fm.CreatedAt)

		domain, slugValue := inferDomainAndSlug(normalizedURL)
		if fm.SourceDomain != "" {
			domain = fm.SourceDomain
		}

		if domain == "" {
			warnings = append(warnings, fmt.Sprintf("%s: could not determine source domain.", location))
		}
		if slugValue == "" {
			warnings = append(warnings, fmt.Sprintf("%s: could not determine article slug.", location))
		}

	if normalizedURL != "" {
		bucket := urlSet[normalizedURL]
		idKey := fm.ID
			if idKey == "" {
				idKey = location
			}
			urlSet[normalizedURL] = append(bucket, idKey)
		}

		bodyHTML := ""
		if strings.TrimSpace(body) != "" {
			var buf bytes.Buffer
			if err := markdown.Convert([]byte(body), &buf); err != nil {
				return nil, fmt.Errorf("render markdown for %s: %w", location, err)
			}
			bodyHTML = buf.String()
		}

		quoteObj := &Quote{
			ID:            fm.ID,
			Quote:         fm.Quote,
			Name:          fm.Name,
			URL:           fm.URL,
			NormalizedURL: normalizedURL,
			ArticleTitle:  fm.ArticleTitle,
			SourceDomain:  fallbackString(fm.SourceDomain, domain, "unknown-source"),
			ArticleSlug:   fallbackString(slugValue, "index"),
			CreatedAt:     createdAt,
			Tags:          copyStrings(fm.Tags),
			BodyHTML:      bodyHTML,
			Location:      location,
		}

		quotesOut = append(quotesOut, quoteObj)
	}

	for pageURL, ids := range urlSet {
		if len(ids) > 1 {
			sort.Strings(ids)
			warnings = append(warnings, fmt.Sprintf("Multiple quotes reference the same url (%s): %s", pageURL, strings.Join(ids, ", ")))
		}
	}

	return &LoadResult{Quotes: quotesOut, Warnings: warnings, Errors: errors}, nil
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func fallbackString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parseFrontMatter(raw string) (*frontMatter, string, error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.TrimSpace(normalized)
	if !strings.HasPrefix(normalized, "---\n") {
		return nil, "", errors.New("missing YAML front matter")
	}

	rest := normalized[len("---\n"):]
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return nil, "", errors.New("unterminated YAML front matter")
	}

	yamlSection := rest[:idx]
	content := strings.TrimSpace(rest[idx+len("\n---"):])

	var fm frontMatter
	if err := yaml.Unmarshal([]byte(yamlSection), &fm); err != nil {
		return nil, "", err
	}

	return &fm, content, nil
}

func normalizeURL(input string) (string, string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", "", nil
	}
	parsed, err := urlParse(trimmed)
	if err != nil {
		return "", "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", "", fmt.Errorf("invalid scheme")
	}
	parsed.Fragment = ""
	normalized := parsed.String()
	normalized = strings.TrimSuffix(normalized, "/")
	return normalized, "", nil
}

func inferDomainAndSlug(normalizedURL string) (string, string) {
	if normalizedURL == "" {
		return "", ""
	}
	parsed, err := urlParse(normalizedURL)
	if err != nil {
		return "", ""
	}
	host := parsed.Hostname()
	path := strings.Trim(parsed.EscapedPath(), "/")
	if path == "" {
		return host, "index"
	}
	sl := slug.Make(path)
	if sl == "" {
		return host, "index"
	}
	return host, sl
}

func parseTime(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, trimmed); err == nil {
			return &ts
		}
	}
	return nil
}

// urlParse is a small wrapper to avoid importing net/url in tests multiple times.
func urlParse(raw string) (*url.URL, error) {
	return url.Parse(raw)
}

func init() {
	slug.Lowercase = true
	slug.CustomSub = nil
}
