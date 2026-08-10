package knowledge

import (
	"strings"
	"unicode"
)

// tokenize emits lowercase Latin/number terms and overlapping Han unigrams +
// bigrams. The latter gives useful Chinese lexical recall without an external
// segmentation dependency.
func tokenize(value string) []string {
	value = strings.ToLower(value)
	result := make([]string, 0, len(value)/2)
	var word []rune
	var han []rune
	flushWord := func() {
		if len(word) > 1 || (len(word) == 1 && unicode.IsDigit(word[0])) {
			result = append(result, string(word))
		}
		word = word[:0]
	}
	flushHan := func() {
		for idx, current := range han {
			result = append(result, string(current))
			if idx+1 < len(han) {
				result = append(result, string(han[idx:idx+2]))
			}
		}
		han = han[:0]
	}

	for _, current := range value {
		switch {
		case unicode.Is(unicode.Han, current):
			flushWord()
			han = append(han, current)
		case unicode.IsLetter(current) || unicode.IsDigit(current) || current == '_' || current == '-':
			flushHan()
			word = append(word, current)
		default:
			flushWord()
			flushHan()
		}
	}
	flushWord()
	flushHan()
	return result
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeForPhrase(value string) string {
	var result strings.Builder
	for _, current := range strings.ToLower(value) {
		if unicode.IsLetter(current) || unicode.IsDigit(current) {
			result.WriteRune(current)
		}
	}
	return result.String()
}
