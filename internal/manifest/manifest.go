package manifest

import (
	"encoding/json"
	"errors"
	"os"
	"time"
)

type Manifest struct {
	Version               int                       `json:"version"`
	GeneratedAt           string                    `json:"generatedAt"`
	CardVersion           *string                   `json:"cardVersion"`
	CardRenderVersion     string                    `json:"cardRenderVersion"`
	CardRenderHash        string                    `json:"cardRenderHash"`
	FontsHash             string                    `json:"fontsHash"`
	WrapperRenderVersion  string                    `json:"wrapperRenderVersion"`
	WrapperTemplateHash   string                    `json:"wrapperTemplateHash"`
	SourceRenderVersion   string                    `json:"sourceRenderVersion"`
	SourceTemplateHash    string                    `json:"sourceTemplateHash"`
	Quotes                map[string]QuoteEntry     `json:"quotes"`
}

type QuoteEntry struct {
	CardHash      string  `json:"cardHash"`
	WrapperHash   string  `json:"wrapperHash"`
	GroupItemHash string  `json:"groupItemHash"`
	SourceKey     string  `json:"sourceKey"`
	SourceDomain  string  `json:"sourceDomain"`
	ArticleSlug   string  `json:"articleSlug"`
}

func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errorsIs(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Quotes == nil {
		m.Quotes = map[string]QuoteEntry{}
	}
	return &m, nil
}

func Save(path string, m *Manifest) error {
	if m == nil {
		return errors.New("manifest is nil")
	}
	m.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	payload, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func errorsIs(err, target error) bool {
	return errors.Is(err, target)
}
