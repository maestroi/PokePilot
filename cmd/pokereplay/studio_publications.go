package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "path"
 "strings"
 "time"

 "github.com/maestroi/pokepilot/artifactstore"
 "github.com/maestroi/pokepilot/media/packaging"
 "github.com/maestroi/pokepilot/media/publication"
 "github.com/maestroi/pokepilot/media/producer"
)

const studioPublicationMaxJSON = 64 << 10
const studioChannelThrottleKey = "publications/studio/channel-throttle.json"

type studioPublicationIndex struct {
 Version int `json:"version"`
 RunID string `json:"run_id"`
 IDs []string `json:"ids"`
}
type studioPublicationRequest struct {
 AssessmentHash string `json:"assessment_hash"`
 PackageHash string `json:"package_hash,omitempty"`
 Format publication.Format `json:"format"`
 TitleID string `json:"title_id,omitempty"`
 ThumbnailID string `json:"thumbnail_id,omitempty"`
 Privacy string `json:"privacy,omitempty"`
 NotBefore int64 `json:"not_before_unix,omitempty"`
}
type studioChannelThrottle struct {
 LastStarted int64 `json:"last_started_unix"`
 CandidateID string `json:"candidate_id"`
}
type studioPublicationList struct{
 Candidates []publication.Candidate `json:"candidates"`
}

func studioPublicationRoot(runID string)string {
 return path.Join("publications","studio",sanitizeKeySegment(runID))
}
func studioPublicationKey(runID,id string)string{
 return path.Join(studioPublicationRoot(runID),"candidates",id+".json")
}
func studioPublicationIndexKey(runID string)string{
 return path.Join(studioPublicationRoot(runID),"index.json")
}
func studioYouTubeID(id string)string{return "studio-"+id}
func (s *replayServer) loadPublicationIndex(ctx context.Context,runID string)(studioPublicationIndex,error){
 idx:=studioPublicationIndex{Version:1,RunID:runID,IDs:[]string{}}
 obj,err:=s.store.GetObject(ctx,studioPublicationIndexKey(runID),"")
 if artifactstore.IsNotFound(err){return idx,nil}
 if err!=nil{return idx,err}
 defer obj.Body.Close()
 data,err:=io.ReadAll(io.LimitReader(obj.Body,studioPublicationMaxJSON+1))
 if err!=nil||len(data)>studioPublicationMaxJSON{return idx,errors.New("publication: index unreadable")}
 if err:=json.Unmarshal(data,&idx);err!=nil{return idx,err}
 if idx.Version!=1||idx.RunID!=runID||len(idx.IDs)>1000{return idx,errors.New("publication: invalid index")}
 for _,id:=range idx.IDs{if !validEditHash(id){return idx,errors.New("publication: invalid index candidate")}}
 return idx,nil
}
func (s *replayServer) loadPublication(ctx context.Context,runID,id string)(publication.Candidate,error){
 var c publication.Candidate
 if !validEditHash(id)||runID==""{return c,errors.New("publication: invalid candidate identity")}
 obj,err:=s.store.GetObject(ctx,studioPublicationKey(runID,id),"")
 if err!=nil{return c,err}
 defer obj.Body.Close()
 data,err:=io.ReadAll(io.LimitReader(obj.Body,studioPublicationMaxJSON+1))
 if err!=nil||len(data)>studioPublicationMaxJSON{return c,errors.New("publication: stored candidate oversized")}
 if err:=json.Unmarshal(data,&c);err!=nil{return c,err}
 if c.Proposal.RunID!=runID||c.ID!=id{return c,errors.New("publication: candidate differs from run")}
 return c,c.Validate()
}
func (s *replayServer) savePublication(ctx context.Context,c publication.Candidate)error{
 if err:=c.Validate();err!=nil{return err}
 data,err:=json.Marshal(c)
 if err!=nil{return err}
 _,err=s.store.PutObject(ctx,studioPublicationKey(c.Proposal.RunID,c.ID),"application/json",data)
 return err
}
func (s *replayServer) savePublicationIndex(ctx context.Context,idx studioPublicationIndex)error{
 raw,err:=json.Marshal(idx)
 if err!=nil{return err}
 _,err=s.store.PutObject(ctx,studioPublicationIndexKey(idx.RunID),"application/json",raw)
 return err
}

// studioQualityPolicy is deliberately server-owned. A user request cannot
// override the producer classification or lower the review gates.
func studioQualityPolicy() publication.Policy {
 p:=publication.DefaultPolicy()
 return p
}
func (s *replayServer) currentPublicationInputs(ctx context.Context,c publication.Candidate)(publication.Inputs,error){
 p:=c.Proposal
 a,err:=s.loadEditAssessment(ctx,p.RunID,p.AssessmentHash)
 if err!=nil{return publication.Inputs{},err}
 in:=publication.Inputs{Assessment:a}
 if p.Format==publication.Archive{return in,nil}
 pkg,err:=s.loadVerifiedThumbnailPackage(ctx,p.RunID,p.PackageHash)
 if err!=nil{return in,err}
 in.Package=&pkg
 plan,assessment,recordings,err:=s.loadEditInputs(ctx,p.RunID,p.PlanHash)
 if err!=nil{return in,err}
 if assessment.Hash!=a.Hash{return in,errors.New("publication: edit source assessment changed")}
 in.Plan=&plan
 st:=s.currentEditStatus(ctx,p.RunID,recordings,plan)
 if st.State=="ready"{
  in.VideoReady=true;in.VideoSize=st.Size;in.VideoKey=st.ObjectKey
 }
 if p.ThumbnailID!=""{
  manifest,err:=s.getThumbnailManifest(ctx,p.RunID,pkg)
  if err!=nil{return in,err}
  selected,err:=s.getThumbnailSelection(ctx,p.RunID,manifest)
  if err!=nil{return in,err}
  in.ThumbnailID=selected
  for _,candidate:=range manifest.Candidates{
   if candidate.ID==selected&&candidate.ID==p.ThumbnailID{
    in.ThumbnailKey=candidate.ObjectKey
    if _,err:=s.store.HeadObject(ctx,candidate.ObjectKey);err!=nil{return in,err}
    break
   }
  }
 }
 return in,nil
}
func (s *replayServer) publicationStatus(ctx context.Context,c publication.Candidate)publication.Candidate{
 if !c.Approved {return c}
 if c.State!=publication.Publishing&&c.State!=publication.Failed&&c.State!=publication.Published {return c}
 rec,ok,err:=s.loadYouTubePublication(ctx,studioYouTubeID(c.ID))
 if err!=nil||!ok{return c}
 switch rec.State {
 case youtubeStatePublished:
  c.State=publication.Published
  c.ProviderID=rec.VideoID
  c.ProviderURL=rec.VideoURL
 case youtubeStateFailed:
  c.State=publication.Failed
  c.Reason="YouTube upload failed: "+rec.Error
 case youtubeStateInterrupted:
  c.State=publication.Failed
  c.Reason="resumable upload interrupted; operator retry required"
 case youtubeStateUploading:
  if !s.youtubeUploadActive(studioYouTubeID(c.ID)){
   c.State=publication.Failed
   c.Reason="uploader restarted; retry resumes its stored session"
  } else {c.State=publication.Publishing}
 }
 return c
}
// The shared S3 checkpoint provides an across-restart channel pacing hook.
// Deploy only one active replay publisher: S3 object writes are not CAS leases.
func studioMinimumInterval()time.Duration {
 raw:=strings.TrimSpace(os.Getenv("POKEPILOT_STUDIO_MIN_UPLOAD_INTERVAL"))
 if raw=="" {return 15*time.Minute}
 gap,err:=time.ParseDuration(raw)
 if err!=nil||gap<0{return 15*time.Minute}
 return gap
}
func (s *replayServer) studioThrottle(ctx context.Context,now time.Time,id string)error{
 obj,err:=s.store.GetObject(ctx,studioChannelThrottleKey,"")
 if artifactstore.IsNotFound(err){return nil}
 if err!=nil{return err}
 defer obj.Body.Close()
 var last studioChannelThrottle
 if err:=json.NewDecoder(io.LimitReader(obj.Body,2048)).Decode(&last);err!=nil{return err}
 if last.CandidateID==id{return nil}
 gap:=studioMinimumInterval()
 if last.LastStarted>0 && now.Sub(time.Unix(last.LastStarted,0))<gap{
  return fmt.Errorf("publication: channel minimum upload interval not elapsed (%s)",gap)
 }
 return nil
}
func (s *replayServer) saveStudioThrottle(ctx context.Context,now time.Time,id string)error{
 data,_:=json.Marshal(studioChannelThrottle{LastStarted:now.Unix(),CandidateID:id})
 _,err:=s.store.PutObject(ctx,studioChannelThrottleKey,"application/json",data)
 return err
}
func (s *replayServer) handleStudioPublicationList(w http.ResponseWriter,r *http.Request){
 runID:=r.PathValue("id")
 if runID==""||s.store==nil{writeJSON(w,http.StatusServiceUnavailable,map[string]string{"error":"studio publication storage unavailable"});return}
 idx,err:=s.loadPublicationIndex(r.Context(),runID)
 if err!=nil{writeJSON(w,http.StatusBadGateway,map[string]string{"error":"cannot read publication index"});return}
 out:=studioPublicationList{Candidates:make([]publication.Candidate,0,len(idx.IDs))}
 for _,id:=range idx.IDs{
  c,err:=s.loadPublication(r.Context(),runID,id)
  if err!=nil{writeJSON(w,http.StatusBadGateway,map[string]string{"error":"publication candidate unreadable"});return}
  out.Candidates=append(out.Candidates,s.publicationStatus(r.Context(),c))
 }
 writeJSON(w,http.StatusOK,out)
}
func (s *replayServer) handleStudioPublicationCreate(w http.ResponseWriter,r *http.Request){
 runID:=r.PathValue("id")
 if runID==""||s.store==nil{writeJSON(w,http.StatusServiceUnavailable,map[string]string{"error":"publication storage unavailable"});return}
 var req studioPublicationRequest
 decoder:=json.NewDecoder(io.LimitReader(r.Body,4097))
 decoder.DisallowUnknownFields()
 if err:=decoder.Decode(&req);err!=nil||decoder.Decode(new(any))!=io.EOF{
  writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid publication candidate request"});return
 }
 candidate,err:=publication.New(publication.Proposal{
  RunID:runID,AssessmentHash:req.AssessmentHash,PackageHash:req.PackageHash,
  PlanHash:"",Format:req.Format,TitleID:req.TitleID,ThumbnailID:req.ThumbnailID,
  Privacy:req.Privacy,NotBefore:req.NotBefore,
 })
 if req.Format!=publication.Archive{
  // The selected edit plan belongs to this exact package; it is not a
  // client-authorized free-form path into S3.
  pkg,loadErr:=s.loadVerifiedThumbnailPackage(r.Context(),runID,req.PackageHash)
  if loadErr!=nil{writeJSON(w,http.StatusUnprocessableEntity,map[string]string{"error":"package evidence unavailable"});return}
  candidate,err=publication.New(publication.Proposal{
   RunID:runID,AssessmentHash:req.AssessmentHash,PackageHash:req.PackageHash,
   PlanHash:pkg.PlanHash,Format:req.Format,TitleID:req.TitleID,ThumbnailID:req.ThumbnailID,
   Privacy:req.Privacy,NotBefore:req.NotBefore,
  })
 }
 if err!=nil{writeJSON(w,http.StatusBadRequest,map[string]string{"error":err.Error()});return}
 in,err:=s.currentPublicationInputs(r.Context(),candidate)
 if err!=nil{writeJSON(w,http.StatusUnprocessableEntity,map[string]string{"error":"source evidence unavailable: "+clipError(err)});return}
 state,reason:=publication.Evaluate(candidate,in,studioQualityPolicy())
 candidate.State=state
 candidate.Reason=reason
 candidate.UpdatedAt=time.Now().Unix()
 // Each run's index is a durable review queue. This mutex only serializes
 // this single replay-host process; deploy one active uploader per store.
 s.studioPublishMu.Lock()
 defer s.studioPublishMu.Unlock()
 existing,err:=s.loadPublication(r.Context(),runID,candidate.ID)
 if err==nil {writeJSON(w,http.StatusOK,s.publicationStatus(r.Context(),existing));return}
 if !artifactstore.IsNotFound(err){writeJSON(w,http.StatusBadGateway,map[string]string{"error":"publication storage lookup failed"});return}
 idx,err:=s.loadPublicationIndex(r.Context(),runID)
 if err!=nil{writeJSON(w,http.StatusBadGateway,map[string]string{"error":"publication index unavailable"});return}
 if len(idx.IDs)>=1000{writeJSON(w,http.StatusConflict,map[string]string{"error":"publication queue limit reached"});return}
 if err:=s.savePublication(r.Context(),candidate);err!=nil{writeJSON(w,http.StatusBadGateway,map[string]string{"error":"cannot persist candidate"});return}
 idx.IDs=append(idx.IDs,candidate.ID)
 if err:=s.savePublicationIndex(r.Context(),idx);err!=nil{writeJSON(w,http.StatusBadGateway,map[string]string{"error":"cannot persist queue index"});return}
 writeJSON(w,http.StatusCreated,candidate)
}
func (s *replayServer) handleStudioPublicationAction(w http.ResponseWriter,r *http.Request){
 runID,id,action:=r.PathValue("id"),r.PathValue("candidate"),r.PathValue("action")
 if !validEditHash(id)||runID==""||s.store==nil{
  writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid publication candidate"});return
 }
 if action!="approve"&&action!="reject"&&action!="publish"{
  writeJSON(w,http.StatusNotFound,map[string]string{"error":"unknown publication action"});return
 }
 s.studioPublishMu.Lock()
 defer s.studioPublishMu.Unlock()
 c,err:=s.loadPublication(r.Context(),runID,id)
 if err!=nil{writeJSON(w,http.StatusNotFound,map[string]string{"error":"publication candidate missing"});return}
 c=s.publicationStatus(r.Context(),c)
 now:=time.Now()
 if action=="reject"{
  c,err=publication.Reject(c,now)
  if err!=nil{writeJSON(w,http.StatusConflict,map[string]string{"error":err.Error()});return}
  if err:=s.savePublication(r.Context(),c);err!=nil{writeJSON(w,http.StatusBadGateway,map[string]string{"error":"cannot save rejection"});return}
  writeJSON(w,http.StatusOK,c);return
 }
 in,err:=s.currentPublicationInputs(r.Context(),c)
 if err!=nil{writeJSON(w,http.StatusConflict,map[string]string{"error":"validated media unavailable: "+clipError(err)});return}
 state,reason:=publication.Evaluate(c,in,studioQualityPolicy())
 if state!=publication.Queued{
  if c.State==publication.Published{writeJSON(w,http.StatusOK,c);return}
  writeJSON(w,http.StatusConflict,map[string]string{"error":"quality gate: "+reason,"state":string(state)});return
 }
 if action=="approve"{
  c,err=publication.Approve(c,in,studioQualityPolicy(),now)
  if err!=nil{writeJSON(w,http.StatusConflict,map[string]string{"error":err.Error()});return}
  if err:=s.savePublication(r.Context(),c);err!=nil{writeJSON(w,http.StatusBadGateway,map[string]string{"error":"cannot record approval"});return}
  writeJSON(w,http.StatusOK,c);return
 }
 if err:=publication.CanDispatch(c,now);err!=nil{
  writeJSON(w,http.StatusConflict,map[string]string{"error":err.Error()});return
 }
 if !s.youtube.configured(){writeJSON(w,http.StatusServiceUnavailable,map[string]string{"error":"YouTube uploader is not configured"});return}
 if err:=s.studioThrottle(r.Context(),now,c.ID);err!=nil{
  writeJSON(w,http.StatusConflict,map[string]string{"error":clipError(err)});return
 }
 s.youtubeMu.Lock()
 activeYouTube:=len(s.youtubeActive)
 s.youtubeMu.Unlock()
 if activeYouTube>0 {writeJSON(w,http.StatusConflict,map[string]string{"error":"channel upload already active; retry after completion"});return}
 if c.SourceKey!=in.VideoKey||c.SourceSize!=in.VideoSize||c.ThumbnailKey!=in.ThumbnailKey{
  writeJSON(w,http.StatusConflict,map[string]string{"error":"approved artifacts changed; create a new candidate"});return
 }
 idx,err:=s.loadPublicationIndex(r.Context(),runID)
 if err!=nil{writeJSON(w,http.StatusBadGateway,map[string]string{"error":"cannot verify channel duplicate gate"});return}
 for _,otherID:=range idx.IDs{
  if otherID==id{continue}
  other,loadErr:=s.loadPublication(r.Context(),runID,otherID)
  if loadErr!=nil{writeJSON(w,http.StatusBadGateway,map[string]string{"error":"cannot verify prior publications"});return}
  other=s.publicationStatus(r.Context(),other)
  if other.Proposal.Format==c.Proposal.Format&&(other.State==publication.Published||other.State==publication.Publishing){
   writeJSON(w,http.StatusConflict,map[string]string{"error":"another candidate of this format is already uploading or published for this run"});return
  }
 }
 synthetic:=studioYouTubeID(c.ID)
 if !s.claimYouTubeUpload(synthetic){
  writeJSON(w,http.StatusAccepted,c);return
 }
 active:=false
 defer func(){if !active{s.releaseYouTubeUpload(synthetic)}}()
 rec,exists,err:=s.loadYouTubePublication(r.Context(),synthetic)
 if err!=nil{writeJSON(w,http.StatusBadGateway,map[string]string{"error":"cannot read uploader checkpoint"});return}
 if exists&&rec.State==youtubeStatePublished {
  c.State=publication.Published;c.ProviderID=rec.VideoID;c.ProviderURL=rec.VideoURL
  _=s.savePublication(r.Context(),c)
  writeJSON(w,http.StatusOK,c);return
 }
 if exists && (rec.SourceKey!=c.SourceKey||rec.SourceSize!=c.SourceSize) {
  writeJSON(w,http.StatusConflict,map[string]string{"error":"resumable uploader source differs from approved artifact"});return
 }
 if !exists {rec=youtubePublication{RunID:synthetic,SourceKey:c.SourceKey,SourceSize:c.SourceSize}}
 rec.State=youtubeStateUploading
 rec.Privacy=c.Proposal.Privacy
 meta:=publicationMeta(c,*in.Package)
 rec.Title=meta.Title
 rec.Error=""
 if err:=s.saveYouTubePublication(r.Context(),rec);err!=nil{writeJSON(w,http.StatusBadGateway,map[string]string{"error":"cannot checkpoint YouTube upload"});return}
 c.State=publication.Publishing;c.Attempts++;c.UpdatedAt=now.Unix();c.Reason=""
 if err:=s.savePublication(r.Context(),c);err!=nil{writeJSON(w,http.StatusBadGateway,map[string]string{"error":"cannot mark publication as uploading"});return}
 if err:=s.saveStudioThrottle(r.Context(),now,c.ID);err!=nil {
  writeJSON(w,http.StatusBadGateway,map[string]string{"error":"cannot persist channel rate limit"});return
 }
 active=true
 go s.publishStudioCandidate(c,rec,meta)
 writeJSON(w,http.StatusAccepted,c)
}
func publicationMeta(c publication.Candidate,pkg packaging.Package)youtubeVideoMeta{
 title:=""
 if c.Proposal.Format==publication.Short {
  for _,candidate:=range pkg.Shorts{
   if candidate.ID==c.Proposal.TitleID {title=candidate.Title;break}
  }
 } else {
  for _,candidate:=range pkg.Titles{
   if candidate.ID==c.Proposal.TitleID {title=candidate.Title;break}
  }
 }
 desc:=pkg.Description
 if len(pkg.Chapters)>0 {
  desc+="\n\nChapters:\n"
  for _,chapter:=range pkg.Chapters{
   seconds:=chapter.StartMS/1000
   desc+=fmt.Sprintf("%d:%02d %s\n",seconds/60,seconds%60,chapter.Title)
  }
 }
 if len(desc)>youtubeDescMaxBytes {desc=desc[:youtubeDescMaxBytes]}
 return youtubeVideoMeta{Title:title,Description:desc}
}
func (s *replayServer) publishStudioCandidate(c publication.Candidate,rec youtubePublication,meta youtubeVideoMeta){
 synthetic:=studioYouTubeID(c.ID)
 defer s.releaseYouTubeUpload(synthetic)
 ctx,cancel:=context.WithTimeout(context.Background(),youtubeUploadTimeout)
 defer cancel()
 result:=s.publishYouTube(ctx,rec,meta)
 if result.State==youtubeStatePublished{
  c.State=publication.Published;c.ProviderID=result.VideoID;c.ProviderURL=result.VideoURL;c.Reason=""
  if c.ThumbnailKey!=""{
   if err:=s.uploadStudioThumbnail(ctx,result.VideoID,c.ThumbnailKey);err!=nil{
    c.ThumbnailStatus="failed"
    c.Reason="video published; thumbnail upload failed: "+clipError(err)
   }else{c.ThumbnailStatus="ready"}
  }
 }else{
  c.State=publication.Failed;c.Reason="YouTube upload failed: "+result.Error
 }
 c.UpdatedAt=time.Now().Unix()
 saveCtx,done:=context.WithTimeout(context.Background(),wallTimeout)
 defer done()
 s.studioPublishMu.Lock()
 defer s.studioPublishMu.Unlock()
 if err:=s.savePublication(saveCtx,c);err!=nil{
  // The existing resumable uploader record is the authoritative checkpoint;
  // a subsequent GET/retry reconciles the publication outcome.
  fmt.Fprintf(os.Stderr,"pokereplay: studio publication status store failed: %v\n",err)
 }
}
