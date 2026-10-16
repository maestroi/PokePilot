package packaging

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
    "time"
)

// ChatModel is a narrow OpenAI-compatible angle selector. Only validated
// choices can affect code-generated public metadata.
type ChatModel struct {
    Endpoint string
    ModelID string
    Deployment string
    APIKey string
    Client *http.Client
}

func (m ChatModel) Identity() string {
    if m.Deployment!="" { return "chat:"+m.ModelID+":"+m.Deployment }
    return "chat:"+m.ModelID
}
const selectionSystemPrompt = "You select editorial angles for existing verified PokePilot run events. Return ONLY JSON with selections array; each selection has attempt, event_id, angle. Choose two or three entries, using exactly the supplied attempt and event_id, and at least two different angles from moment, challenge, recap. Prefer distinct significant events and do not narrate routine repetition. Do not write factual claims, titles, descriptions, thumbnails, prompt reasoning or shell commands. An unknown event, angle, extra field or invalid schema will be rejected."

func (m ChatModel) Select(ctx context.Context, data ModelContext)([]byte,error) {
    u,err:=url.Parse(strings.TrimSpace(m.Endpoint))
    if err!=nil||u==nil||(u.Scheme!="https"&&u.Scheme!="http")||u.Host==""||u.User!=nil||u.RawQuery!=""||u.Fragment!="" {
        return nil,errors.New("packaging: chat endpoint must be absolute HTTP(S) without credentials or query")
    }
    if strings.TrimSpace(m.ModelID)=="" {return nil,errors.New("packaging: model ID required")}
    // Expose bounded, spectator-safe semantics only, not game planner turns.
    if len(data.Events)>64 {
        sampled:=make([]Fact,0,64)
        for i:=0;i<64;i++ { sampled=append(sampled,data.Events[i*(len(data.Events)-1)/63]) }
        data.Events=sampled
    }
    payload,err:=json.Marshal(data)
    if err!=nil{return nil,err}
    body,err:=json.Marshal(map[string]any{
        "model":m.ModelID,"temperature":0,
        "messages":[]map[string]string{
            {"role":"system","content":selectionSystemPrompt},
            {"role":"user","content":string(payload)},
        },
    })
    if err!=nil{return nil,err}
    req,err:=http.NewRequestWithContext(ctx,http.MethodPost,u.String(),bytes.NewReader(body))
    if err!=nil{return nil,err}
    req.Header.Set("Content-Type","application/json")
    if m.APIKey!="" {req.Header.Set("Authorization","Bearer "+m.APIKey)}
    client:=m.Client
    if client==nil {client=&http.Client{Timeout:60*time.Second}}
    res,err:=client.Do(req)
    if err!=nil{return nil,errors.New("packaging: model endpoint unavailable")}
    defer res.Body.Close()
    buf,err:=io.ReadAll(io.LimitReader(res.Body,MaxModelBytes+1))
    if err!=nil{return nil,err}
    if len(buf)>MaxModelBytes{return nil,errors.New("packaging: model response exceeded bound")}
    if res.StatusCode<200||res.StatusCode>=300{return nil,fmt.Errorf("packaging: model endpoint returned %s",res.Status)}
    var envelope struct {
        Choices []struct {
            Message struct {Content string `json:"content"`} `json:"message"`
        } `json:"choices"`
    }
    if err:=json.Unmarshal(buf,&envelope);err!=nil{return nil,errors.New("packaging: invalid model transport envelope")}
    if len(envelope.Choices)==0{return nil,errors.New("packaging: no model choices returned")}
    return []byte(envelope.Choices[0].Message.Content),nil
}
