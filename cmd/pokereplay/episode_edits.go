package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/maestroi/pokepilot/artifactstore"
	"github.com/maestroi/pokepilot/farm"
	mediaartifact "github.com/maestroi/pokepilot/media/artifact"
	"github.com/maestroi/pokepilot/media/editplan"
	"github.com/maestroi/pokepilot/media/producer"
	mediasegment "github.com/maestroi/pokepilot/media/segment"
)

const (
	editRenderMode = "studio-edit"
	editMaxJSONBytes = 128 << 10
)

type editCreateRequest struct {
	AssessmentHash string `json:"assessment_hash"`
	Profile editplan.Profile `json:"profile"`
	Segments []editplan.Segment `json:"segments,omitempty"`
}
type editReply struct {
	Plan editplan.Plan `json:"plan"`
	ObjectKey string `json:"object_key"`
}
type editStatus struct {
	RunID string `json:"run_id"`
	PlanHash string `json:"plan_hash"`
	State string `json:"state"`
	ObjectKey string `json:"object_key,omitempty"`
	ManifestKey string `json:"manifest_key,omitempty"`
	SourceState string `json:"source_state,omitempty"`
	JobID string `json:"job_id,omitempty"`
	Error string `json:"error,omitempty"`
	Size int64 `json:"size,omitempty"`
}
type editManifest struct {
	Version int `json:"version"`
	RunID string `json:"run_id"`
	Plan editplan.Plan `json:"plan"`
	SourceVideo string `json:"source_video"`
	OutputVideo string `json:"output_video"`
	Chapters []mediasegment.EditChapter `json:"chapters,omitempty"`
}

func validEditHash(hash string) bool {
	return len(hash)==64 && strings.Trim(hash,"0123456789abcdef")==""
}

func editPlanKey(runID,hash string) string {
	// Reuse the producer's content-addressed per-run namespace.
	return path.Join(path.Dir(path.Dir(producerObjectKey(runID,hash))),"edits","v1",hash+".json")
}

func (s *replayServer) editTimelines(ctx context.Context,runID string,recordings []replayRecording)([]farm.MediaTimeline,error){
	if len(recordings)==0{return nil,errors.New("studio edit: no replay recordings")}
	out:=make([]farm.MediaTimeline,0,len(recordings))
	for _,rec:=range recordings{
		t,err:=s.mediaTimelineAttempt(ctx,runID,rec.Attempt)
		if err!=nil{return nil,fmt.Errorf("studio edit: attempt %d requires durable semantic timeline: %w",rec.Attempt,err)}
		if t.Attempt==0{t.Attempt=rec.Attempt}
		if t.Attempt!=rec.Attempt{return nil,fmt.Errorf("studio edit: timeline attempt mismatch")}
		out=append(out,t)
	}
	return out,nil
}
func (s *replayServer) loadEditAssessment(ctx context.Context,runID,hash string)(producer.Assessment,error){
	if !validEditHash(hash){return producer.Assessment{},errors.New("studio edit: invalid assessment hash")}
	obj,err:=s.store.GetObject(ctx,producerObjectKey(runID,hash),"")
	if err!=nil{return producer.Assessment{},err}
	defer obj.Body.Close()
	raw,err:=io.ReadAll(io.LimitReader(obj.Body,editMaxJSONBytes+1))
	if err!=nil{return producer.Assessment{},err}
	if len(raw)>editMaxJSONBytes{return producer.Assessment{},errors.New("studio edit: assessment oversized")}
	var a producer.Assessment
	if err:=json.Unmarshal(raw,&a);err!=nil{return a,err}
	if a.RunID!=runID||a.Hash!=hash{return a,errors.New("studio edit: assessment source mismatch")}
	if err:=producer.VerifyChecksum(a);err!=nil{return a,err}
	return a,nil
}
func (s *replayServer) loadEditPlan(ctx context.Context,runID,hash string)(editplan.Plan,error){
	if !validEditHash(hash){return editplan.Plan{},errors.New("studio edit: invalid plan hash")}
	obj,err:=s.store.GetObject(ctx,editPlanKey(runID,hash),"")
	if err!=nil{return editplan.Plan{},err}
	defer obj.Body.Close()
	raw,err:=io.ReadAll(io.LimitReader(obj.Body,editMaxJSONBytes+1))
	if err!=nil{return editplan.Plan{},err}
	if len(raw)>editMaxJSONBytes{return editplan.Plan{},errors.New("studio edit: stored plan oversized")}
	var p editplan.Plan
	if err:=json.Unmarshal(raw,&p);err!=nil{return p,err}
	if p.RunID!=runID||p.Hash!=hash{return p,errors.New("studio edit: stored identity mismatch")}
	return p,nil
}

func (s *replayServer) editVideoKey(runID string,recordings []replayRecording,p editplan.Plan) string {
	profile:=s.replayProfileForMode(replayModeBroadcast,replaySegmentFrames())
	profile.Name=mediaartifact.ProfileHighlight
	if p.Profile==editplan.Short{profile.Name=mediaartifact.ProfileShortVertical}
	profile.Video.Width,profile.Video.Height=1280,720
	profile.Video.Aspect="16:9"
	if p.Profile==editplan.Short{profile.Video.Width,profile.Video.Height,profile.Video.Aspect=1080,1920,"9:16"}
	profile.Video.FPSNumerator,profile.Video.FPSDenominator=30,1
	profile.Encoder=mediaartifact.Encoder{Codec:"h264",Policy:mediasegment.EditEncoderVersion}
	profile.EventPolicy="studio-edit-events-v1"
	profile.OverlayPolicy="studio-edit-overlays-v1"
	profile.AudioPolicy="none-cues-only"
	profile.SegmentPolicy=mediaartifact.Component{ID:"typed-edit",Version:editplan.RenderVersion}
	profile.EditPlan=&mediaartifact.Content{Version:fmt.Sprintf("editplan-v%d",p.Version),SHA256:p.Hash}
	input:=mediaartifact.IdentityInput{Profile:mediaartifact.NormalizeProfile(profile)}
	for _,rec:=range recordings{
		input.Sources=append(input.Sources,mediaartifact.Source{Attempt:rec.Attempt,SHA256:rec.Artifact.SHA256,FallbackID:rec.Artifact.ObjectKey})
		input.Timelines=append(input.Timelines,mediaartifact.Content{
			Attempt:rec.Attempt,Version:fmt.Sprintf("media-timeline-v%d",farm.MediaTimelineVersion),
			SHA256:rec.Timeline.SHA256,FallbackID:rec.Timeline.ObjectKey,
		})
	}
	token:=mediaartifact.Token(input)
	dir:=path.Join(path.Dir(s.replayCacheKeyForMode(runID,recordings,replayModeBroadcast)),"studio","edits")
	return path.Join(dir,fmt.Sprintf("edit-%s-%s.mp4",p.Hash,token))
}

func (s *replayServer) currentEditStatus(ctx context.Context,runID string,recordings []replayRecording,p editplan.Plan)editStatus{
	key:=s.editVideoKey(runID,recordings,p)
	st:=editStatus{RunID:runID,PlanHash:p.Hash,ObjectKey:key,ManifestKey:strings.TrimSuffix(key,".mp4")+".json",JobID:farm.MediaRenderJobID(key)}
	if s.store==nil{st.State="disabled";return st}
	if obj,err:=s.store.HeadObject(ctx,key);err==nil{
		if _,manifestErr:=s.store.HeadObject(ctx,st.ManifestKey);manifestErr==nil{
			st.State,st.Size="ready",obj.Size;return st
		}
	}else if !artifactstore.IsNotFound(err){st.State="error";st.Error=err.Error();return st}
	if job,ok,err:=s.getRenderJob(ctx,st.JobID);err==nil&&ok{
		st.State=job.State
		if job.State==farm.MediaRenderJobRunning||job.Active(){st.State="generating"}
		st.Error=job.Error
		return st
	}
	s.mu.Lock()
	local,ok:=s.jobs[key]
	s.mu.Unlock()
	if ok{st.State,st.Error,st.Size=local.State,local.Error,local.Size;return st}
	source:=s.replayStatus(ctx,runID,recordings,replayModeBroadcast)
	st.SourceState=source.State
	if source.State!="ready"||source.LegacyIdentity {st.State="source_missing"}else{st.State="missing"}
	return st
}

func (s *replayServer) loadEditInputs(ctx context.Context,runID,hash string)(editplan.Plan,producer.Assessment,[]replayRecording,error){
	var empty editplan.Plan
	recordings,missing,err:=s.recordingsWithGaps(ctx,runID)
	if err!=nil{return empty,producer.Assessment{},nil,err}
	if len(missing)>0{return empty,producer.Assessment{},nil,errors.New("studio edit: recording attempts missing")}
	p,err:=s.loadEditPlan(ctx,runID,hash)
	if err!=nil{return empty,producer.Assessment{},nil,err}
	a,err:=s.loadEditAssessment(ctx,runID,p.AssessmentHash)
	if err!=nil{return empty,producer.Assessment{},nil,err}
	timelines,err:=s.editTimelines(ctx,runID,recordings)
	if err!=nil{return empty,producer.Assessment{},nil,err}
	if err:=editplan.Validate(p,a,timelines);err!=nil{return empty,producer.Assessment{},nil,err}
	return p,a,recordings,nil
}

// POST creates a deterministic draft from a producer assessment, or accepts
// typed editor-authored segments. Server-owned source fields are never
// accepted from a model/client.
func (s *replayServer) handleEditCreate(w http.ResponseWriter,r *http.Request){
	runID:=r.PathValue("id")
	if s.store==nil{writeJSON(w,http.StatusServiceUnavailable,map[string]string{"error":"studio storage disabled"});return}
	if r.ContentLength>editMaxJSONBytes{writeJSON(w,http.StatusRequestEntityTooLarge,map[string]string{"error":"plan request oversized"});return}
	var req editCreateRequest
	decoder:=json.NewDecoder(io.LimitReader(r.Body,editMaxJSONBytes+1))
	decoder.DisallowUnknownFields()
	if err:=decoder.Decode(&req);err!=nil {writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid plan request"});return}
	if decoder.Decode(new(any))!=io.EOF{writeJSON(w,http.StatusBadRequest,map[string]string{"error":"extra plan fields or oversized body"});return}
	recordings,missing,err:=s.recordingsWithGaps(r.Context(),runID)
	if err!=nil{writeReplayError(w,err);return}
	if len(missing)>0{writeJSON(w,http.StatusConflict,map[string]string{"error":"studio edits require complete replay attempts"});return}
	a,err:=s.loadEditAssessment(r.Context(),runID,req.AssessmentHash)
	if err!=nil{writeJSON(w,http.StatusNotFound,map[string]string{"error":"producer assessment unavailable or invalid"});return}
	timelines,err:=s.editTimelines(r.Context(),runID,recordings)
	if err!=nil{writeJSON(w,http.StatusConflict,map[string]string{"error":err.Error()});return}
	var p editplan.Plan
	if req.Segments==nil{
		p,err=editplan.Build(a,timelines,req.Profile)
	}else{
		p,err=editplan.Initialize(runID,a,req.Profile,timelines)
		if err==nil{p.Segments=req.Segments;p=editplan.Seal(p)}
	}
	if err==nil{err=editplan.Validate(p,a,timelines)}
	if err!=nil{writeJSON(w,http.StatusUnprocessableEntity,map[string]string{"error":err.Error()});return}
	data,err:=json.Marshal(p)
	if err!=nil{writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"encode edit plan"});return}
	key:=editPlanKey(runID,p.Hash)
	if _,err:=s.store.PutObject(r.Context(),key,"application/json",data);err!=nil{
		writeJSON(w,http.StatusBadGateway,map[string]string{"error":"persist edit plan failed"});return
	}
	writeJSON(w,http.StatusOK,editReply{Plan:p,ObjectKey:key})
}
func (s *replayServer) handleEditPlan(w http.ResponseWriter,r *http.Request){
	if s.store==nil{writeJSON(w,http.StatusServiceUnavailable,map[string]string{"error":"studio disabled"});return}
	p,_,_,err:=s.loadEditInputs(r.Context(),r.PathValue("id"),r.PathValue("hash"))
	if err!=nil{writeJSON(w,http.StatusNotFound,map[string]string{"error":err.Error()});return}
	writeJSON(w,http.StatusOK,p)
}
func (s *replayServer) handleEditStatus(w http.ResponseWriter,r *http.Request){
	if s.store==nil{writeJSON(w,http.StatusServiceUnavailable,map[string]string{"error":"studio disabled"});return}
	p,_,recs,err:=s.loadEditInputs(r.Context(),r.PathValue("id"),r.PathValue("hash"))
	if err!=nil{writeJSON(w,http.StatusNotFound,map[string]string{"error":err.Error()});return}
	writeJSON(w,http.StatusOK,s.currentEditStatus(r.Context(),p.RunID,recs,p))
}
func (s *replayServer) handleEditVideo(w http.ResponseWriter,r *http.Request){
	if s.store==nil{writeJSON(w,http.StatusServiceUnavailable,map[string]string{"error":"studio disabled"});return}
	p,_,recs,err:=s.loadEditInputs(r.Context(),r.PathValue("id"),r.PathValue("hash"))
	if err!=nil{writeJSON(w,http.StatusNotFound,map[string]string{"error":err.Error()});return}
	st:=s.currentEditStatus(r.Context(),p.RunID,recs,p)
	if st.State!="ready"{writeJSON(w,http.StatusConflict,st);return}
	obj,err:=s.store.GetObject(r.Context(),st.ObjectKey,"")
	if err!=nil{writeJSON(w,http.StatusBadGateway,map[string]string{"error":"edit video unavailable"});return}
	defer obj.Body.Close()
	copyObjectResponse(w,obj,"video/mp4","episode.mp4")
}
func (s *replayServer) handleEditRender(w http.ResponseWriter,r *http.Request){
	if s.store==nil{writeJSON(w,http.StatusServiceUnavailable,map[string]string{"error":"studio disabled"});return}
	p,_,recordings,err:=s.loadEditInputs(r.Context(),r.PathValue("id"),r.PathValue("hash"))
	if err!=nil{writeJSON(w,http.StatusUnprocessableEntity,map[string]string{"error":err.Error()});return}
	runID:=p.RunID
	st:=s.currentEditStatus(r.Context(),runID,recordings,p)
	if st.State=="ready"{writeJSON(w,http.StatusOK,st);return}
	source:=s.replayStatus(r.Context(),runID,recordings,replayModeBroadcast)
	if source.State!="ready"||source.LegacyIdentity {
		st.State="source_missing"
		st.Error="Render the full broadcast replay first; studio edits never re-run gameplay"
		writeJSON(w,http.StatusConflict,st);return
	}
	key:=st.ObjectKey
	job,jobErr:=s.ensureRenderJobMode(r.Context(),runID,recordings,editRenderMode,key)
	if jobErr==nil{
		if job.State==farm.MediaRenderJobFailed||job.State==farm.MediaRenderJobCancelled {
			if job.State==farm.MediaRenderJobFailed&&!job.Retryable(){
				st.State="error";st.Error="nonretryable render failure";writeJSON(w,http.StatusConflict,st);return
			}
			job,err=s.retryRenderJob(r.Context(),job.ID)
			if err!=nil{writeJSON(w,http.StatusBadGateway,map[string]string{"error":err.Error()});return}
		}else if job.Active(){st.State="generating";writeJSON(w,http.StatusAccepted,st);return}
	}
	if jobErr!=nil&&!errors.Is(jobErr,errMediaRenderJobAPIUnavailable){
		writeJSON(w,http.StatusBadGateway,map[string]string{"error":jobErr.Error()});return
	}
	controlID:=key
	if jobErr==nil{controlID=job.ID}
	release,reason,detail:=s.tryAdmitRenderJob(controlID)
	if release==nil {st.State="generating";st.Error="queued_"+reason+": "+detail;writeJSON(w,http.StatusAccepted,st);return}
	jobID:=""
	if jobErr==nil{
		claimed,ok,err:=s.claimRenderJob(r.Context(),job.ID)
		if err!=nil{release();writeJSON(w,http.StatusBadGateway,map[string]string{"error":err.Error()});return}
		if !ok{release();st.State="generating";writeJSON(w,http.StatusAccepted,st);return}
		jobID=claimed.ID
	}
	s.setJob(key,replayStatus{RunID:runID,JobID:jobID,State:"generating",ObjectKey:key,Stage:editRenderMode})
	go func(){defer release();s.renderEdit(jobID,runID,recordings,p,key)}()
	st.State="generating"
	writeJSON(w,http.StatusAccepted,st)
}

func (s *replayServer) renderEdit(jobID,runID string,recordings []replayRecording,p editplan.Plan,key string){
	s.rendering.Add(1)
	defer s.rendering.Add(-1)
	ctx,cancel:=withReplayTimeout(context.Background(),s.capacity.JobTimeout)
	defer cancel()
	var cancelled atomic.Bool
	cancelForControl:=func(){cancelled.Store(true);cancel()}
	controlID:=key
	if jobID!=""{controlID=jobID}
	s.registerRenderCancel(controlID,cancelForControl)
	defer s.unregisterRenderCancel(controlID)
	stopLease:=s.keepRenderJobLease(ctx,jobID,cancelForControl)
	defer stopLease()
	fail:=func(err error){
		class:=classifyRenderFailure(err,ctx.Err())
		if cancelled.Load(){class=farm.MediaRenderFailureCancelled}
		s.renderFailures.Add(1)
		s.setJob(key,replayStatus{RunID:runID,JobID:jobID,State:"error",ObjectKey:key,Error:clipError(err),FailureClass:class,Stage:farm.MediaRenderJobFailed})
		if jobID!=""{
			_ =s.finishRenderJob(context.Background(),jobID,farm.MediaRenderJobFailed,farm.MediaRenderJobFailed,clipError(err),0,class)
		}
	}
	dir,err:=os.MkdirTemp(s.capacity.ScratchDir,replayScratchJobPrefix+"edit-")
	if err!=nil{fail(err);return}
	defer os.RemoveAll(dir)
	sourceKey:=s.replayCacheKeyForMode(runID,recordings,replayModeBroadcast)
	if _,err:=s.store.HeadObject(ctx,sourceKey);err!=nil{fail(fmt.Errorf("source replay missing: %w",err));return}
	url,err:=s.store.PresignGetObject(sourceKey,replaySegmentURLTTL)
	if err!=nil{fail(err);return}
	dest:=pathJoinOS(dir,"episode.mp4")
	s.encoderProcesses.Add(1)
	result,err:=mediasegment.RenderEdit(ctx,url,p,dir,dest,nil)
	s.encoderProcesses.Add(-1)
	if err!=nil{fail(err);return}
	if err:=probeReplayVideo(ctx,dest,time.Duration(p.DurationMS)*time.Millisecond);err!=nil{fail(fmt.Errorf("validate episode: %w",err));return}
	f,err:=os.Open(dest)
	if err!=nil{fail(err);return}
	obj,err:=s.store.PutObjectReader(ctx,key,"video/mp4",f)
	_ = f.Close()
	if err!=nil{fail(err);return}
	manifest:=editManifest{Version:1,RunID:runID,Plan:p,SourceVideo:sourceKey,OutputVideo:key,Chapters:result.Chapters}
	raw,err:=json.Marshal(manifest)
	if err!=nil{fail(err);return}
	manifestKey:=strings.TrimSuffix(key,".mp4")+".json"
	if _,err:=s.store.PutObject(ctx,manifestKey,"application/json",raw);err!=nil{fail(err);return}
	s.setJob(key,replayStatus{RunID:runID,JobID:jobID,State:"ready",ObjectKey:key,Size:obj.Size,Stage:farm.MediaRenderJobReady})
	if jobID!=""{
		if err:=s.finishRenderJob(context.Background(),jobID,farm.MediaRenderJobReady,farm.MediaRenderJobReady,"",obj.Size);err!=nil{
			log.Printf("pokereplay: finish studio edit job: %v",err)
		}
	}
}

// Render-job recovery is deliberately source-identity guarded: a queued edit
// cannot silently switch to a different replay/policy after worker restart.
func (s *replayServer) recoverEditRenderJob(ctx context.Context,job farm.MediaRenderJob){
	file:=path.Base(job.ArtifactKey)
	if !strings.HasPrefix(file,"edit-") || len(file)<69 {
		return
	}
	hash:=strings.TrimPrefix(file,"edit-")[:64]
	if !validEditHash(hash){return}
	p,_,recordings,err:=s.loadEditInputs(ctx,job.RunID,hash)
	if err!=nil{return}
	key:=s.editVideoKey(job.RunID,recordings,p)
	if key!=job.ArtifactKey{return}
	source:=s.replayStatus(ctx,job.RunID,recordings,replayModeBroadcast)
	if source.State!="ready"||source.LegacyIdentity{return}
	release,_,_:=s.tryAdmitRenderJob(job.ID)
	if release==nil{return}
	claimed,ok,err:=s.claimRenderJob(ctx,job.ID)
	if err!=nil||!ok{release();return}
	go func(){defer release();s.renderEdit(claimed.ID,job.RunID,recordings,p,key)}()
}

var _ = strconv.IntSize
var _ = bytes.MinRead
