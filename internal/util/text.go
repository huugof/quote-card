package util

import "strings"

var htmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	"\"", "&quot;",
	"'", "&#39;",
)

var svgEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
)

func EscapeHTML(value string) string {
	return htmlEscaper.Replace(value)
}

func EscapeForSVG(value string) string {
	return svgEscaper.Replace(value)
}

func NormalizeWhitespace(value string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(value), " "))
}
