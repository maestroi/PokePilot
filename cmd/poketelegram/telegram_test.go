package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeTG struct {
	srv   *httptest.Server
	calls []map[string]any
	reply func(method string, body map[string]any) (int, string)
}

func newFakeTG(t *testing.T) *fakeTG {
	f := &fakeTG{}
	f.reply = func(string, map[string]any) (int, string) { return 200, `{"ok":true,"result":{"message_id":42}}` }
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		body := map[string]any{"_method": method}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.calls = append(f.calls, body)
		code, out := f.reply(method, body)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(out))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTG) client() *telegramClient {
	return &telegramClient{token: "T", http: f.srv.Client(), base: f.srv.URL}
}

func TestSendHTMLReturnsMessageIDAndReplies(t *testing.T) {
	f := newFakeTG(t)
	id, err := f.client().send(context.Background(), 7, outgoing{Text: "<b>" + h("a<b&c") + "</b>", HTML: true, ReplyTo: 9, Silent: true})
	if err != nil || id != 42 {
		t.Fatalf("send: %d %v", id, err)
	}
	c := f.calls[0]
	if c["parse_mode"] != "HTML" || c["text"] != "<b>a&lt;b&amp;c</b>" || c["disable_notification"] != true {
		t.Fatalf("payload: %+v", c)
	}
	if rp, _ := c["reply_parameters"].(map[string]any); rp["message_id"] != float64(9) || rp["allow_sending_without_reply"] != true {
		t.Fatalf("reply: %+v", c)
	}
}

func TestEditClassifiesTelegramErrors(t *testing.T) {
	f := newFakeTG(t)
	f.reply = func(string, map[string]any) (int, string) {
		return 400, `{"ok":false,"description":"Bad Request: message is not modified"}`
	}
	if err := f.client().edit(context.Background(), 1, 2, outgoing{Text: "x"}); !errors.Is(err, errNotModified) {
		t.Fatalf("not modified: %v", err)
	}
	f.reply = func(string, map[string]any) (int, string) {
		return 400, `{"ok":false,"description":"Bad Request: message to edit not found"}`
	}
	if err := f.client().edit(context.Background(), 1, 2, outgoing{Text: "x"}); !errors.Is(err, errMessageGone) {
		t.Fatalf("gone: %v", err)
	}
	f.reply = func(string, map[string]any) (int, string) {
		return 400, `{"ok":false,"description":"Bad Request: message can't be edited"}`
	}
	if err := f.client().edit(context.Background(), 1, 2, outgoing{Text: "x"}); !errors.Is(err, errMessageGone) {
		t.Fatalf("too old: %v", err)
	}
}

func TestSetCommands(t *testing.T) {
	f := newFakeTG(t)
	f.reply = func(string, map[string]any) (int, string) { return 200, `{"ok":true,"result":true}` }
	if err := f.client().setCommands(context.Background(), [][2]string{{"menu", "Open the menu"}}); err != nil {
		t.Fatal(err)
	}
	if f.calls[0]["_method"] != "setMyCommands" {
		t.Fatalf("%+v", f.calls)
	}
}

func TestTransportErrorsDoNotLeakToken(t *testing.T) {
	f := newFakeTG(t)
	c := f.client()
	c.token = "SECRET123"
	f.srv.Close() // connection refused
	ctx := context.Background()
	_, e1 := c.getUpdates(ctx, 0, 0)
	_, e2 := c.send(ctx, 1, outgoing{Text: "x"})
	e3 := c.sendPhoto(ctx, 1, []byte("p"), "", "c")
	for i, err := range []error{e1, e2, e3} {
		if err == nil || strings.Contains(err.Error(), "SECRET123") {
			t.Fatalf("%d: %v", i, err)
		}
	}
}
