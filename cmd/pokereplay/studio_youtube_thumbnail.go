package main

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "image"
 "io"
 "net/http"
 "net/url"
 "os"
 "strings"

 "github.com/maestroi/pokepilot/media/thumbnail"
)

// uploadStudioThumbnail adds selected artwork to a video already uploaded by
// the existing #178 resumable YouTube publisher. This is NOT a second video
// uploader. Video publication remains successful if this later step fails.
func (s *replayServer) uploadStudioThumbnail(ctx context.Context,videoID,key string)error{
 if videoID==""||key=="" {return errors.New("publication: thumbnail/video identity missing")}
 obj,err:=s.store.GetObject(ctx,key,"")
 if err!=nil{return errors.New("publication: selected thumbnail missing")}
 defer obj.Body.Close()
 raw,err:=io.ReadAll(io.LimitReader(obj.Body,thumbnail.MaxImageBytes+1))
 if err!=nil||len(raw)==0||len(raw)>thumbnail.MaxImageBytes{
  return errors.New("publication: selected thumbnail exceeds YouTube size constraints")
 }
 config,format,err:=image.DecodeConfig(bytes.NewReader(raw))
 if err!=nil||format!="jpeg"||config.Width!=thumbnail.Width||config.Height!=thumbnail.Height{
  return errors.New("publication: selected image format or dimensions unsupported")
 }
 token,err:=s.youtubeAccessToken(ctx)
 if err!=nil{return err}
 endpoint:=strings.TrimSpace(os.Getenv("POKEPILOT_YOUTUBE_THUMBNAIL_UPLOAD_URL"))
 if endpoint==""{endpoint="https://www.googleapis.com/upload/youtube/v3/thumbnails/set"}
 u,err:=url.Parse(endpoint)
 if err!=nil||u==nil||(u.Scheme!="https"&&u.Scheme!="http")||u.User!=nil||u.Host==""{
  return errors.New("publication: thumbnail upload endpoint invalid")
 }
 q:=u.Query()
 q.Set("videoId",videoID)
 q.Set("uploadType","media")
 u.RawQuery=q.Encode()
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,u.String(),bytes.NewReader(raw))
 if err!=nil{return err}
 req.Header.Set("Authorization","Bearer "+token)
 req.Header.Set("Content-Type","image/jpeg")
 res,err:=s.youtubeHTTP.Do(req)
 if err!=nil{return fmt.Errorf("publication: thumbnail upload unavailable: %w",stripURLError(err))}
 defer res.Body.Close()
 if res.StatusCode<200||res.StatusCode>=300{return fmt.Errorf("publication: thumbnail upload returned HTTP %d",res.StatusCode)}
 return nil
}
