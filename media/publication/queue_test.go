package publication

import (
 "strings"
 "testing"
 "time"

 "github.com/maestroi/pokepilot/media/producer"
 "github.com/maestroi/pokepilot/media/packaging"
 "github.com/maestroi/pokepilot/media/editplan"
)

func fixture(t *testing.T)(Candidate,Inputs){
 t.Helper()
 a:=producer.Assessment{
  RunID:"run-studio",Hash:strings.Repeat("a",64),Publication:producer.FullEpisode,
  Beats:[]producer.Beat{{Kind:"gym"},{Kind:"recovery"},{Kind:"finish"}},
  Scores:producer.Scores{Novelty:producer.Score{Value:75},Drama:producer.Score{Value:64},
   ChallengeDifficulty:producer.Score{Value:60},FailureAndRecovery:producer.Score{Value:78}},
 }
 pkg:=packaging.Package{Hash:strings.Repeat("b",64),AssessmentHash:a.Hash,
  PlanHash:strings.Repeat("c",64),Titles:[]packaging.TitleCandidate{
   {ID:"title-1",Evidence:producer.EvidenceRef{EventID:"a-beat"},Title:"PokePilot recorded gym event"},
  }}
 plan:=editplan.Plan{Hash:pkg.PlanHash,Profile:editplan.Episode}
 p:=Proposal{RunID:a.RunID,AssessmentHash:a.Hash,PackageHash:pkg.Hash,
  PlanHash:pkg.PlanHash,Format:Episode,TitleID:"title-1",ThumbnailID:"selected-thumb"}
 c,err:=New(p)
 if err!=nil{t.Fatal(err)}
 return c,Inputs{Assessment:a,Package:&pkg,Plan:&plan,VideoReady:true,
  VideoKey:"replays/edited.mp4",VideoSize:1024,ThumbnailID:"selected-thumb",ThumbnailKey:"art/thumb.jpg"}
}
func TestApprovalRetryAndNoImplicitPublication(t *testing.T){
 c,in:=fixture(t)
 if state,reason:=Evaluate(c,in,DefaultPolicy());state!=Queued{t.Fatalf("gate %s: %s",state,reason)}
 if CanDispatch(c,time.Now())==nil{t.Fatal("unapproved candidate can publish")}
 now:=time.Unix(101000,0)
 ok,err:=Approve(c,in,DefaultPolicy(),now)
 if err!=nil{t.Fatal(err)}
 if ok.State!=Approved||!ok.Approved||ok.SourceKey!=in.VideoKey{t.Fatal("approval did not seal source")}
 if err:=CanDispatch(ok,now);err!=nil{t.Fatal(err)}
 if err:=ok.Validate();err!=nil{t.Fatal(err)}
 ok.State=Failed
 if err:=CanDispatch(ok,now);err!=nil{t.Fatal("failed upload should be safely retryable",err)}
 ok.State=Published
 if err:=CanDispatch(ok,now);err==nil{t.Fatal("published candidate can upload again")}
 if _,err:=Reject(ok,now);err==nil{t.Fatal("published candidate was rejected")}
 rejected,err:=Reject(c,now)
 if err!=nil||rejected.State!=Skipped||rejected.Approved{t.Fatal("rejection not durable")}
 if err:=CanDispatch(rejected,now);err==nil{t.Fatal("rejected candidate could publish")}
}
func TestQualityGatesAndOutputClass(t *testing.T){
 c,in:=fixture(t)
 table:=[]struct{name string; mutate func(*Candidate,*Inputs);want State}{
  {"not ready",func(_ *Candidate,x *Inputs){x.VideoReady=false},Rendering},
  {"insufficient beats",func(_ *Candidate,x *Inputs){x.Assessment.Beats=nil},Blocked},
  {"not enough quality",func(_ *Candidate,x *Inputs){x.Assessment.Scores=producer.Scores{}},Blocked},
  {"stale thumbnail",func(_ *Candidate,x *Inputs){x.ThumbnailID="new-selection"},Blocked},
  {"package mismatch",func(_ *Candidate,x *Inputs){x.Package.Hash="wrong"},Blocked},
  {"archive classification",func(_ *Candidate,x *Inputs){x.Assessment.Publication=producer.ArchiveOnly},Skipped},
  {"do not publish",func(_ *Candidate,x *Inputs){x.Assessment.Publication=producer.DoNotPublish},Skipped},
  {"short-only cannot be full",func(_ *Candidate,x *Inputs){x.Assessment.Publication=producer.ShortOnly},Blocked},
  {"similarity",func(_ *Candidate,x *Inputs){x.Assessment.Scores.RecentSimilarity=&producer.Score{Value:96}},Blocked},
 }
 for _,tc:=range table{t.Run(tc.name,func(t *testing.T){v:=c;x:=in;p:=*in.Package;x.Package=&p;a:=in.Assessment;x.Assessment=a;tc.mutate(&v,&x);state,_:=Evaluate(v,x,DefaultPolicy());if state!=tc.want{t.Fatalf("got %s expected %s",state,tc.want)}})}
 short:=c
 short.Proposal.Format=Short
 short.Proposal.TitleID="short-1"
 x:=in
 p:=*in.Package
 p.Shorts=[]packaging.ShortCandidate{{ID:"short-1",Evidence:producer.EvidenceRef{EventID:"verified"},Title:"Short moment"}}
 x.Package=&p
 plan:=*in.Plan;plan.Profile=editplan.Short
 x.Plan=&plan
 short.ID=Hash(short.Proposal)
 if state,reason:=Evaluate(short,x,DefaultPolicy());state!=Queued{t.Fatalf("short blocked: %s %s",state,reason)}
 archive,err:=New(Proposal{RunID:c.Proposal.RunID,AssessmentHash:c.Proposal.AssessmentHash,Format:Archive})
 if err!=nil{t.Fatal(err)}
 if state,_:=Evaluate(archive,x,DefaultPolicy());state!=Skipped{t.Fatal("archive output should skip")}
}
func TestIdentityAndScheduling(t *testing.T){
 c,_:=fixture(t)
 twice,err:=New(c.Proposal)
 if err!=nil||c.ID!=twice.ID{t.Fatal("identity is unstable")}
 c.Proposal.Privacy="public"
 if c.Validate()==nil{t.Fatal("mutated candidate identity accepted")}
 c,_=fixture(t)
 c.Proposal.NotBefore=123460
 c.ID=Hash(c.Proposal)
 c.State=Approved;c.Approved=true;c.SourceKey="safe";c.SourceSize=100
 if CanDispatch(c,time.Unix(123000,0))==nil{t.Fatal("early dispatch allowed")}
 if CanDispatch(c,time.Unix(123460,0))!=nil{t.Fatal("due publication blocked")}
 c,_=fixture(t)
 c.Proposal.Format=Format("invalid")
 if _,err:=New(c.Proposal);err==nil{t.Fatal("invalid format allowed")}
}
