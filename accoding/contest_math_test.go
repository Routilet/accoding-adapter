package accoding

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestContestMathNormalization(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`第一行 $1 \\le n \\le 10^5$，长度 $1 \\sim 20$。`,`第一行 $1 \le n \le 10^5$，长度 $1 \sim 20$。`},
		{`$1 \le n$`,`$1 \le n$`},
		{`$$\begin{matrix}1\\2\end{matrix}$$`,`$$\begin{matrix}1\\2\end{matrix}$$`},
		{`$\begin{aligned}x\\le y\end{aligned}$`,`$\begin{aligned}x\\le y\end{aligned}$`},
		{`$x\\lemon$ $x\\le y\\z$`,`$x\\lemon$ $x\\le y\\z$`},
		{"`$1 \\\\le n$`", "`$1 \\\\le n$`"},
		{"```\n$1 \\\\le n$\n```\n", "```\n$1 \\\\le n$\n```\n"},
		{"    $1 \\\\le n$\n", "    $1 \\\\le n$\n"},
		{`\$1 \\le n\$`,`\$1 \\le n\$`},
	} {
		if got := normalizeContestStatement(tc.in); got != tc.want {
			t.Errorf("input %q: got %q, want %q",tc.in,got,tc.want)
		}
		if got := normalizeContestStatement(tc.want); got != tc.want {
			t.Errorf("normalization not idempotent: %q",got)
		}
	}
}

func TestContestExportMathAndSamples(t *testing.T) {
	var raw rawContest
	if err := json.Unmarshal([]byte(contestFixture), &raw); err != nil { t.Fatal(err) }
	description := `人数 $1 \\le n \\le 10^5$。` + "\n\n## 输入样例\n\n    2\n\n## 输出样例\n\n    $1 \\\\le n$\n"
	(*raw.Problems)[1].Description = &description
	payload, err := json.Marshal(raw)
	if err != nil { t.Fatal(err) }
	c := newClient(transportFunc(func(r *http.Request) (*http.Response,error) {
		return js(r,string(payload)),nil
	}))
	v,err := c.GetContest(context.Background(),"42")
	if err != nil || len(v.Problems) != 2 { t.Fatal(v,err) }
	p := v.Problems[0]
	if !strings.Contains(p.Statement.Content,`人数 $1 \le n \le 10^5$。`) ||
		len(p.Samples)!=1 || p.Samples[0].Input!="2\n" ||
		p.Samples[0].Expected!="$1 \\\\le n$\n" {
		t.Fatalf("export: %+v",p)
	}
}
