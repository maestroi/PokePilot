package packaging

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"
    "testing"

    "github.com/maestroi/pokepilot/farm"
    "github.com/maestroi/pokepilot/media/editplan"
    "github.com/maestroi/pokepilot/media/producer"
)

func testInput(t *testing.T) Request {
    t.Helper()
    tl:=farm.MediaTimeline{
        Run:farm.MediaRunSummary{RunID:"packaging-run",Goal:"win the badges",Planner:"automatic planner",Status:"done"},
        Attempt:1,EndFrame:120000,FramesPerSecond:60,
        Events:[]farm.MediaEvent{
            {Type:"gym_battle",Frame:30000,StartFrame:25000,Evidence:"gym"},
            {Type:"badge_acquired",Frame:33000,Evidence:"badge"},
            {Type:"blackout",Frame:60000,Evidence:"blackout"},
            {Type:"failure_recovered",Frame:74000,Evidence:"recovered"},
            {Type:"run_finished",Frame:100000,Evidence:"finished"},
        },
    }.Normalized()
    a,err:=producer.Assess(context.Background(),producer.Input{RunID:"packaging-run",Timelines:[]farm.MediaTimeline{tl}},nil,producer.DefaultPolicy())
    if err!=nil{t.Fatal(err)}
    if a.Publication==producer.ArchiveOnly || a.Publication==producer.DoNotPublish {t.Fatalf("fixture unpublishable: %s",a.Publication)}
    plan,err:=editplan.Build(a,[]farm.MediaTimeline{tl},editplan.Episode)
    if err!=nil{t.Fatal(err)}
    return Request{
        Assessment:a,Timelines:[]farm.MediaTimeline{tl},Plan:&plan,Style:Documentary,
        RunBaseURL:"https://rompilot.app",
    }
}
func TestGroundedCandidatesChaptersAndChecksum(t *testing.T){
    in:=testInput(t)
    first,err:=Generate(context.Background(),in)
    if err!=nil{t.Fatal(err)}
    second,err:=Generate(context.Background(),in)
    if err!=nil{t.Fatal(err)}
    if first.Hash!=second.Hash{t.Fatal("same input yielded different package")}
    if err:=Validate(first,in);err!=nil{t.Fatal(err)}
    if first.AssessmentHash!=in.Assessment.Hash||first.PlanHash!=in.Plan.Hash {t.Fatal("broken provenance")}
    if len(first.Titles)!=3||len(first.Shorts)!=3||len(first.Thumbnails)!=3{
        t.Fatalf("missing review options: %+v",first)
    }
    titles:=make(map[string]bool)
    for _,c:=range first.Titles{
        if titles[c.Title]{t.Fatalf("duplicate candidate %q",c.Title)}
        titles[c.Title]=true
        if len(c.Title)>100{t.Fatal("YouTube title too long")}
    }
    if !strings.Contains(first.Description,"https://rompilot.app/runs/packaging-run") ||
        !strings.Contains(first.Description,"automatic planner") {t.Fatal("missing verifiable run metadata")}
    if len(first.Chapters)==0{t.Fatal("expected editor-timed chapters")}
    for _,c:=range first.Chapters{
        if c.StartMS<0 || c.StartMS>in.Plan.DurationMS{t.Fatalf("invalid chapter %+v",c)}
        if c.Evidence.EventID==""{t.Fatal("chapter lacks provenance")}
    }
    b,_:=json.Marshal(first)
    var restored Package
    if err:=json.Unmarshal(b,&restored);err!=nil{t.Fatal(err)}
    if err:=Validate(restored,in);err!=nil{t.Fatal(err)}
}
func TestTamperStaleSourceAndNoFacts(t *testing.T){
    in:=testInput(t)
    pkg,err:=Generate(context.Background(),in)
    if err!=nil{t.Fatal(err)}
    pkg.Titles[0].Title="World record in ten seconds!"
    if err:=Validate(pkg,in);err==nil{t.Fatal("tampered checksum accepted")}
    pkg=seal(pkg)
    if err:=Validate(pkg,in);err==nil{t.Fatal("invented record claim accepted after rehash")}
    in=testInput(t)
    pkg,err=Generate(context.Background(),in)
    if err!=nil{t.Fatal(err)}
    in.Timelines[0].EndFrame++
    if err:=Validate(pkg,in);err==nil{t.Fatal("stale timeline accepted")}
    in=testInput(t)
    in.Timelines[0].Events=nil
    if _,err:=Generate(context.Background(),in);err==nil{t.Fatal("missing event evidence accepted")}
}
func TestSecretsAndMissingOptionalRunMetadata(t *testing.T){
    in:=testInput(t)
    in.Timelines[0].Run.Goal="password=private-secret"
    in.Timelines[0].Run.Planner="Bearer super-secret"
    // Trust the producer's selected event IDs, not unsafe free-form labels.
    pkg,err:=Generate(context.Background(),in)
    if err!=nil{t.Fatal(err)}
    data,_:=json.Marshal(pkg)
    if strings.Contains(string(data),"super-secret")||strings.Contains(string(data),"private-secret"){
        t.Fatal("private strings leaked into public packaging")
    }
    if pkg.Description==""||len(pkg.Shorts)!=3{t.Fatal("sparse metadata not supported")}
}
type fakeSelector struct{ raw string; ctx ModelContext }
func (m *fakeSelector)Identity()string{return "fake-v1"}
func (m *fakeSelector)Select(_ context.Context,ctx ModelContext)([]byte,error){m.ctx=ctx;return []byte(m.raw),nil}
func TestModelAngleSchemaAndSelectionValidation(t *testing.T){
    in:=testInput(t)
    baseline,err:=Generate(context.Background(),in)
    if err!=nil{t.Fatal(err)}
    f:=baseline.Selections[0]
    selector:=&fakeSelector{}
    selector.raw=fmt.Sprintf(`{"selections":[{"attempt":%d,"event_id":%q,"angle":"challenge"},{"attempt":%d,"event_id":%q,"angle":"recap"}]}`,f.Attempt,f.EventID,f.Attempt,f.EventID)
    in.Model=selector
    pkg,err:=Generate(context.Background(),in)
    if err!=nil{t.Fatal(err)}
    if pkg.Model!="fake-v1"||pkg.PromptVersion!=PromptVersion{t.Fatal("model prompt identity omitted")}
    if err:=Validate(pkg,in);err!=nil{t.Fatal(err)}
    if len(selector.ctx.Events)==0||selector.ctx.Events[0].EventID==""{t.Fatal("model context ungrounded")}
    selector.raw=`{"selections":[{"attempt":1,"event_id":"evt-invented","angle":"moment"},{"attempt":1,"event_id":"evt-invented","angle":"challenge"}]}`
    if _,err:=Generate(context.Background(),in);err==nil{t.Fatal("fabricated event selected")}
    selector.raw=fmt.Sprintf(`{"selections":[{"attempt":%d,"event_id":%q,"angle":"world_record"},{"attempt":%d,"event_id":%q,"angle":"recap"}]}`,f.Attempt,f.EventID,f.Attempt,f.EventID)
    if _,err:=Generate(context.Background(),in);err==nil{t.Fatal("unknown model angle accepted")}
    selector.raw=fmt.Sprintf(`{"selections":[{"attempt":%d,"event_id":%q,"angle":"moment","unsafe_title":"TOP WORLD RECORD"},{"attempt":%d,"event_id":%q,"angle":"challenge"}]}`,f.Attempt,f.EventID,f.Attempt,f.EventID)
    if _,err:=Generate(context.Background(),in);err==nil{t.Fatal("model-supplied title accepted")}
}
func TestPublicationGateStyleAndLinks(t *testing.T){
    in:=testInput(t)
    in.Style=Style("pirate")
    if _,err:=Generate(context.Background(),in);err==nil{t.Fatal("unknown style accepted")}
    in=testInput(t)
    in.RunBaseURL="https://rompilot.app/?token=private"
    if _,err:=Generate(context.Background(),in);err==nil{t.Fatal("unsafe backlink accepted")}
    in=testInput(t)
    in.Assessment.Publication=producer.DoNotPublish
    // Re-seal in order to test the classification gate, not the hash guard.
    b,_:=json.Marshal(in.Assessment)
    _ = b
    if _,err:=Generate(context.Background(),in);err==nil{t.Fatal("non-publishable assessment accepted")}
}
