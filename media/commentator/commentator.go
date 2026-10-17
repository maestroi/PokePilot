// Package commentator makes sparse, evidence-backed live editorial decisions.
// It is deliberately independent of gameplay controllers and model prompts.
package commentator

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "strings"
 "time"

 "github.com/maestroi/pokepilot/farm"
)

const PolicyVersion = "live-v1"
const maxText = 220

type Action string
const (
 Silence Action = "silence"
 Comment Action = "comment"
 Recap Action = "recap"
 Milestone Action = "milestone"
 Comparison Action = "comparison"
)

type Settings struct {
 Enabled bool `json:"enabled"`
 Voice string `json:"voice,omitempty"`
 TTS bool `json:"tts"`
 Style string `json:"style"`
 CadenceSeconds int `json:"cadence_seconds"`
 Verbosity string `json:"verbosity"`
}

func DefaultSettings() Settings {
 return Settings{Enabled:false,Style:"documentary",CadenceSeconds:120,Verbosity:"sparse"}
}
func (s Settings) Validate() error {
 if s.Style!="documentary" && s.Style!="sports" && s.Style!="calm" { return errors.New("style must be documentary, sports, or calm") }
 if s.Verbosity!="sparse" && s.Verbosity!="normal" { return errors.New("verbosity must be sparse or normal") }
 if s.CadenceSeconds<30 || s.CadenceSeconds>3600 { return errors.New("cadence_seconds must be between 30 and 3600") }
 if len(s.Voice)>64 { return errors.New("voice is too long") }
 for _,r:=range s.Voice { if !(r>='a'&&r<='z'||r>='A'&&r<='Z'||r>='0'&&r<='9'||r=='-'||r=='_') { return errors.New("voice contains unsupported characters") } }
 return nil
}

type Context struct {
 RunID string `json:"run_id"`
 Attempt int `json:"attempt"`
 Goal string `json:"goal,omitempty"`
 Objective string `json:"objective,omitempty"`
 Location string `json:"location,omitempty"`
 BadgeCount int `json:"badge_count,omitempty"`
 PartyCount int `json:"party_count,omitempty"`
 Frame uint64 `json:"frame"`
 Recent []farm.MediaEvent `json:"recent_events,omitempty"`
 History []string `json:"verified_history,omitempty"`
}

type Emission struct {
 Policy string `json:"policy"`
 RunID string `json:"run_id"`
 Attempt int `json:"attempt"`
 EventID string `json:"event_id"`
 EventType string `json:"event_type"`
 Frame uint64 `json:"frame"`
 TimestampMS int64 `json:"timestamp_ms"`
 EmittedAt time.Time `json:"emitted_at"`
 Action Action `json:"action"`
 Text string `json:"text"`
 Model string `json:"model"`
 Style string `json:"style"`
 TTSState string `json:"tts_state,omitempty"`
}

type Proposal struct {
 Action Action `json:"action"`
 Text string `json:"text"`
}
type Model interface {
 Identity() string
 Generate(context.Context, Context, farm.MediaEvent, Settings) ([]byte,error)
}

type Engine struct {
 Settings Settings
 Model Model
 Seen map[string]struct{}
 Order []string
 LastAt time.Time
 LastAttempt int
}

func New(settings Settings, model Model) (*Engine,error) {
 if err:=settings.Validate(); err!=nil {return nil,err}
 return &Engine{Settings:settings,Model:model,Seen:make(map[string]struct{})},nil
}

// Priority is grounded entirely in explicit semantic event types. Unknown
// events, navigation and ordinary checkpoints do not merit narration.
func Priority(eventType string) int {
 s:=strings.ToLower(strings.TrimSpace(eventType))
 switch s {
 case "champion","champion_battle","elite_four","elite_four_battle","near_wipe","blackout","run_finished","badge_acquired","evolution","pace_record","personal_best":
  return 3
 case "gym_battle","rival_battle","run_failed","repeated_failure","stall","navigation_stall","recovery","recovery_blackout","recovery_stall","objective_completed":
  return 2
 }
 if strings.HasPrefix(s,"recovery_") {return 2}
 return 0
}

func sanitize(value string, limit int) string {
 value=strings.TrimSpace(strings.Map(func(r rune) rune {
  if r=='\n'||r=='\r'||r=='\t' {return ' '}
  if r<32 || r==127 {return -1}
  return r
 },value))
 if len(value)>limit {value=string([]rune(value)[:min(limit,len([]rune(value)))])}
 return value
}

func validProposal(data []byte) (Proposal,error) {
 if len(data)>4096 {return Proposal{},errors.New("commentator response too large")}
 var p Proposal
 decoder:=json.NewDecoder(strings.NewReader(string(data)))
 decoder.DisallowUnknownFields()
 if err:=decoder.Decode(&p);err!=nil{return p,fmt.Errorf("invalid commentator proposal: %w",err)}
 if err:=decoder.Decode(new(any));err!=io.EOF{return p,errors.New("trailing commentator response")}
 switch p.Action {
 case Silence:
  if strings.TrimSpace(p.Text)!="" {return p,errors.New("silence must not contain text")}
 case Comment,Recap,Milestone,Comparison:
  if p.Text=="" || strings.TrimSpace(p.Text)!=p.Text || len([]rune(p.Text))>maxText {return p,errors.New("commentary text invalid")}
  for _,r:=range p.Text {if r<32||r==127 {return p,errors.New("commentary contains control characters")}}
 default: return p,errors.New("unknown commentary action")
 }
 return p,nil
}

// Consider consumes safe rolling run facts. It never calls gameplay APIs and
// favors silence. A failed/malformed LLM response yields silence, not fallback
// invented speech. Now is supplied so policy is deterministic in unit tests.
func (e *Engine) Consider(ctx context.Context, now time.Time, in Context) (Emission,bool) {
 if !e.Settings.Enabled || in.RunID=="" {return Emission{},false}
 if e.Seen==nil {e.Seen=make(map[string]struct{})}
 if in.Attempt!=e.LastAttempt {
  e.LastAttempt=in.Attempt
  e.Seen=make(map[string]struct{})
  e.Order=nil
 }
 var selected farm.MediaEvent
 best:=0
 for _,event:=range in.Recent {
  if event.ID=="" || event.Frame>in.Frame {continue}
  if _,ok:=e.Seen[event.ID];ok {continue}
  e.markSeen(event.ID)
  priority:=Priority(event.Type)
  if priority>best {selected=event;best=priority}
 }
 if best==0 {return Emission{},false}
 cooldown:=time.Duration(e.Settings.CadenceSeconds)*time.Second
 if e.Settings.Verbosity=="sparse" {cooldown*=2}
 // Critical events bypass the normal cooldown, but never emit back-to-back
 // within five seconds (e.g. badge and evolution in the same poll).
 if !e.LastAt.IsZero() && now.Sub(e.LastAt)<cooldown {
  if best<3 || now.Sub(e.LastAt)<5*time.Second {return Emission{},false}
 }
 proposal:=Proposal{Action:Comment,Text:sanitize(selected.Summary,maxText)}
 if best==3 {proposal.Action=Milestone}
 if proposal.Text=="" {proposal.Text="A major milestone was recorded."}
 modelName:="rules:"+PolicyVersion
 if e.Model!=nil {
  modelName=e.Model.Identity()
  bounded:=Context{
   RunID:in.RunID, Attempt:in.Attempt, Goal:sanitize(in.Goal,180),
   Objective:sanitize(in.Objective,180), Location:sanitize(in.Location,120),
   BadgeCount:in.BadgeCount,PartyCount:in.PartyCount,Frame:in.Frame,
  }
  if n:=len(in.Recent); n>0 {
   start:=0; if n>12 {start=n-12}
   for _,v:=range in.Recent[start:] {
    v.Summary=sanitize(v.Summary,180);v.Evidence="";v.Metadata=nil
    bounded.Recent=append(bounded.Recent,v)
   }
  }
  for i,v:=range in.History {if i>=4 {break};bounded.History=append(bounded.History,sanitize(v,160))}
  data,err:=e.Model.Generate(ctx,bounded,selected,e.Settings)
  if err!=nil {return Emission{},false}
  parsed,err:=validProposal(data)
  if err!=nil || parsed.Action==Silence {return Emission{},false}
  proposal=parsed
 } else if strings.TrimSpace(selected.Summary)=="" {
  // A generic or empty event summary is not worth broadcasting.
  return Emission{},false
 }
 if best<3 && (proposal.Action==Milestone||proposal.Action==Comparison) {return Emission{},false}
 if proposal.Action==Comparison && len(in.History)==0 {return Emission{},false}
 e.LastAt=now
 return Emission{
  Policy:PolicyVersion, RunID:in.RunID, Attempt:in.Attempt, EventID:selected.ID,
  EventType:selected.Type, Frame:selected.Frame, TimestampMS:selected.TimestampMS,
  EmittedAt:now.UTC(), Action:proposal.Action, Text:proposal.Text,
  Model:modelName, Style:e.Settings.Style,
 },true
}

func (e *Engine) markSeen(id string) {
 if _,ok:=e.Seen[id];ok {return}
 e.Seen[id]=struct{}{}
 e.Order=append(e.Order,id)
 if len(e.Order)>512 {delete(e.Seen,e.Order[0]);e.Order=e.Order[1:]}
}
