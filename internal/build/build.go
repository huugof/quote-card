package build

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/huugof/quote-cards/internal/html"
	"github.com/huugof/quote-cards/internal/manifest"
	"github.com/huugof/quote-cards/internal/quotes"
	"github.com/huugof/quote-cards/internal/render"
	"github.com/huugof/quote-cards/internal/static"
	"github.com/huugof/quote-cards/internal/util"
)

const (
	cardRenderVersion    = "20240505"
	wrapperRenderVersion = "20240505"
	sourceRenderVersion  = "20240505"
)

type Config struct {
	RootDir     string
	BasePath    string
	SiteOrigin  string
	CardVersion *string
	Force       bool
}

type Result struct {
	QuotesTotal        int
	CardsRendered      int
	WrappersRendered   int
	SourcePagesRendered int
	CardsRemoved       int
	WrappersRemoved    int
	SourcePagesRemoved int
}

type removalStats struct {
	cardsRemoved    int
	wrappersRemoved int
}

type sourceGroup struct {
	domain      string
	slug        string
	sourceURL   string
	articleTitle string
	quotes      []*quotes.Quote
}

type groupMeta struct {
	domain string
	slug   string
}

type removedQuote struct {
	id    string
	entry manifest.QuoteEntry
}

type BuildOutcome struct {
	Result   *Result
	Manifest *manifest.Manifest
}

func Execute(cfg Config, quoteList []*quotes.Quote, manifestPath string) (*BuildOutcome, error) {
	cardsDir := filepath.Join(cfg.RootDir, "cards")
	wrappersDir := filepath.Join(cfg.RootDir, "q")
	sourcesDir := filepath.Join(cfg.RootDir, "sources")

	man, err := manifest.Load(manifestPath)
	if err != nil {
		return nil, err
	}

	if cfg.Force {
		if err := cleanOutputs(cardsDir, wrappersDir, sourcesDir); err != nil {
			return nil, err
		}
		man = nil
	}

	result := &Result{QuotesTotal: len(quoteList)}

	if len(quoteList) == 0 {
		if err := cleanOutputs(cardsDir, wrappersDir, sourcesDir); err != nil {
			return nil, err
		}
		if err := os.Remove(manifestPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return &BuildOutcome{Result: result, Manifest: &manifest.Manifest{}}, nil
	}

	fontsHash := util.HashStrings(util.HashBytes(static.AtkinsonRegular), util.HashBytes(static.AtkinsonBold))
	cardRenderHash := util.HashStrings(cardRenderVersion, fontsHash)
	wrapperTemplateHash := util.HashBytes([]byte(static.WrapperTemplate))
	sourceTemplateHash := util.HashBytes([]byte(static.SourceTemplate))

    manifestQuotes := map[string]manifest.QuoteEntry{}
	var prevCardVersion *string
	if man != nil {
		manifestQuotes = man.Quotes
		prevCardVersion = man.CardVersion
	}

	cardVersionKey := cfg.CardVersion

	cardRenderChanged := cfg.Force || man == nil || man.CardRenderHash != cardRenderHash
	wrapperTemplateChanged := cfg.Force || man == nil || man.WrapperTemplateHash != wrapperTemplateHash
	wrapperRenderChanged := cfg.Force || man == nil || man.WrapperRenderVersion != wrapperRenderVersion
	sourceTemplateChanged := cfg.Force || man == nil || man.SourceTemplateHash != sourceTemplateHash
	sourceRenderChanged := cfg.Force || man == nil || man.SourceRenderVersion != sourceRenderVersion
	cardVersionChanged := cfg.Force || !equalStringPtr(prevCardVersion, cardVersionKey)

	dirtyCards := map[string]struct{}{}
	dirtyWrappers := map[string]struct{}{}
	dirtyGroups := map[string]struct{}{}
	nextManifestQuotes := map[string]manifest.QuoteEntry{}
	removedQuotes := []removedQuote{}
	sourceGroups := map[string]*sourceGroup{}
	groupMetadata := map[string]groupMeta{}

	for _, q := range quoteList {
		groupKey := buildGroupKey(q)
		groupMetadata[groupKey] = groupMeta{domain: q.SourceDomain, slug: q.ArticleSlug}
		sourceURL := q.NormalizedURL
		if sourceURL == "" {
			sourceURL = q.URL
		}

		grp := sourceGroups[groupKey]
		if grp == nil {
			grp = &sourceGroup{
				domain:      q.SourceDomain,
				slug:        q.ArticleSlug,
				sourceURL:   sourceURL,
				articleTitle: q.ArticleTitle,
			}
			sourceGroups[groupKey] = grp
		} else if grp.sourceURL == "" {
			grp.sourceURL = sourceURL
		}
		if grp.articleTitle == "" && q.ArticleTitle != "" {
			grp.articleTitle = q.ArticleTitle
		}
		grp.quotes = append(grp.quotes, q)

		entry := buildManifestEntry(q, groupKey, cfg)
		nextManifestQuotes[q.ID] = entry

		prev, ok := manifestQuotes[q.ID]
		cardDirty := cardRenderChanged || !ok || prev.CardHash != entry.CardHash
		wrapperDirty := wrapperRenderChanged || wrapperTemplateChanged || cardVersionChanged || !ok || prev.WrapperHash != entry.WrapperHash
		groupDirty := sourceRenderChanged || sourceTemplateChanged || !ok || prev.GroupItemHash != entry.GroupItemHash || prev.SourceKey != entry.SourceKey

		if cardDirty {
			dirtyCards[q.ID] = struct{}{}
		}
		if wrapperDirty {
			dirtyWrappers[q.ID] = struct{}{}
		}
		if groupDirty {
			dirtyGroups[groupKey] = struct{}{}
		}
	}

    for id, prev := range manifestQuotes {
        if _, exists := nextManifestQuotes[id]; exists {
            continue
        }
        removedQuotes = append(removedQuotes, removedQuote{id: id, entry: prev})
        meta := groupMeta{domain: prev.SourceDomain, slug: prev.ArticleSlug}
        groupMetadata[prev.SourceKey] = meta
        dirtyGroups[prev.SourceKey] = struct{}{}
    }

	if err := ensureDirs(cardsDir, wrappersDir, sourcesDir); err != nil {
		return nil, err
	}

    removal, err := removeDeletedQuoteOutputs(removedQuotes, cardsDir, wrappersDir)
	if err != nil {
		return nil, err
	}
	result.CardsRemoved = removal.cardsRemoved
	result.WrappersRemoved = removal.wrappersRemoved

	sourcePagesRemoved := 0

	for _, q := range quoteList {
		if _, dirty := dirtyCards[q.ID]; !dirty {
			continue
		}
		imageBytes, err := render.RenderQuoteCard(q.Quote)
		if err != nil {
			return nil, fmt.Errorf("render card %s: %w", q.ID, err)
		}
		cardPath := filepath.Join(cardsDir, fmt.Sprintf("%s.jpg", q.ID))
		if err := os.WriteFile(cardPath, imageBytes, 0o644); err != nil {
			return nil, err
		}
		result.CardsRendered++
	}

	for _, q := range quoteList {
		if _, dirty := dirtyWrappers[q.ID]; !dirty {
			continue
		}
		payload := buildWrapperPayload(q, cfg)
		htmlContent := html.Apply(static.WrapperTemplate, payload)
		wrapperDir := filepath.Join(wrappersDir, q.ID)
		if err := os.MkdirAll(wrapperDir, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(wrapperDir, "index.html"), []byte(htmlContent), 0o644); err != nil {
			return nil, err
		}
		result.WrappersRendered++
	}

	for _, group := range sourceGroups {
		sort.SliceStable(group.quotes, func(i, j int) bool {
			a := group.quotes[i]
			b := group.quotes[j]
			timeA := timeValue(a.CreatedAt)
			timeB := timeValue(b.CreatedAt)
			return timeA > timeB
		})
	}

    for groupKey := range dirtyGroups {
        meta := groupMetadata[groupKey]
        grp := sourceGroups[groupKey]
        if grp == nil || len(grp.quotes) == 0 {
            removed, err := removeSourceGroup(meta, sourcesDir)
            if err != nil {
                return nil, err
            }
            sourcePagesRemoved += removed
            continue
        }
        markup := buildSourceGroupHTML(grp, cfg)
        outputDir := filepath.Join(sourcesDir, grp.domain, grp.slug)
        if err := os.MkdirAll(outputDir, 0o755); err != nil {
            return nil, err
        }
        if err := os.WriteFile(filepath.Join(outputDir, "index.html"), []byte(markup), 0o644); err != nil {
            return nil, err
        }
        result.SourcePagesRendered++
    }

	result.SourcePagesRemoved = sourcePagesRemoved

	nextManifest := &manifest.Manifest{
		Version:              1,
		CardVersion:          cardVersionKey,
		CardRenderVersion:    cardRenderVersion,
		CardRenderHash:       cardRenderHash,
		FontsHash:            fontsHash,
		WrapperRenderVersion: wrapperRenderVersion,
		WrapperTemplateHash:  wrapperTemplateHash,
		SourceRenderVersion:  sourceRenderVersion,
		SourceTemplateHash:   sourceTemplateHash,
		Quotes:               nextManifestQuotes,
	}

	if err := manifest.Save(manifestPath, nextManifest); err != nil {
		return nil, err
	}

	return &BuildOutcome{Result: result, Manifest: nextManifest}, nil
}

func buildManifestEntry(q *quotes.Quote, groupKey string, cfg Config) manifest.QuoteEntry {
    tags := append([]string{}, q.Tags...)
    sort.Strings(tags)
    tagsJoined := strings.Join(tags, "|")
    created := ""
    if q.CreatedAt != nil {
        created = q.CreatedAt.UTC().Format(time.RFC3339)
    }

	cardHash := util.HashJSON([]string{
		cardRenderVersion,
		q.Quote,
		q.Name,
		tagsJoined,
	})

	cardVersion := ""
	if cfg.CardVersion != nil {
		cardVersion = *cfg.CardVersion
	}

	wrapperHash := util.HashJSON([]string{
		wrapperRenderVersion,
		cfg.BasePath,
		cfg.SiteOrigin,
		cardVersion,
		q.Quote,
		q.Name,
		q.ArticleTitle,
		q.URL,
		q.SourceDomain,
	})

	groupItemHash := util.HashJSON([]string{
		sourceRenderVersion,
		cfg.BasePath,
		q.ID,
		q.Quote,
		q.Name,
		q.BodyHTML,
		q.URL,
		q.ArticleTitle,
		q.SourceDomain,
		q.ArticleSlug,
		created,
		tagsJoined,
	})

    return manifest.QuoteEntry{
        CardHash:      cardHash,
        WrapperHash:   wrapperHash,
        GroupItemHash: groupItemHash,
        SourceKey:     groupKey,
        SourceDomain:  q.SourceDomain,
        ArticleSlug:   q.ArticleSlug,
    }
}

func buildGroupKey(q *quotes.Quote) string {
	return fmt.Sprintf("%s__%s", q.SourceDomain, q.ArticleSlug)
}

func buildWrapperPayload(q *quotes.Quote, cfg Config) map[string]string {
	sourceDomain := q.SourceDomain
	if sourceDomain == "" {
		sourceDomain = "original-source"
	}
	articleTitle := q.ArticleTitle
	if articleTitle == "" {
		articleTitle = sourceDomain
	}
	hasAuthor := q.Name != ""

	var description string
	if q.ArticleTitle != "" {
		if hasAuthor {
			description = fmt.Sprintf("From %s by %s", q.ArticleTitle, q.Name)
		} else {
			description = fmt.Sprintf("From %s on %s", q.ArticleTitle, sourceDomain)
		}
	} else {
		if hasAuthor {
			description = fmt.Sprintf("%s on %s", q.Name, sourceDomain)
		} else {
			description = fmt.Sprintf("Collected from %s", sourceDomain)
		}
	}

	cardPath := fmt.Sprintf("/cards/%s.jpg", q.ID)
	versionSuffix := ""
	if cfg.CardVersion != nil && *cfg.CardVersion != "" {
		versionSuffix = fmt.Sprintf("?v=%s", url.QueryEscape(*cfg.CardVersion))
	}
	ogImage := absoluteURL(cfg, cardPath+versionSuffix)

	payload := map[string]string{
		"page_title":      util.EscapeHTML(articleTitle),
		"meta_description": util.EscapeHTML(description),
		"og_title":        util.EscapeHTML(articleTitle),
		"og_description":  util.EscapeHTML(description),
		"og_image":        util.EscapeHTML(ogImage),
		"canonical_url":   q.URL,
		"source_url":      q.URL,
		"quote_text":      util.EscapeHTML(q.Quote),
		"card_url":        util.EscapeHTML(publicPath(cfg, cardPath)),
	}

	if hasAuthor {
		payload["quote_author"] = util.EscapeHTML(q.Name)
	} else {
		payload["quote_author"] = ""
	}

	if q.ArticleTitle != "" {
		payload["article_title"] = util.EscapeHTML(q.ArticleTitle)
	} else {
		payload["article_title"] = ""
	}

	return payload
}

func buildSourceGroupHTML(group *sourceGroup, cfg Config) string {
	quoteItems := make([]string, 0, len(group.quotes))
	for _, q := range group.quotes {
		quoteItems = append(quoteItems, buildSourceQuoteHTML(q, cfg))
	}

	pageTitle := group.articleTitle
	if pageTitle == "" {
		pageTitle = fmt.Sprintf("Quotes from %s", group.domain)
	} else {
		pageTitle = fmt.Sprintf("%s — %s", group.articleTitle, group.domain)
	}

	sourceURL := group.sourceURL
	if sourceURL == "" && len(group.quotes) > 0 {
		sourceURL = group.quotes[0].URL
	}

	payload := map[string]string{
		"page_title":   util.EscapeHTML(pageTitle),
		"source_domain": util.EscapeHTML(group.domain),
		"source_url":   sourceURL,
		"quote_items":  strings.Join(quoteItems, "\n\n"),
	}

	return html.Apply(static.SourceTemplate, payload)
}

func buildSourceQuoteHTML(q *quotes.Quote, cfg Config) string {
	parts := []string{
		"<article>",
		fmt.Sprintf("  <blockquote>“%s”</blockquote>", util.EscapeHTML(q.Quote)),
		fmt.Sprintf("  <cite>%s</cite>", util.EscapeHTML(q.Name)),
	}
	if strings.TrimSpace(q.BodyHTML) != "" {
		parts = append(parts, fmt.Sprintf("  <div class=\"body\">%s</div>", q.BodyHTML))
	}
	parts = append(parts, "  <div class=\"meta\">")
	parts = append(parts, fmt.Sprintf("    <span><a href=\"%s\">Quote page</a></span>", util.EscapeHTML(publicPath(cfg, fmt.Sprintf("/q/%s/", q.ID)))))
	parts = append(parts, fmt.Sprintf("    <span><a href=\"%s\">Download JPG</a></span>", util.EscapeHTML(publicPath(cfg, fmt.Sprintf("/cards/%s.jpg", q.ID)))))
	parts = append(parts, "  </div>")
	parts = append(parts, "</article>")
	return strings.Join(parts, "\n")
}

func publicPath(cfg Config, relative string) string {
	normalized := relative
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	return cfg.BasePath + normalized
}

func absoluteURL(cfg Config, relative string) string {
	path := publicPath(cfg, relative)
	if cfg.SiteOrigin == "" {
		return path
	}
	return cfg.SiteOrigin + path
}

func ensureDirs(paths ...string) error {
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func cleanOutputs(paths ...string) error {
	for _, p := range paths {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	return nil
}

func removeDeletedQuoteOutputs(entries []removedQuote, cardsDir, wrappersDir string) (removalStats, error) {
	stats := removalStats{}
	for _, item := range entries {
		cardPath := filepath.Join(cardsDir, fmt.Sprintf("%s.jpg", item.id))
		if err := os.Remove(cardPath); err == nil {
			stats.cardsRemoved++
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return stats, err
		}

		wrapperDir := filepath.Join(wrappersDir, item.id)
		if info, err := os.Stat(wrapperDir); err == nil && info.IsDir() {
			if err := os.RemoveAll(wrapperDir); err != nil {
				return stats, err
			}
			stats.wrappersRemoved++
		}
	}
	return stats, nil
}

func removeSourceGroup(meta groupMeta, sourcesDir string) (int, error) {
	if meta.domain == "" || meta.slug == "" {
		return 0, nil
	}
	dir := filepath.Join(sourcesDir, meta.domain, meta.slug)
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return 0, err
	}
	return 1, nil
}

func timeValue(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return t.Unix()
}

func equalStringPtr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
