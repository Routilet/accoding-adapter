package wire
import("context";"encoding/json";"github.com/Routilet/accoding-adapter/accoding")
func(a *accountHandler) extraRead(ctx context.Context,method string,raw json.RawMessage)(json.RawMessage,*Failure){
 if a.user.ID==""{return nil,&Failure{"auth","login required"}}
 u,e:=a.client.Account(ctx);if e!=nil{return nil,accountError(e)};if u.ID!=a.user.ID{a.clear();return nil,&Failure{"auth","identity changed"}}
 if method=="contests"{
  if decode(raw,&struct{}{})!=nil{return nil,&Failure{"invalid_params","invalid list query"}}
  c,ok:=a.client.(interface{RecentContests(context.Context)([]accoding.RecentContest,error)});if !ok{return nil,&Failure{"method_not_found","list unavailable"}}
  rows,e:=c.RecentContests(ctx);if e!=nil{return nil,accountError(e)};b,_:=json.Marshal(map[string]any{"accountId":string(u.ID),"contests":rows});return b,nil
 }
 var p struct{ContestID string `json:"contestId"`;ProblemIDs []string `json:"problemIds"`}
 if decode(raw,&p)!=nil||!judgeID.MatchString(p.ContestID)||len(p.ProblemIDs)==0||len(p.ProblemIDs)>200{return nil,&Failure{"invalid_params","invalid contest query"}}
 statuses:=map[string]judgeStatus{}
 for _,id:=range p.ProblemIDs{if !judgeID.MatchString(id){return nil,&Failure{"invalid_params","invalid problem ID"}};statuses[id]=judgeStatus{AccountID:string(u.ID),Target:accoding.Target{Kind:"contest",ContestID:p.ContestID,ProblemID:id}}}
 c,ok:=a.client.(interface{ContestSubmissions(context.Context,string)([]accoding.Submission,error)});if !ok{return nil,&Failure{"method_not_found","batch unavailable"}}
 records,e:=c.ContestSubmissions(ctx,p.ContestID);if e!=nil{return nil,accountError(e)}
 for _,s:=range records{v,ok:=statuses[string(s.ProblemID)];if !ok{continue};r,valid:=checkedJudge(s,v.Target,u.ID);if !valid{return nil,&Failure{"protocol","judge identity mismatch"}};if r.Result=="AC"{v.Accepted=true};if v.Latest==nil||laterID(r.ID,v.Latest.ID){r.Score=string(s.Score);r.Detail=s.Detail;r.CreatedAt=s.CreatedAt;v.Latest=&r};statuses[string(s.ProblemID)]=v}
 out:=[]judgeStatus{};for _,id:=range p.ProblemIDs{out=append(out,statuses[id])};b,e:=json.Marshal(out);if e!=nil||len(b)>MaxLine-256{return nil,&Failure{"protocol","batch exceeds message limit"}};return b,nil
}
