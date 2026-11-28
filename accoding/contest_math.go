package accoding

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Accoding contest descriptions mix ordinary TeX with Markdown-escaped TeX.
// Normalize the observed extra escape layer inside math only, after JSON decode.
// Do not apply Markdown unescaping to the document or to arbitrary TeX escapes.
func normalizeContestFormula(s string) string {
	multiline := strings.Contains(s, "\\begin{") || strings.Contains(s, "\\substack{") ||
		strings.Contains(s, "\\shortstack{") || strings.Contains(s, "\\matrix{") ||
		strings.ContainsRune(s, '&')
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			i++
			continue
		}
		n := runLength(s, i, '\\')
		end := i+n
		next := byte(0)
		if end < len(s) { next = s[end] }
		// A triple backslash at a matrix row ending represents the
		// Markdown-escaped TeX row separator. Keep genuine double separators.
		if n == 3 && multiline && (next == '\r' || next == '\n') {
			b.WriteString("\\\\")
			i = end
			continue
		}
		// In ordinary inline or simple display math, a doubled escape
		// before a TeX control word or escaped punctuation is extra Markdown
		// escaping. Never confuse a row separator with an escaped command.
		if n == 2 && !multiline && ((next >= 'a' && next <= 'z') ||
			(next >= 'A' && next <= 'Z') || strings.ContainsRune("{}%&#$", rune(next))) {
			b.WriteByte('\\')
			i = end
			continue
		}
		// Escaped underscores followed by an index are a Markdown escape,
		// not literal underscores in the observed mathematical notation.
		// Preserve escaped punctuation in text-bearing TeX groups.
		if n == 1 && (next == '_' || next == '(' || next == ')') &&
			!strings.Contains(s, "\\text") && !strings.Contains(s, "\\verb") {
			if next != '_' || (i > 0 && end+1 < len(s) &&
				(strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789)}]", rune(s[i-1]))) &&
				(strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789{", rune(s[end+1])))) {
				b.WriteByte(next)
				i = end+1
				continue
			}
		}
		b.WriteString(s[i:end])
		i = end
	}
	return b.String()
}

func escapedAt(s string, pos int) bool {
	n := 0
	for i := pos-1; i >= 0 && s[i] == '\\'; i-- { n++ }
	return n%2 != 0
}

func runLength(s string, pos int, c byte) int {
	i := pos
	for i < len(s) && s[i] == c { i++ }
	return i-pos
}

// Work on raw source spans, not rendered Markdown; the rest of each span is
// copied verbatim, including inline code and escaped dollar signs.
func normalizeContestMathSpan(s string) string {
	var b strings.Builder
	last := 0
	for i := 0; i < len(s); {
		if s[i] == '`' && !escapedAt(s,i) {
			n := runLength(s,i,'`')
			j := i+n
			matched := false
			for j < len(s) {
				k := strings.IndexByte(s[j:], '`')
				if k < 0 { break }
				j += k
				r := runLength(s,j,'`')
				j += r
				if r == n { i = j; matched = true; break }
			}
			if matched { continue }
			i += n
			continue
		}
		if s[i] != '$' || escapedAt(s,i) { i++; continue }
		n := runLength(s,i,'$')
		if n > 2 { i += n; continue }
		end := -1
		for j := i+n; j < len(s); {
			if n == 1 && (s[j] == '\n' || s[j] == '\r') { break }
			if s[j] != '$' || escapedAt(s,j) { j++; continue }
			r := runLength(s,j,'$')
			if r == n { end = j; break }
			j += r
		}
		if end < 0 {
			// An unclosed display span is not safe to interpret.
			if n == 2 { break }
			i++; continue
		}
		{
			old := s[i+n:end]
			normalized := normalizeContestFormula(old)
			if normalized != old {
				b.WriteString(s[last:i+n])
				b.WriteString(normalized)
				last = end
			}
		}
		i = end+n
	}
	if last == 0 { return s }
	b.WriteString(s[last:])
	return b.String()
}

func normalizeContestStatement(markdown string) string {
	if !strings.Contains(markdown, "\\\\") { return markdown }
	source := []byte(markdown)
	doc := goldmark.New().Parser().Parse(text.NewReader(source))
	var b strings.Builder
	last := 0
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus,error) {
		if !entering { return ast.WalkContinue,nil }
		switch n.Kind() {
		case ast.KindParagraph, ast.KindHeading, ast.KindTextBlock:
			lines := n.Lines()
			if lines.Len() == 0 { return ast.WalkSkipChildren,nil }
			start, stop := lines.At(0).Start, lines.At(lines.Len()-1).Stop
			if start >= last && stop >= start && stop <= len(source) {
				b.WriteString(markdown[last:start])
				b.WriteString(normalizeContestMathSpan(markdown[start:stop]))
				last = stop
			}
			return ast.WalkSkipChildren,nil
		case ast.KindCodeBlock, ast.KindFencedCodeBlock, ast.KindHTMLBlock:
			return ast.WalkSkipChildren,nil
		}
		return ast.WalkContinue,nil
	})
	b.WriteString(markdown[last:])
	return b.String()
}
