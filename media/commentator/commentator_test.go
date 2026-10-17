package commentator

import (
 "context"
 "errors"
 "testing"
 "time"

 "github.com/maestroi/pokepilot/farm"
)

type fakeModel struct {response string; err error}
func (f fakeModel) Identity() string {return "test-model"}
func (f fakeModel) Generate(_ context.Context,_ Context,_ farm.MediaEvent,_ Settings) ([]byte,error) {return []byte(f.response),f.err}

func event(id,kind,summary string,frame uint64) farm.MediaEvent {
 return farm.MediaEvent{ID:id,Type:kind,Summary:summary,Frame:frame,TimestampMS:int64(frame)*16}
}
func settings() Settings {s:=DefaultSettings();s.Enabled=true;return s}
func TestSilenceCooldownPriorityAndDeduplication(t *testing.T) {
 engine,err:=New(settings(),nil)
 if err!=nil {t.Fatal(err)}
 now:=time.Unix(100000,0)
 input:=Context{RunID:"run-1",Attempt:1,Frame:100,Recent:[]farm.MediaEvent{event("walk","navigation","Walking",10)}}
 if _,ok:=engine.Consider(context.Background(),now,input);ok {t.Fatal("navigation must remain silent")}
 input.Recent=append(input.Recent,event("badge","badge_acquired","Boulder Badge earned",20))
 out,ok:=engine.Consider(context.Background(),now,input)
 if !ok||out.Action!=Milestone||out.EventID!="badge" {t.Fatalf("first commentary=%+v %v",out,ok)}
 if _,ok:=engine.Consider(context.Background(),now.Add(10*time.Minute),input);ok {t.Fatal("already seen event emitted again")}
 input.Recent=[]farm.MediaEvent{event("stall","navigation_stall","A long navigation stall",30)}
 if _,ok:=engine.Consider(context.Background(),now.Add(10*time.Second),input);ok {t.Fatal("routine stall should respect cooldown")}
 input.Recent=[]farm.MediaEvent{event("evolution","evolution","The starter evolved",40)}
 out,ok=engine.Consider(context.Background(),now.Add(10*time.Second),input)
 if !ok || out.EventID!="evolution" {t.Fatalf("critical event should bypass normal cadence %+v %v",out,ok)}
 input.Recent=[]farm.MediaEvent{event("wipe","near_wipe","One team member remains",50)}
 if _,ok:=engine.Consider(context.Background(),now.Add(11*time.Second),input);ok {t.Fatal("critical event must respect minimum spacing")}
 input.Recent=[]farm.MediaEvent{event("another-stall","stall","Navigation stalled again",60)}
 if _,ok:=engine.Consider(context.Background(),now.Add(4*time.Minute),input);ok {t.Fatal("sparse cadence is 2 x configured seconds")}
 input.Recent=[]farm.MediaEvent{event("resolved","recovery_stall","Navigation recovered",70)}
 if _,ok:=engine.Consider(context.Background(),now.Add(5*time.Minute),input);!ok {t.Fatal("recovery after cooldown should be announced")}
}
func TestMalformedOutputAndProviderErrorFailSilent(t *testing.T) {
 invalid:=[]string{
  `{"action":"comment","text":"ok","private_trace":"leak"}`,
  `{"action":"milestone","text":""}`,
  `{"action":"silence","text":"should not speak"}`,
  `{"action":"comment","text":"ok"}{}`,
  `{"action":"arbitrary","text":"ok"}`,
  "not json",
 }
 now:=time.Now()
 for _,response:=range invalid {
  engine,err:=New(settings(),fakeModel{response:response})
  if err!=nil {t.Fatal(err)}
  if _,ok:=engine.Consider(context.Background(),now,Context{
   RunID:"run-1",Attempt:1,Frame:100,Recent:[]farm.MediaEvent{event("badge","badge_acquired","Boulder Badge earned",99)},
  });ok {t.Errorf("malformed response caused commentary %q",response)}
 }
 engine,_:=New(settings(),fakeModel{err:errors.New("provider failed")})
 if _,ok:=engine.Consider(context.Background(),now,Context{
  RunID:"run-1",Attempt:1,Frame:100,Recent:[]farm.MediaEvent{event("badge","badge_acquired","Boulder Badge earned",99)},
 });ok {t.Fatal("provider failure must result in silence")}
}
func TestComparisonRequiresSourcedHistoryAndAttemptChangesReset(t *testing.T) {
 model:=fakeModel{response:`{"action":"comparison","text":"The last verified run took longer."}`}
 engine,_:=New(settings(),model)
 now:=time.Now()
 input:=Context{RunID:"live",Attempt:1,Frame:90,Recent:[]farm.MediaEvent{event("record","personal_best","New personal best",80)}}
 if _,ok:=engine.Consider(context.Background(),now,input);ok {t.Fatal("comparison must require history")}
 input.Attempt=2
 input.History=[]string{"run-older, source stats, duration 120s"}
 out,ok:=engine.Consider(context.Background(),now.Add(time.Minute),input)
 if !ok||out.Action!=Comparison||out.Model!="test-model" {t.Fatalf("verified comparison rejected %+v %v",out,ok)}
}
func TestSettingsRejectUnsafeValues(t *testing.T) {
 for _,s:=range []Settings{
  {Enabled:true,Style:"unbounded",Verbosity:"sparse",CadenceSeconds:60},
  {Enabled:true,Style:"calm",Verbosity:"chatter",CadenceSeconds:60},
  {Enabled:true,Style:"calm",Verbosity:"normal",CadenceSeconds:0},
  {Enabled:true,Style:"calm",Verbosity:"normal",CadenceSeconds:60,Voice:"../../secret"},
 } {
  if _,err:=New(s,nil);err==nil {t.Errorf("accepted unsafe configuration %+v",s)}
 }
}
