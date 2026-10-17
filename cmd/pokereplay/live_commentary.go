package main

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "os"
 "path"
 "strings"
 "sync"
 "time"

 "github.com/maestroi/pokepilot/media/commentator"
 "github.com/maestroi/pokepilot/media/producer"
)

const (
 liveCommentaryPoll = time.Second
 maxCommentaryHistory = 128
 maxCommentaryRequest = 8 << 10
 maxTTSBytes = 24_000 * 2 * 12 // 12 seconds mono PCM @ 24k
)

// The commentator has a separate lifecycle and process inputs from the game.
// Only safe live-run telemetry ever crosses into its model.
type liveCommentarySession struct {
 server *replayServer
 runID string
 engine *commentator.Engine
 settings commentator.Settings
 historyFacts []string
 cancel context.CancelFunc
 done chan struct{}

 mu sync.Mutex
 state string
 emitted uint64
 history []commentator.Emission
 lastError string
 audio chan []byte // bounded, best-effort presentation-only speech PCM
}

type liveCommentaryStatus struct {
 RunID string `json:"run_id"`
 State string `json:"state"`
 Settings commentator.Settings `json:"settings"`
 Model string `json:"model"`
 TTSConfigured bool `json:"tts_configured"`
 Emitted uint64 `json:"emitted"`
 Recent []commentator.Emission `json:"recent,omitempty"`
 LastError string `json:"last_error,omitempty"`
}

func configuredLiveCommentaryModel() (commentator.Model,error) {
 endpoint:=strings.TrimSpace(os.Getenv("POKEPILOT_LIVE_COMMENTARY_CHAT_URL"))
 name:=strings.TrimSpace(os.Getenv("POKEPILOT_LIVE_COMMENTARY_MODEL"))
 if endpoint=="" {
  if name!="" {return nil,errors.New("live commentary model requires chat URL")}
  return nil,nil
 }
 if name=="" {return nil,errors.New("live commentary chat URL requires model")}
 return commentator.ChatModel{
  Endpoint:endpoint,ModelID:name,Deployment:os.Getenv("POKEPILOT_LIVE_COMMENTARY_DEPLOYMENT"),
  APIKey:os.Getenv("POKEPILOT_LIVE_COMMENTARY_API_KEY"),
 },nil
}

func liveTTSConfigured() bool {
 return strings.TrimSpace(os.Getenv("POKEPILOT_LIVE_TTS_URL"))!="" && strings.TrimSpace(os.Getenv("POKEPILOT_LIVE_TTS_MODEL"))!=""
}

func (s *replayServer) commentarySessionForRun(runID string) *liveCommentarySession {
 s.commentaryMu.Lock()
 defer s.commentaryMu.Unlock()
 return s.commentarySessions[strings.TrimSpace(runID)]
}

func (s *replayServer) stopLiveCommentaries() {
 s.commentaryMu.Lock()
 sessions:=make([]*liveCommentarySession,0,len(s.commentarySessions))
 for _,session:=range s.commentarySessions {sessions=append(sessions,session)}
 s.commentaryMu.Unlock()
 for _,session:=range sessions {session.cancel()}
}

func (c *liveCommentarySession) snapshot() liveCommentaryStatus {
 c.mu.Lock()
 defer c.mu.Unlock()
 model:="rules:"+commentator.PolicyVersion
 if c.engine.Model!=nil {model=c.engine.Model.Identity()}
 out:=liveCommentaryStatus{
  RunID:c.runID,State:c.state,Settings:c.settings,Model:model,
  TTSConfigured:liveTTSConfigured(),Emitted:c.emitted,LastError:c.lastError,
 }
 // Never export raw model payloads. Only vetted emitted text is inspectable.
 if n:=len(c.history);n>0 {
  start:=0;if n>20 {start=n-20}
  out.Recent=append([]commentator.Emission(nil),c.history[start:]...)
 }
 return out
}

func (c *liveCommentarySession) remember(emission commentator.Emission) {
 c.mu.Lock()
 c.emitted++
 c.history=append(c.history,emission)
 if len(c.history)>maxCommentaryHistory {c.history=append([]commentator.Emission(nil),c.history[len(c.history)-maxCommentaryHistory:]...)}
 c.mu.Unlock()
}

func (c *liveCommentarySession) run(ctx context.Context) {
 defer close(c.done)
 defer func(){c.mu.Lock();c.state="stopped";c.mu.Unlock()}()
 ticker:=time.NewTicker(liveCommentaryPoll)
 defer ticker.Stop()
 for {
  run,err:=c.server.fetchLiveRun(ctx,c.runID)
  if err!=nil {
   if ctx.Err()!=nil{return}
   // Provider and wall errors are local to this presentation sidecar.
   c.mu.Lock();c.lastError="live telemetry temporarily unavailable";c.mu.Unlock()
  } else if liveRunBetweenAttempts(run.Status) {
   // Never turn a handoff into a false terminal event.
  } else if !strings.EqualFold(strings.TrimSpace(run.Status),"running") {
   return
  } else {
   c.process(ctx,run)
  }
  select {
  case <-ctx.Done():return
  case <-ticker.C:
  }
 }
}

func (c *liveCommentarySession) process(ctx context.Context,run liveRunView) {
 // Activity detail is not presentation-safe. Never pass it to a model,
 // even as fallback when a summary is absent.
 safeRun:=run
 safeRun.Activity=append([]liveRunActivity(nil),run.Activity...)
 for i:=range safeRun.Activity {safeRun.Activity[i].Detail=""}
 timeline:=liveTimelineFromRun(safeRun)
 in:=commentator.Context{
  RunID:c.runID,Attempt:timeline.Attempt,Goal:run.Goal,Objective:run.Decision,
  Location:fmt.Sprintf("map %d at %d,%d",run.Map,run.X,run.Y),Frame:run.Frame,
  History:append([]string(nil),c.historyFacts...),
 }
 if run.Player!=nil {in.BadgeCount=len(run.Player.Badges);in.PartyCount=len(run.Player.Party)}
 // Recent semantic events are bounded. The policy deduplicates across polls;
 // no raw prompt, activity Detail, logs or arbitrary planner state is sent.
 for _,event:=range timeline.Events {
  if len(in.Recent)>=32 {in.Recent=in.Recent[1:]}
  in.Recent=append(in.Recent,event)
 }
 modelCtx,cancel:=context.WithTimeout(ctx,10*time.Second)
 emission,ok:=c.engine.Consider(modelCtx,time.Now(),in)
 cancel()
 if !ok{return}
 if c.settings.TTS {
  emission.TTSState="unavailable"
  if liveTTSConfigured() {
   ttsCtx,ttsCancel:=context.WithTimeout(ctx,12*time.Second)
   pcm,err:=generateLivePCM(ttsCtx,emission.Text,c.settings.Voice)
   ttsCancel()
   if err!=nil {
    emission.TTSState="failed"
   } else {
    emission.TTSState="queued"
    select {case c.audio<-pcm:default:emission.TTSState="dropped"}
   }
  }
 } else {emission.TTSState="disabled"}
 c.remember(emission)
 // Persistence is best effort and never gates either gameplay or broadcast.
 if c.server.store!=nil {
  record,err:=json.Marshal(emission)
  if err==nil {
   key:=liveCommentaryObjectKey(c.runID,emission)
   persistCtx,release:=context.WithTimeout(ctx,5*time.Second)
   _,err=c.server.store.PutObject(persistCtx,key,"application/json",record)
   release()
  }
  if err!=nil {c.mu.Lock();c.lastError="commentary persistence temporarily unavailable";c.mu.Unlock()}
 }
}

func liveCommentaryObjectKey(runID string,emission commentator.Emission) string {
 hash:=sha256.Sum256([]byte(runID))
 eventHash:=sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%d",emission.Attempt,emission.EventID,emission.EmittedAt.UnixNano())))
 return path.Join("studio",hex.EncodeToString(hash[:]),"live-commentary","v1",hex.EncodeToString(eventHash[:])+".json")
}

func generateLivePCM(ctx context.Context,text,voice string) ([]byte,error) {
 endpoint:=strings.TrimSpace(os.Getenv("POKEPILOT_LIVE_TTS_URL"))
 model:=strings.TrimSpace(os.Getenv("POKEPILOT_LIVE_TTS_MODEL"))
 u,err:=url.Parse(endpoint)
 if err!=nil || u.Host=="" || (u.Scheme!="https"&&u.Scheme!="http") || u.User!=nil || u.Fragment!="" || u.RawQuery!="" {return nil,errors.New("invalid TTS endpoint")}
 if model=="" {return nil,errors.New("TTS model not configured")}
 if voice=="" {voice=strings.TrimSpace(os.Getenv("POKEPILOT_LIVE_TTS_VOICE"))}
 if voice=="" {voice="alloy"}
 payload,err:=json.Marshal(map[string]string{
  "model":model,"voice":voice,"input":text,"response_format":"pcm",
 })
 if err!=nil{return nil,err}
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,u.String(),bytes.NewReader(payload))
 if err!=nil{return nil,err}
 req.Header.Set("Content-Type","application/json")
 if key:=os.Getenv("POKEPILOT_LIVE_TTS_API_KEY");key!="" {req.Header.Set("Authorization","Bearer "+key)}
 client:=&http.Client{Timeout:12*time.Second}
 res,err:=client.Do(req)
 if err!=nil{return nil,err}
 defer res.Body.Close()
 if res.StatusCode<200||res.StatusCode>=300 {return nil,fmt.Errorf("TTS provider returned %s",res.Status)}
 pcm,err:=io.ReadAll(io.LimitReader(res.Body,maxTTSBytes+1))
 if err!=nil{return nil,err}
 if len(pcm)==0||len(pcm)>maxTTSBytes||len(pcm)%2!=0 {return nil,errors.New("invalid TTS PCM length")}
 return pcm,nil
}

func (s *replayServer) handleLiveCommentaryStatus(w http.ResponseWriter,r *http.Request) {
 runID:=strings.TrimSpace(r.PathValue("id"))
 if runID=="" {writeJSON(w,http.StatusBadRequest,map[string]string{"error":"missing run id"});return}
 if session:=s.commentarySessionForRun(runID);session!=nil {
  writeJSON(w,http.StatusOK,session.snapshot());return
 }
 writeJSON(w,http.StatusOK,liveCommentaryStatus{
  RunID:runID,State:"stopped",Settings:commentator.DefaultSettings(),TTSConfigured:liveTTSConfigured(),Model:"rules:"+commentator.PolicyVersion,
 })
}

type liveCommentaryStartRequest struct {
  commentator.Settings
  History []producer.HistoricalRun `json:"history,omitempty"`
}

// History is accepted only from the private operator and must carry explicit
// durable-source provenance, mirroring the post-run producer's contract.
// These are prior *completed* runs with the exact same goal, not model claims.
func verifiedLiveHistory(currentRunID,goal string,history []producer.HistoricalRun) ([]string,error) {
 if len(history)>4 {return nil,errors.New("at most four historical runs are supported")}
 out:=make([]string,0,len(history))
 seen:=make(map[string]bool)
 for _,h:=range history {
  if h.RunID=="" || h.RunID==currentRunID || h.Source=="" ||
   strings.TrimSpace(h.Goal)!=strings.TrimSpace(goal) || h.Status!="done" ||
   h.DurationMS<=0 || h.DurationMS>365*24*60*60*1000 || seen[h.RunID] ||
   len(h.RunID)>128 || len(h.Source)>128 || len(h.Goal)>180 {
   return nil,errors.New("history must identify distinct completed runs with sourced statistics and the same goal")
  }
  seen[h.RunID]=true
  out=append(out,fmt.Sprintf("Prior run %s (%s) completed the same goal in %d ms",h.RunID,h.Source,h.DurationMS))
 }
 return out,nil
}

func (s *replayServer) handleLiveCommentaryStart(w http.ResponseWriter,r *http.Request) {
 runID:=strings.TrimSpace(r.PathValue("id"))
 if runID=="" {writeJSON(w,http.StatusBadRequest,map[string]string{"error":"missing run id"});return}
 var req liveCommentaryStartRequest
 decoder:=json.NewDecoder(io.LimitReader(r.Body,maxCommentaryRequest+1))
 decoder.DisallowUnknownFields()
 if err:=decoder.Decode(&req);err!=nil {writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid commentary settings"});return}
 if err:=decoder.Decode(new(any));err!=io.EOF {writeJSON(w,http.StatusBadRequest,map[string]string{"error":"trailing or oversized commentary settings"});return}
 if !req.Enabled {writeJSON(w,http.StatusBadRequest,map[string]string{"error":"enabled must be true; use /stop to disable"});return}
 if err:=req.Settings.Validate();err!=nil {writeJSON(w,http.StatusBadRequest,map[string]string{"error":err.Error()});return}
 model,err:=configuredLiveCommentaryModel()
 if err!=nil {writeJSON(w,http.StatusServiceUnavailable,map[string]string{"error":err.Error()});return}
 run,err:=s.fetchLiveRun(r.Context(),runID)
 if err!=nil {writeReplayError(w,err);return}
 if !strings.EqualFold(run.Status,"running") {writeJSON(w,http.StatusConflict,map[string]string{"error":"commentary requires an active run"});return}
 historyFacts,err:=verifiedLiveHistory(runID,run.Goal,req.History)
 if err!=nil {writeJSON(w,http.StatusBadRequest,map[string]string{"error":err.Error()});return}
 engine,err:=commentator.New(req.Settings,model)
 if err!=nil {writeJSON(w,http.StatusBadRequest,map[string]string{"error":err.Error()});return}
 s.commentaryMu.Lock()
 if prior:=s.commentarySessions[runID];prior!=nil {
  select {case <-prior.done:default:
   s.commentaryMu.Unlock()
   writeJSON(w,http.StatusConflict,map[string]string{"error":"commentary already active; stop before updating settings"})
   return
  }
 }
 ctx,cancel:=context.WithCancel(context.Background())
 session:=&liveCommentarySession{
  server:s,runID:runID,engine:engine,settings:req.Settings,historyFacts:historyFacts,cancel:cancel,done:make(chan struct{}),
  state:"live",audio:make(chan []byte,2),
 }
 s.commentarySessions[runID]=session
 s.commentaryMu.Unlock()
 go session.run(ctx)
 writeJSON(w,http.StatusAccepted,session.snapshot())
}

func (s *replayServer) handleLiveCommentaryStop(w http.ResponseWriter,r *http.Request) {
 runID:=strings.TrimSpace(r.PathValue("id"))
 if runID=="" {writeJSON(w,http.StatusBadRequest,map[string]string{"error":"missing run id"});return}
 session:=s.commentarySessionForRun(runID)
 if session==nil {s.handleLiveCommentaryStatus(w,r);return}
 session.mu.Lock();session.state="stopping";session.mu.Unlock()
 session.cancel()
 writeJSON(w,http.StatusOK,session.snapshot())
}
