package accoding

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Accoding contest descriptions have been observed with doubled escapes
// in simple inline relations, e.g. $1 \\le n$. The JSON transport has already
// been decoded; this is content normalization, not another JSON unescape.
// Restrict it to known relation commands in inline dollar math, never code,
// display math or formulas containing other potentially intentional line breaks.
var doubledRelation = regexp.MustCompile(`\\\\(leq|geq|le|ge|sim)([^a-zA-Z]|$)`)

func normalizeContestFormula(s string) string {
	if strings.ContainsAny(s, "\r\n&") || strings.Contains(s, "\\begin") ||
		strings.Contains(s, "\\end") || strings.Contains(s, "\\substack") ||
		strings.Contains(s, "\\shortstack") || strings.Contains(s, "\\matrix") ||
		strings.Contains(s, "\\\\\\" ) {
		return s
	}
	// Check every doubled backslash, including adjacent commands. An unknown
	// doubled sequence could be an intentional TeX line break; leave it alone.
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) || s[i+1] != '\\' {
			continue
		}
		m := doubledRelation.FindStringIndex(s[i:])
		if m == nil || m[0] != 0 {
			return s
		}
		i++
	}
	// Do not consume the command's trailing delimiter: adjacent commands
	// must remain independent. The scan above has already validated all pairs.
	return strings.ReplaceAll(s, "\\\\", "\\")
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
			for j < len(s) {
				k := strings.IndexByte(s[j:], '`')
				if k < 0 { break }
				j += k
				r := runLength(s,j,'`')
				j += r
				if r == n { i = j; break }
			}
			if i == j { continue }
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
		if n == 1 {
			old := s[i+1:end]
			normalized := normalizeContestFormula(old)
			if normalized != old {
				b.WriteString(s[last:i+1])
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
