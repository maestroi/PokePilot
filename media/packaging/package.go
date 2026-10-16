// Package packaging produces evidence-grounded, versioned editorial metadata.
// A model may SELECT a story angle and a real event, never author unverifiable facts.
package packaging

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/media/editplan"
	"github.com/maestroi/pokepilot/media/producer"
)

const (
	SchemaVersion = 1
	TemplateVersion = "grounded-templates-v1"
	PromptVersion = "angle-selection-v1"
	MaxModelBytes = 32 << 10
)

type Style string

const (
	Neutral Style = "neutral"
	Documentary Style = "documentary"
	Energetic Style = "energetic"
)
type Angle string

const (
	Moment Angle = "moment"
	Challenge Angle = "challenge"
	Recap Angle = "recap"
)

// Selection is the only creative instruction accepted from a model. It cites
// one EXACT semantic event and an allowlisted narrative angle.
type Selection struct {
	Attempt int `json:"attempt"`
	EventID string `json:"event_id"`
	Angle Angle `json:"angle"`
}
type Fact struct {
	Attempt int `json:"attempt"`
	EventID string `json:"event_id"`
	Kind string `json:"kind"`
	Label string `json:"label"`
	TimestampMS int64 `json:"timestamp_ms"`
}
type ModelContext struct {
	Goal string `json:"goal"`
	Result string `json:"result"`
	Style Style `json:"style"`
	Events []Fact `json:"events"`
}
type Model interface {
	Identity() string
	Select(context.Context, ModelContext) ([]byte,error)
}
type Request struct {
	Assessment producer.Assessment
	Timelines []farm.MediaTimeline
	Plan *editplan.Plan
	Style Style
	RunBaseURL string
	Model Model
}
type TitleCandidate struct {
	ID string `json:"id"`
	Title string `json:"title"`
	Angle Angle `json:"angle"`
	Evidence producer.EvidenceRef `json:"evidence"`
}
type Chapter struct {
	StartMS int64 `json:"start_ms"`
	Title string `json:"title"`
	Evidence producer.EvidenceRef `json:"evidence"`
}
type ShortCandidate struct {
	ID string `json:"id"`
	Title string `json:"title"`
	Hook string `json:"hook"`
	Caption string `json:"caption"`
	Evidence producer.EvidenceRef `json:"evidence"`
}
type ThumbnailConcept struct {
	ID string `json:"id"`
	TitleID string `json:"title_id"`
	Subject string `json:"subject"`
	Composition string `json:"composition"`
	Emphasis string `json:"emphasis"`
	Text string `json:"text"`
	Evidence producer.EvidenceRef `json:"evidence"`
}
type Package struct {
	Version int `json:"version"`
	RunID string `json:"run_id"`
	AssessmentHash string `json:"assessment_hash"`
	PlanHash string `json:"plan_hash,omitempty"`
	TimelineSHA256 string `json:"timeline_sha256"`
	Model string `json:"model"`
	PromptVersion string `json:"prompt_version"`
	TemplateVersion string `json:"template_version"`
	Style Style `json:"style"`
	Selections []Selection `json:"selections"`
	Titles []TitleCandidate `json:"titles"`
	Description string `json:"description"`
	Chapters []Chapter `json:"chapters,omitempty"`
	Shorts []ShortCandidate `json:"shorts"`
	Thumbnails []ThumbnailConcept `json:"thumbnail_concepts"`
	Hash string `json:"hash"`
}

func normalizeStyle(style Style) (Style,error) {
	if style=="" {style=Documentary}
	if style!=Neutral && style!=Documentary && style!=Energetic {return "",errors.New("packaging: unsupported channel style")}
	return style,nil
}

// cleanFacts accepts ordinary public run labels but drops text that resembles
// credentials or connection strings. It never passes private model prompts.
func cleanFacts(value string, limit int) string {
	value=strings.TrimSpace(value)
	lower:=strings.ToLower(value)
	for _,bad:=range []string{"api_key","apikey","password","secret","bearer ","sk-","token=","authorization:","://","-----begin"} {
		if strings.Contains(lower,bad) {return ""}
	}
	var out []rune
	for _,r:=range value {
		if unicode.IsControl(r) {r=' '}
		if unicode.IsPrint(r) {out=append(out,r)}
		if len(out)>=limit {break}
	}
	return strings.Join(strings.Fields(string(out))," ")
}
func label(kind string) string {
	switch kind {
	case "badge_acquired":return "Badge earned"
	case "gym_battle":return "Gym battle"
	case "boss_battle":return "Boss battle"
	case "run_finished":return "Run completed"
	case "run_failed","failure_terminal":return "Run ended"
	case "blackout":return "Blackout"
	case "planner_recovery","failure_recovered":return "Recovery"
	case "rare_catch":return "Rare catch"
	case "important_catch":return "Important catch"
	case "evolution":return "Evolution"
	case "milestone":return "Milestone reached"
	case "objective_failed":return "Objective setback"
	case "planner_stall":return "Planner stall"
	default:return ""
	}
}
func significance(kind string) int {
	switch kind {
	case "run_finished":return 110
	case "boss_battle":return 105
	case "gym_battle":return 100
	case "badge_acquired":return 95
	case "blackout":return 90
	case "run_failed","failure_terminal":return 85
	case "planner_recovery","failure_recovered":return 80
	case "rare_catch","important_catch":return 75
	case "evolution":return 70
	case "milestone":return 65
	default:return 30
	}
}
type source struct {
	facts []Fact
	refs map[string]producer.EvidenceRef
	goal string
	planner string
	result string
	timelineHash string
}
func key(attempt int, eventID string)string{return fmt.Sprintf("%d/%s",attempt,eventID)}

func establish(in Request)(source,error){
	if err:=producer.VerifyChecksum(in.Assessment);err!=nil{return source{},err}
	if in.Assessment.Publication!=producer.FullEpisode && in.Assessment.Publication!=producer.ShortOnly {
		return source{},errors.New("packaging: producer assessment does not authorize publication")
	}
	normalized:=make([]farm.MediaTimeline,0,len(in.Timelines))
	seen:=make(map[int]bool)
	evidence:=make(map[string]farm.MediaEvent)
	sourceRefs:=make(map[string]producer.EvidenceRef)
	out:=source{refs:make(map[string]producer.EvidenceRef)}
	for i,raw:=range in.Timelines {
		t:=raw.Normalized()
		if t.Attempt==0 && len(in.Timelines)==1 {t.Attempt=1}
		if err:=t.Validate();err!=nil{return out,err}
		if seen[t.Attempt]||t.Attempt<1||t.Run.RunID!=in.Assessment.RunID {return out,errors.New("packaging: timeline run/attempt mismatch")}
		seen[t.Attempt]=true
		if out.goal=="" {out.goal=cleanFacts(t.Run.Goal,72)}
		if out.planner=="" {out.planner=cleanFacts(t.Run.Planner,50)}
		if t.Run.Status!="" {out.result=cleanFacts(t.Run.Status,30)}
		normalized=append(normalized,t)
		for _,e:=range t.Events {
			k:=key(t.Attempt,e.ID)
			evidence[k]=e
			sourceRefs[k]=producer.EvidenceRef{Attempt:t.Attempt,EventID:e.ID,Frame:e.Frame,TimestampMS:e.TimestampMS}
		}
		_ = i
	}
	if len(normalized)==0 {return out,errors.New("packaging: semantic timelines required")}
	sort.Slice(normalized,func(i,j int)bool{return normalized[i].Attempt<normalized[j].Attempt})
	raw,err:=json.Marshal(normalized)
	if err!=nil{return out,err}
	sum:=sha256.Sum256(raw)
	out.timelineHash=hex.EncodeToString(sum[:])
	// The producer can select narrative beats, but each proposed fact must
	// match a real recorded event down to its frame and timestamp.
	for _,beat:=range in.Assessment.Beats {
		for _,ref:=range beat.Evidence {
			k:=key(ref.Attempt,ref.EventID)
			actual,ok:=sourceRefs[k]
			if !ok||actual.Frame!=ref.Frame||actual.TimestampMS!=ref.TimestampMS {
				return out,fmt.Errorf("packaging: producer beat cites missing event %s",k)
			}
			event:=evidence[k]
			if name:=label(event.Type);name!="" && out.refs[k].EventID=="" {
				out.refs[k]=ref
				out.facts=append(out.facts,Fact{Attempt:ref.Attempt,EventID:ref.EventID,Kind:event.Type,Label:name,TimestampMS:ref.TimestampMS})
			}
		}
	}
	if len(out.facts)==0{return out,errors.New("packaging: no grounded publishable beats")}
	sort.Slice(out.facts,func(i,j int)bool{
		a,b:=out.facts[i],out.facts[j]
		if a.Attempt!=b.Attempt{return a.Attempt<b.Attempt}
		if a.TimestampMS!=b.TimestampMS{return a.TimestampMS<b.TimestampMS}
		return a.EventID<b.EventID
	})
	if in.Plan!=nil{
		if err:=editplan.Validate(*in.Plan,in.Assessment,normalized);err!=nil{return out,fmt.Errorf("packaging: invalid edit plan: %w",err)}
	}
	return out,nil
}
func validAngle(a Angle)bool{return a==Moment||a==Challenge||a==Recap}

func proposal(raw []byte, facts []Fact) ([]Selection,error) {
	if len(raw)==0||len(raw)>MaxModelBytes{return nil,errors.New("packaging: invalid model response size")}
	var response struct {Selections []Selection `json:"selections"`}
	dec:=json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err:=dec.Decode(&response);err!=nil{return nil,fmt.Errorf("packaging: invalid model schema: %w",err)}
	var trailing any
	if dec.Decode(&trailing)!=io.EOF{return nil,errors.New("packaging: trailing model JSON")}
	if len(response.Selections)<2||len(response.Selections)>3 {return nil,errors.New("packaging: model must select two or three angles")}
	available:=make(map[string]bool)
	for _,f:=range facts {available[key(f.Attempt,f.EventID)]=true}
	used:=make(map[string]bool)
	angles:=make(map[Angle]bool)
	for _,s:=range response.Selections{
		if !available[key(s.Attempt,s.EventID)]||!validAngle(s.Angle){return nil,errors.New("packaging: model selected unverified event or angle")}
		k:=fmt.Sprintf("%s:%s",key(s.Attempt,s.EventID),s.Angle)
		if used[k]{return nil,errors.New("packaging: duplicate model selection")}
		used[k]=true
		angles[s.Angle]=true
	}
	if len(angles)<2{return nil,errors.New("packaging: candidates lack distinct narrative angles")}
	return response.Selections,nil
}
func defaults(facts []Fact) []Selection {
	ranked:=append([]Fact(nil),facts...)
	sort.SliceStable(ranked,func(i,j int)bool{return significance(ranked[i].Kind)>significance(ranked[j].Kind)})
	angles:=[]Angle{Moment,Challenge,Recap}
	selections:=make([]Selection,0,3)
	for i,angle:=range angles {
		f:=ranked[i%len(ranked)]
		// Spread selection across different kinds whenever possible.
		if i>0 {
			for _,candidate:=range ranked {
				already:=false
				for _,prev:=range selections {if prev.Attempt==candidate.Attempt&&prev.EventID==candidate.EventID{already=true;break}}
				if !already {f=candidate;break}
			}
		}
		selections=append(selections,Selection{Attempt:f.Attempt,EventID:f.EventID,Angle:angle})
	}
	return selections
}
func safeGoal(goal string) string {
	if goal=="" {return "autonomous challenge"}
	return goal
}
func safeResult(result string)string{
	switch strings.ToLower(result){
	case "done","completed","success":return "completed"
	case "failed","error","stopped":return "ended without confirmed completion"
	default:return "not independently verified"
	}
}
func limit(value string,max int)string{
	r:=[]rune(value)
	if len(r)>max{return string(r[:max])}
	return value
}
func titleFor(f Fact,angle Angle,style Style,goal string)string{
	g:=safeGoal(goal)
	switch angle {
	case Moment:
		if style==Energetic{return limit(f.Label+"! | "+g+" — PokePilot",96)}
		return limit(f.Label+" | "+g+" — PokePilot",96)
	case Challenge:
		if style==Documentary{return limit("Inside the "+g+" run: "+f.Label,96)}
		return limit(g+" challenge | "+f.Label,96)
	default:
		return limit("PokePilot run recap: "+g+" / "+f.Label,96)
	}
}
func shortFor(f Fact,angle Angle,style Style)(string,string,string){
	title:=f.Label+" | PokePilot #Shorts"
	hook:="A recorded moment: "+f.Label+"."
	switch angle {
	case Challenge:hook="From the autonomous run: "+f.Label+"."
	case Recap:hook="Run recap moment: "+f.Label+"."
	}
	if style==Energetic{hook=f.Label+" — from the recorded run."}
	return limit(title,96),limit(hook,120),limit(f.Label+" • Autonomous gameplay",180)
}
func description(s source,in Request,link string)string{
	lines:=[]string{"PokePilot — recorded autonomous gameplay.","Challenge: "+safeGoal(s.goal),"Result: "+safeResult(s.result)}
	if s.planner!=""{lines=append(lines,"Planner: "+s.planner)}
	lines=append(lines,"")
	lines=append(lines,"Verified moments in this run:")
	seen:=make(map[string]bool)
	for _,fact:=range s.facts{
		if seen[fact.Kind] {continue}
		seen[fact.Kind]=true
		lines=append(lines,"• "+fact.Label)
		if len(seen)>=6 {break}
	}
	if link!=""{lines=append(lines,"","Run page: "+link)}
	lines=append(lines,"","Edited from PokePilot's recorded run and semantic event timeline.")
	return strings.Join(lines,"
")
}
func runLink(base,runID string)(string,error){
	if base=="" {return "",nil}
	u,err:=url.Parse(base)
	if err!=nil||u.Scheme!="https"||u.Host==""||u.User!=nil||u.RawQuery!=""||u.Fragment!="" {
		return "",errors.New("packaging: public run base must be an HTTPS URL without query/credentials")
	}
	u.Path=strings.TrimRight(u.Path,"/")+"/runs/"+url.PathEscape(runID)
	return u.String(),nil
}
func chapters(p *editplan.Plan,s source)[]Chapter{
	if p==nil {return nil}
	var out []Chapter
	var at int64
	seen:=map[string]bool{}
	for _,seg:=range p.Segments{
		switch seg.Action{
		case editplan.Keep:
			for _,id:=range seg.EventIDs{
				k:=key(seg.Attempt,id)
				ref,ok:=s.refs[k]
				if !ok||seen[k]||ref.TimestampMS<seg.StartMS||ref.TimestampMS>seg.EndMS{continue}
				var fact Fact
				for _,f:=range s.facts {if f.Attempt==ref.Attempt&&f.EventID==ref.EventID{fact=f;break}}
				if fact.Label=="" {continue}
				start:=at+int64(math.Round(float64(ref.TimestampMS-seg.StartMS)/float64(seg.Speed)))
				out=append(out,Chapter{StartMS:start,Title:fact.Label,Evidence:ref})
				seen[k]=true
			}
			at+=int64(math.Ceil(float64(seg.EndMS-seg.StartMS)/float64(seg.Speed)))
		case editplan.Card,editplan.Pad:at+=seg.DurationMS
		}
	}
	sort.SliceStable(out,func(i,j int)bool{return out[i].StartMS<out[j].StartMS})
	return out
}
func build(in Request,src source,modelID,prompt string,selections []Selection)(Package,error){
	style,_:=normalizeStyle(in.Style)
	link,err:=runLink(in.RunBaseURL,in.Assessment.RunID)
	if err!=nil{return Package{},err}
	p:=Package{
		Version:SchemaVersion,RunID:in.Assessment.RunID,AssessmentHash:in.Assessment.Hash,
		TimelineSHA256:src.timelineHash,Model:modelID,PromptVersion:prompt,TemplateVersion:TemplateVersion,
		Style:style,Selections:selections,Description:description(src,in,link),Chapters:chapters(in.Plan,src),
	}
	if in.Plan!=nil {p.PlanHash=in.Plan.Hash}
	known:=make(map[string]Fact)
	for _,f:=range src.facts {known[key(f.Attempt,f.EventID)]=f}
	titles:=make(map[string]bool)
	shortTitles:=make(map[string]bool)
	for i,choice:=range selections {
		f,ok:=known[key(choice.Attempt,choice.EventID)]
		if !ok||!validAngle(choice.Angle) {return Package{},errors.New("packaging: unverified selection")}
		ref:=src.refs[key(f.Attempt,f.EventID)]
		title:=titleFor(f,choice.Angle,style,src.goal)
		if titles[strings.ToLower(title)] {return Package{},errors.New("packaging: duplicate titles")}
		titles[strings.ToLower(title)]=true
		id:=fmt.Sprintf("title-%d",i+1)
		p.Titles=append(p.Titles,TitleCandidate{ID:id,Title:title,Angle:choice.Angle,Evidence:ref})
		_,hook,caption:=shortFor(f,choice.Angle,style)
		shortTitle:=limit(fmt.Sprintf("%s — %s | #Shorts",f.Label,choice.Angle),96)
		if shortTitles[strings.ToLower(shortTitle)]{return Package{},errors.New("packaging: duplicate Shorts titles")}
		shortTitles[strings.ToLower(shortTitle)]=true
		p.Shorts=append(p.Shorts,ShortCandidate{ID:fmt.Sprintf("short-%d",i+1),Title:shortTitle,Hook:hook,Caption:caption,Evidence:ref})
		layout:=[]string{"subject-left/text-right","centered-subject","split-scene"}[i%3]
		emphasis:=[]string{"event-first","challenge-first","recap-first"}[i%3]
		p.Thumbnails=append(p.Thumbnails,ThumbnailConcept{
			ID:fmt.Sprintf("concept-%d",i+1),TitleID:id,Subject:f.Label,
			Composition:layout,Emphasis:emphasis,Text:limit(strings.ToUpper(f.Label),28),Evidence:ref,
		})
	}
	if len(p.Titles)<2{return Package{},errors.New("packaging: insufficient independent candidates")}
	return seal(p),nil
}
func seal(p Package)Package{
	p.Hash=""
	data,_:=json.Marshal(p)
	digest:=sha256.Sum256(data)
	p.Hash=hex.EncodeToString(digest[:])
	return p
}
func VerifyChecksum(p Package)error{
	hash:=p.Hash
	p=seal(p)
	if hash!=p.Hash{return errors.New("packaging: artifact checksum mismatch")}
	return nil
}

// Generate creates a review-only package. When configured, the creative model
// chooses among real events/angles. All published text is deterministic code.
func Generate(ctx context.Context,in Request)(Package,error){
	style,err:=normalizeStyle(in.Style)
	if err!=nil{return Package{},err}
	in.Style=style
	src,err:=establish(in)
	if err!=nil{return Package{},err}
	selections:=defaults(src.facts)
	identity,prompt:="rules-v1","none"
	if in.Model!=nil{
		identity=strings.TrimSpace(in.Model.Identity())
		if identity=="" {return Package{},errors.New("packaging: empty model identity")}
		prompt=PromptVersion
		raw,err:=in.Model.Select(ctx,ModelContext{Goal:safeGoal(src.goal),Result:safeResult(src.result),Style:style,Events:src.facts})
		if err!=nil{return Package{},fmt.Errorf("packaging: model selection failed: %w",err)}
		selections,err=proposal(raw,src.facts)
		if err!=nil{return Package{},err}
	}
	return build(in,src,identity,prompt,selections)
}
// Validate recomputes every field from the persisted selections and the
// authoritative assessment, timeline, edit plan, and channel configuration.
func Validate(p Package,in Request)error{
	if p.Version!=SchemaVersion||p.TemplateVersion!=TemplateVersion||p.RunID!=in.Assessment.RunID {
		return errors.New("packaging: version/run mismatch")
	}
	if err:=VerifyChecksum(p);err!=nil{return err}
	style,err:=normalizeStyle(in.Style)
	if err!=nil{return err}
	in.Style=style
	src,err:=establish(in)
	if err!=nil{return err}
	if p.Model==""||(p.Model=="rules-v1"&&p.PromptVersion!="none")||(p.Model!="rules-v1"&&p.PromptVersion!=PromptVersion) {
		return errors.New("packaging: model/prompt identity mismatch")
	}
	if p.Model=="rules-v1"&&!reflect.DeepEqual(p.Selections,defaults(src.facts)){
		return errors.New("packaging: rules selection changed")
	}
	if p.Model!="rules-v1"{
		b,_:=json.Marshal(struct{Selections []Selection `json:"selections"`}{p.Selections})
		if _,err:=proposal(b,src.facts);err!=nil{return err}
	}
	expected,err:=build(in,src,p.Model,p.PromptVersion,p.Selections)
	if err!=nil{return err}
	if !reflect.DeepEqual(expected,p){return errors.New("packaging: derived metadata differs from source evidence")}
	return nil
}
