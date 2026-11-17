package domain

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf16"

	"golang.org/x/text/unicode/norm"
)

// jsLen counts UTF-16 code units, like JavaScript's String.length, so length
// limits behave exactly as in the TypeScript core.
func jsLen(s string) int { return len(utf16.Encode([]rune(s))) }

// lowerTR lowercases with Turkish rules (I→ı, İ→i), like toLocaleLowerCase("tr-TR").
func lowerTR(s string) string { return strings.ToLowerSpecial(unicode.TurkishCase, s) }

var (
	reBR       = regexp.MustCompile(`(?i)<br\s*/?>`)
	reTag      = regexp.MustCompile(`<[^>]+>`)
	reEntity   = regexp.MustCompile(`(?i)&[#a-z0-9]+;`)
	reNewlines = regexp.MustCompile(`\n{3,}`)
	reNonWord  = regexp.MustCompile(`[^\p{L}\p{N}]+`)
	reSpaces   = regexp.MustCompile(`[\s\x{a0}\p{Zs}\x{feff}\x{2028}\x{2029}]+`) // JS \s
)

var entities = map[string]string{
	"&amp;": "&", "&lt;": "<", "&gt;": ">", "&quot;": `"`, "&#039;": "'", "&#39;": "'", "&nbsp;": " ",
}

// CleanText strips HTML tags from descriptions; returns nil for empty text.
func CleanText(text *string) *string {
	if text == nil || *text == "" {
		return nil
	}
	out := reBR.ReplaceAllString(*text, "\n")
	out = reTag.ReplaceAllString(out, "")
	out = reEntity.ReplaceAllStringFunc(out, func(e string) string {
		if r, ok := entities[strings.ToLower(e)]; ok {
			return r
		}
		return e
	})
	out = reNewlines.ReplaceAllString(out, "\n\n")
	out = strings.TrimSpace(out)
	if out == "" {
		return nil
	}
	return &out
}

// NormalizeTitle is the lowercase, accent-free, punctuation-free form used to compare
// titles across sources.
func NormalizeTitle(s string) string {
	s = norm.NFKD.String(lowerTR(s))
	s = strings.Map(func(r rune) rune {
		if r >= 0x300 && r <= 0x36f {
			return -1
		}
		if r == 'ı' {
			return 'i'
		}
		return r
	}, s)
	s = reNonWord.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func normalizeName(name string) string {
	return reSpaces.ReplaceAllString(strings.TrimSpace(name), " ")
}

func nameKey(name string) string { return lowerTR(normalizeName(name)) }
