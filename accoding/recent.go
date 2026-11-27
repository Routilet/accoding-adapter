package accoding
import("context";"sort";"strings";"time")
type RecentContest struct{ID string `json:"id"`;Title string `json:"title"`;Course string `json:"course"`;Start string `json:"start"`;End string `json:"end"`;Status string `json:"status"`}
func parseRecent(html string)([]RecentContest,error){
 n,e:=parseHTML(html);if e!=nil{return nil,e}
 if n.one(attr("input","name","password"))!=nil{return nil,fail(Auth,"请先登录")}
 head:=n.one(func(x *node)bool{return x.tag=="tr"&&x.hasClass("contest-title-tr")})
 if head==nil{return nil,fail(Protocol,"比赛列表结构变化")}
 table:=head.ancestor(tag("table"));if table==nil{return nil,fail(Protocol,"比赛列表缺失")}
 out:=[]RecentContest{};seen:=map[string]bool{}
 for _,row:=range table.all(tag("tr")){
  if row==head{continue};cells:=[]*node{}
  for _,c:=range row.children{if c.tag=="td"{cells=append(cells,c)}}
  if len(cells)!=6{return nil,fail(Protocol,"比赛列表列数变化")}
  v:=func(i int)string{return strings.TrimSpace(cells[i].text())}
  id:=v(0);a:=cells[2].one(tag("a"));if !validID(id)||id=="0"||seen[id]||a==nil||a.attr["href"]!="/contest-ng/index.html#/"+id||v(2)==""{return nil,fail(Protocol,"比赛记录无效")}
  for _,i:=range []int{4,5}{if _,e=time.Parse("2006-01-02 15:04:05",v(i));e!=nil{return nil,fail(Protocol,"比赛时间无效")}}
  seen[id]=true;out=append(out,RecentContest{id,v(2),v(3),v(4),v(5),v(1)})
 }
 sort.SliceStable(out,func(i,j int)bool{return out[i].Start>out[j].Start});if len(out)>10{out=out[:10]};return out,nil
}
func(c *Client) RecentContests(ctx context.Context)([]RecentContest,error){
 ctx,e,done:=c.operation(ctx,false,"");defer done()
 r,err:=e.request(ctx,"GET","/contest/index","","",false);if err!=nil{return nil,err};if r.url.Path!="/contest/index"{return nil,fail(Auth,"比赛列表不可访问")};return parseRecent(string(r.body))
}
// Return only the authenticated user's submissions for the exact contest, once.
func(c *Client) ContestSubmissions(ctx context.Context,id string)([]Submission,error){
 if err:=checkID(id);err!=nil{return nil,err};ctx,e,done:=c.operation(ctx,false,"");defer done()
 u,err:=currentUser(ctx,e);if err!=nil{return nil,err}
 var records []Submission;if err=e.json(ctx,"GET","/api/contests/"+id+"/submissions","","",false,&records);err!=nil{return nil,err}
 out:=[]Submission{};for _,s:=range records{if s.CreatorID!=u.ID{continue};t:=Target{Kind:"contest",ContestID:id,ProblemID:string(s.ProblemID)};if err=t.validate();err!=nil{return nil,err};if err=s.verify(t,u.ID);err!=nil{return nil,err};out=append(out,s)};return out,nil
}
