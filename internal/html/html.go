package html

import (
	"regexp"
)

var (
	sectionPattern = regexp.MustCompile(`(?s){{#(\w+)}}(.*?){{/(\w+)}}`)
	tokenPattern   = regexp.MustCompile(`{{(\w+)}}`)
)

func Apply(template string, data map[string]string) string {
	result := sectionPattern.ReplaceAllStringFunc(template, func(match string) string {
		sub := sectionPattern.FindStringSubmatch(match)
		if len(sub) != 4 {
			return ""
		}
		key := sub[1]
		closing := sub[3]
		if key != closing {
			return ""
		}
		if val, ok := data[key]; ok && val != "" {
			return sub[2]
		}
		return ""
	})

	result = tokenPattern.ReplaceAllStringFunc(result, func(match string) string {
		sub := tokenPattern.FindStringSubmatch(match)
		if len(sub) != 2 {
			return ""
		}
		key := sub[1]
		if val, ok := data[key]; ok {
			return val
		}
		return ""
	})

	return result
}
