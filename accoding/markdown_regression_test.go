package accoding
import("testing";"strings")
func TestAccodingIndentedSamples(t *testing.T){
 for _,tc:=range []struct{md,input,output string}{
 {"## 输入样例\r\n\r\n    ~~~\r\n\r\n## 输出样例\r\n\r\n    @@`\r\n","~~~\n","@@`\n"},
 {"## 输入样例\n    1\n\n输出：\n\n        E   \n        S   \n\n## Hint\n    unrelated\n","1\n","    E   \n    S   \n"},
 {"## 输入\n无\n## 输出样例\n    hello\n","","hello\n"},
 {"## 输入样例 1\n    2\n\n## 输出样例 1\n    *\n\n    ^\n","2\n","*\n\n^\n"},
 }{s,w:=samplesMarkdown(tc.md);if len(s)!=1||len(w)!=0||s[0].Input!=tc.input||s[0].Expected!=tc.output{t.Fatalf("%q: %+v %v",tc.md,s,w)}}
}
func TestRecentContestOrdering(t *testing.T){
 html:="<table><tr class=\"contest-title-tr\"><td>header</td></tr>"
 for _,id:=range []string{"1","3","2"}{html+="<tr><td>"+id+"</td><td>已结束</td><td><a href=\"/contest-ng/index.html#/"+id+"\">比赛"+id+"</a></td><td>课程</td><td>2026-11-0"+id+" 00:00:00</td><td>2026-12-01 00:00:00</td></tr>"}
 rows,e:=parseRecent(html+"</table>");if e!=nil||len(rows)!=3||rows[0].ID!="3"||!strings.Contains(rows[1].Title,"2"){t.Fatal(rows,e)}
}
