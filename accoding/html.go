package accoding

import (
	"encoding/xml"
 "github.com/yuin/goldmark"
 "github.com/yuin/goldmark/ast"
 "github.com/yuin/goldmark/text"
	"io"
	"regexp"
	"strings"
	"unicode"
)

// A deliberately limited HTML reader for the platform's forms and statement
// structure, not a browser or sanitizer. Unknown markup is never rendered.
// Script/style bodies are opaque; scripts are never executed.
type node struct {
	tag      string
	attr     map[string]string
	children []*node
	data     string
	parent   *node
}

var rawElement = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(?:script|style)\s*>`)

func parseHTML(s string) (*node, error) {
	var scripts []string
	s = rawElement.ReplaceAllStringFunc(s, func(raw string) string {
		if strings.HasPrefix(strings.ToLower(raw), "<script") {
			i := strings.IndexByte(raw, '>')
			j := strings.LastIndex(strings.ToLower(raw), "</script")
			if i >= 0 && j > i {
				scripts = append(scripts, raw[i+1:j])
			}
		}
		return ""
	})
	d := xml.NewDecoder(strings.NewReader(s))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	root := &node{tag: "root"}
	cur := root
	count := 0
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fail(Protocol, "HTML 结构无法解析")
		}
		count++
		if count > 250000 {
			return nil, fail(Protocol, "HTML 结构过大")
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{tag: strings.ToLower(t.Name.Local), attr: map[string]string{}, parent: cur}
			for _, a := range t.Attr {
				n.attr[strings.ToLower(a.Name.Local)] = a.Value
			}
			cur.children = append(cur.children, n)
			cur = n
		case xml.EndElement:
			if cur.parent != nil {
				cur = cur.parent
			}
		case xml.CharData:
			cur.children = append(cur.children, &node{data: string(t), parent: cur})
		}
	}
	root.data = strings.Join(scripts, "\n")
	return root, nil
}
func (n *node) all(match func(*node) bool) []*node {
	if n == nil {
		return nil
	}
	var out []*node
	stack := []*node{n}
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if match(x) {
			out = append(out, x)
		}
		for i := len(x.children) - 1; i >= 0; i-- {
			stack = append(stack, x.children[i])
		}
	}
	return out
}
func (n *node) one(match func(*node) bool) *node {
	a := n.all(match)
	if len(a) > 0 {
		return a[0]
	}
	return nil
}
func (n *node) ancestor(match func(*node) bool) *node {
	for x := n; x != nil; x = x.parent {
		if match(x) {
			return x
		}
	}
	return nil
}
func tag(t string) func(*node) bool { return func(n *node) bool { return n.tag == t } }
func attr(t, k, v string) func(*node) bool {
	return func(n *node) bool { return n.tag == t && n.attr[k] == v }
}
func (n *node) hasClass(c string) bool { return includes(strings.Fields(n.attr["class"]), c) }
func (n *node) text() string {
	var b strings.Builder
	for _, x := range n.all(func(x *node) bool { return x.tag == "" || x.tag == "br" }) {
		if x.tag == "br" {
			b.WriteByte('\n')
		} else {
			b.WriteString(x.data)
		}
	}
	return b.String()
}
func (n *node) statementText() string {
	var b strings.Builder
	var visit func(*node, int)
	visit = func(x *node, depth int) {
		if depth > 256 {
			return
		}
		switch x.tag {
		case "form", "script", "style", "button", "input", "select", "textarea":
			return
		case "":
			b.WriteString(x.data)
			return
		case "br":
			b.WriteByte('\n')
			return
		case "pre":
			b.WriteString("\n```\n" + x.text() + "\n```\n")
			return
		}
		block := isHeading(x.tag) || x.tag == "p" || x.tag == "div" || x.tag == "li" || x.tag == "tr"
		if block {
			b.WriteByte('\n')
		}
		if isHeading(x.tag) {
			b.WriteString("## ")
		}
		for _, c := range x.children {
			visit(c, depth+1)
		}
		if block {
			b.WriteByte('\n')
		}
	}
	visit(n, 0)
	return strings.TrimSpace(b.String())
}
func isHeading(s string) bool { return len(s) == 2 && s[0] == 'h' && s[1] >= '1' && s[1] <= '6' }

var mathNum = regexp.MustCompile(`\$\s*([0-9]+)\s*\$`)

func normalizeHeading(s string) string {
	s = mathNum.ReplaceAllString(s, "$1")
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsDigit(r) || strings.ContainsRune(":：#（）()_-*", r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}
func headingKind(s string) string {
	switch normalizeHeading(s) {
	case "输入样例", "样例输入", "输入示例", "示例输入", "sampleinput", "inputsample", "exampleinput":
		return "input"
	case "输出样例", "样例输出", "输出示例", "示例输出", "sampleoutput", "outputsample", "exampleoutput":
		return "expected"
	}
	return ""
}

type sampleReader struct {
	kind                                              string
	inputs, outputs                                   []string
	inputDeclared, inputSection, noInput, empty, code bool
}

func (r *sampleReader) finish() {
	if r.kind == "input" && r.empty && !r.code {
		r.inputs = append(r.inputs, "")
	}
}
func (r *sampleReader) heading(s string) {
	r.finish()
	r.kind = headingKind(s)
	n := normalizeHeading(s)
	r.inputSection = r.kind == "input" || includes([]string{"输入", "输入格式", "输入说明", "输入描述", "input", "inputformat", "inputdescription", "inputspecification"}, n)
	if r.inputSection {
		r.inputDeclared = true
	}
	r.empty = false
	r.code = false
}
func (r *sampleReader) paragraph(s string) {
	if headingKind(s) != "" {
		r.heading(s)
		return
	}
	s = strings.ToLower(strings.Join(strings.Fields(s), ""))
	s = strings.TrimRight(s, "。.!！,，;；")
	if r.inputSection {
		if includes([]string{"无", "无输入", "没有输入", "无需输入", "不需要输入", "本题无输入", "本题无需输入", "noinput", "noinputisrequired", "thereisnoinput"}, s) {
			r.empty = true
			r.noInput = true
		}
	}
}
func (r *sampleReader) block(s string) {
	r.code = true
	switch r.kind {
	case "input":
		r.inputs = append(r.inputs, s)
	case "expected":
		r.outputs = append(r.outputs, s)
	}
}
func (r *sampleReader) result() ([]Sample, []string) {
	r.finish()
	var samples []Sample
	var warnings []string
	if len(r.inputs) == 0 && len(r.outputs) > 0 && (r.noInput || !r.inputDeclared) {
		for range r.outputs {
			r.inputs = append(r.inputs, "")
		}
	}
	if len(r.inputs) == 0 || len(r.inputs) != len(r.outputs) {
		return nil, []string{"公开样例未能完整配对，请检查题面并手动补充；未导入可能错配的数据"}
	}
	for i := 0; i < len(r.inputs) && i < len(r.outputs); i++ {
		samples = append(samples, Sample{r.inputs[i], r.outputs[i]})
	}
	return samples, warnings
}
func samplesHTML(n *node) ([]Sample, []string) {
	r := sampleReader{}
	for _, x := range n.all(func(x *node) bool { return isHeading(x.tag) || x.tag == "p" || x.tag == "pre" }) {
		if x.ancestor(func(y *node) bool { return y.tag == "form" || y.tag == "script" || y.tag == "style" }) != nil {
			continue
		}
		if x.tag == "pre" {
			r.block(x.text())
		} else if isHeading(x.tag) {
			r.heading(x.text())
		} else {
			r.paragraph(x.text())
		}
	}
	return r.result()
}
// Use CommonMark block parsing rather than interpreting indented data as fences.
func samplesMarkdown(s string) ([]Sample, []string) {
 r:=sampleReader{}
 source:=[]byte(strings.ReplaceAll(s,"\r\n","\n"))
 doc:=goldmark.New().Parser().Parse(text.NewReader(source))
 for n:=doc.FirstChild();n!=nil;n=n.NextSibling(){
  switch n.Kind(){
  case ast.KindHeading:r.heading(string(n.Text(source)))
  case ast.KindParagraph:
   p:=strings.TrimSpace(string(n.Text(source)))
   // Observed explicit output label after an input sample, not general prose.
   if (p=="输出："||p=="输出:") && r.kind=="input" && len(r.inputs)>len(r.outputs){r.heading("输出样例")}else{r.paragraph(p)}
  case ast.KindCodeBlock,ast.KindFencedCodeBlock:
   if r.kind==""{continue}
   lines:=n.Lines()
   if n.Kind()==ast.KindFencedCodeBlock {
    // CommonMark accepts unterminated fences; never import an ambiguous sample.
    if lines.Len()==0{return nil,[]string{"空或未闭合的围栏样例，请手动核对"}}
    last:=lines.At(lines.Len()-1)
    rest:=strings.TrimLeft(string(source[last.Stop:]),"\n")
    next:=strings.TrimSpace(strings.SplitN(rest,"\n",2)[0])
    if !(strings.HasPrefix(next,"```")&&strings.Trim(next,"`")==""||strings.HasPrefix(next,"~~~")&&strings.Trim(next,"~")==""){return nil,[]string{"题面代码块未闭合，请手动核对公开样例"}}
   }
   r.block(string(lines.Value(source)))
  }
 }
 return r.result()
}
