package entitymatch

import (
	"regexp"
	"strings"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
)

var nonWordRe = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeText(raw string) string {
	lower := strings.ToLower(raw)
	normalized := nonWordRe.ReplaceAllString(lower, " ")
	return strings.Join(strings.Fields(normalized), " ")
}

func containsTerm(text, needle string) bool {
	n := normalizeText(needle)
	if n == "" {
		return false
	}
	normText := normalizeText(text)
	if normText == "" {
		return false
	}
	haystack := " " + normText + " "
	term := " " + n + " "
	return strings.Contains(haystack, term)
}

func MatchEntity(text string, entity config.Entity) (bool, float64, string) {
	if !entity.Enabled {
		return false, 0, ""
	}

	if containsTerm(text, entity.Name) {
		return true, 0.95, "keyword_match"
	}
	if containsTerm(text, entity.Symbol) {
		return true, 0.9, "keyword_match"
	}
	for _, alias := range entity.Aliases {
		if containsTerm(text, alias) {
			return true, 0.85, "keyword_match"
		}
	}

	return false, 0, ""
}
