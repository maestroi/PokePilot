package main

import (
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "image"
 "image/color"
 "image/jpeg"
 "io"
 "net/http"
 "net/http/httptest"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/maestroi/pokepilot/farm"
 "github.com/maestroi/pokepilot/media/packaging"
 "github.com/maestroi/pokepilot/media/producer"
 "github.com/maestroi/pokepilot/media/publication"
)

func studioAssessmentFixture(t *testing.T) producer.Assessment {
 t.Helper()
 tline:=farm.MediaTimeline{
  Run:farm.MediaRunSummary{RunID:"pub-archive",Goal:"collect badges",Status:"done"},
  Attempt:1,EndFrame:120000,FramesPerSecond:60,
  Events:[]farm.MediaEvent{
   {Type:"gym_battle",Frame:28000,StartFrame:25000,Evidence:"gym"},
   {Type:"badge_acquired",Frame:30000,Evidence:"badge"},
   {Type:"blackout",Frame:66000,Evidence:"blackout"},
   {Type:"failure_recovered",Frame:75000,Evidence:"recovery"},
   {Type:"run_finished",Frame:100000,Evidence:"finish"},
  },
 }.Normalized()
 a,err:=producer.Assess(context.Background(),producer.Input{RunID:"pub-archive",Timelines:[]farm.MediaTimeline{tline}},nil,producer.DefaultPolicy())
 if err!=nil{t.Fatal(err)}
 return a
}
func TestStudioPublicationArchiveAndIdempotentQueue(t *testing.T){
 mem,store:=newMemS3(t)
 a:=studioAssessmentFixture(t)
 raw,_:=json.Marshal(a)
 mem.put(producerObjectKey(a.RunID,a.Hash),raw)
 s:=newReplayServer("","","",store)
 srv:=httptest.NewServer(s.handler())
 defer srv.Close()
 endpoint:=srv.URL+"/v1/runs/"+a.RunID+"/studio/publications"
 create:=func()publication.Candidate{
  t.Helper()
  res,err:=http.Post(endpoint,"application/json",strings.NewReader(fmt.Sprintf(`{"assessment_hash":%q,"format":"archive"}`,a.Hash)))
  if err!=nil{t.Fatal(err)}
  defer res.Body.Close()
  var got publication.Candidate
  data,_:=io.ReadAll(res.Body)
  if res.StatusCode!=http.StatusCreated&&res.StatusCode!=http.StatusOK{
   t.Fatalf("unexpected create %d: %s",res.StatusCode,data)
  }
  if err:=json.Unmarshal(data,&got);err!=nil{t.Fatal(err)}
  return got
 }
 first:=create()
 if first.State!=publication.Skipped||first.Approved{t.Fatalf("archive should never require upload: %+v",first)}
 second:=create()
 if first.ID!=second.ID{t.Fatal("queue is not idempotent")}
 res,err:=http.Get(endpoint)
 if err!=nil{t.Fatal(err)}
 defer res.Body.Close()
 var list studioPublicationList
 if err:=json.NewDecoder(res.Body).Decode(&list);err!=nil{t.Fatal(err)}
 if len(list.Candidates)!=1||list.Candidates[0].State!=publication.Skipped{
  t.Fatalf("durable index includes duplicates or unsafe candidate: %+v",list)
 }
 post,err:=http.Post(endpoint+"/"+first.ID+"/approve","application/json",strings.NewReader("{}"))
 if err!=nil{t.Fatal(err)}
 defer post.Body.Close()
 if post.StatusCode!=http.StatusConflict{t.Fatalf("skipped output approved: %d",post.StatusCode)}
 if !strings.Contains(studioPublicationRoot(a.RunID),"pub-archive"){t.Fatal("unexpected storage scope")}
}
func TestStudioChannelPacingAndShortMetadata(t *testing.T){
 _,store:=newMemS3(t)
 s:=newReplayServer("","","",store)
 t.Setenv("POKEPILOT_STUDIO_MIN_UPLOAD_INTERVAL","20m")
 now:=time.Unix(1_000_000,0)
 if err:=s.studioThrottle(context.Background(),now,"one");err!=nil{t.Fatal(err)}
 if err:=s.saveStudioThrottle(context.Background(),now,"one");err!=nil{t.Fatal(err)}
 if err:=s.studioThrottle(context.Background(),now.Add(time.Minute),"another");err==nil{
  t.Fatal("unrelated publication bypassed channel pacing")
 }
 if err:=s.studioThrottle(context.Background(),now.Add(time.Minute),"one");err!=nil{
  t.Fatal("resume for the same uploader must remain permitted")
 }
 if err:=s.studioThrottle(context.Background(),now.Add(21*time.Minute),"another");err!=nil{t.Fatal(err)}
 c:=publication.Candidate{Proposal:publication.Proposal{Format:publication.Short,TitleID:"short-1"}}
 pkg:=packaging.Package{Description:"Evidenced run",Shorts:[]packaging.ShortCandidate{{ID:"short-1",Title:"Verified short"}}}
 meta:=publicationMeta(c,pkg)
 if meta.Title!="Verified short"{t.Fatalf("short used episode title: %q",meta.Title)}
}
func TestStudioThumbnailSetDoesNotRequireVideoReupload(t *testing.T){
 _,store:=newMemS3(t)
 imageRGB:=image.NewRGBA(image.Rect(0,0,1280,720))
 for i:=range imageRGB.Pix{imageRGB.Pix[i]=70}
 imageRGB.SetRGBA(3,3,color.RGBA{R:220,A:255})
 var buf bytes.Buffer
 if err:=jpeg.Encode(&buf,imageRGB,&jpeg.Options{Quality:80});err!=nil{t.Fatal(err)}
 obj,err:=store.PutObject(context.Background(),"artwork/thumb.jpg","image/jpeg",buf.Bytes())
 if err!=nil{t.Fatal(err)}
 var thumbCalls int
 yt:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  switch r.URL.Path {
  case "/token":
   _,_=io.WriteString(w,`{"access_token":"token"}`)
  case "/thumb":
   thumbCalls++
   if r.URL.Query().Get("videoId")!="example-video"||r.URL.Query().Get("uploadType")!="media"||r.Header.Get("Authorization")!="Bearer token"{
    t.Errorf("invalid thumbnail identity/scope")
   }
   body,_:=io.ReadAll(r.Body)
   if len(body)!=len(buf.Bytes()){t.Errorf("wrong thumbnail payload size")}
   w.WriteHeader(http.StatusOK)
  default:t.Errorf("unexpected YouTube route %s",r.URL.Path)
  }
 }))
 defer yt.Close()
 s:=newReplayServer("","","",store)
 s.youtube=youtubeConfig{ClientID:"client",ClientSecret:"secret",RefreshToken:"refresh",
  TokenURL:yt.URL+"/token"}
 t.Setenv("POKEPILOT_YOUTUBE_THUMBNAIL_UPLOAD_URL",yt.URL+"/thumb")
 if err:=s.uploadStudioThumbnail(context.Background(),"example-video",obj.Key);err!=nil{t.Fatal(err)}
 if thumbCalls!=1{t.Fatalf("thumbnail upload count %d",thumbCalls)}
 if err:=os.Unsetenv("POKEPILOT_YOUTUBE_THUMBNAIL_UPLOAD_URL");err!=nil{t.Fatal(err)}
}
