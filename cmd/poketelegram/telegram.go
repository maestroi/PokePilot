package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type telegramUser struct {
	ID int64 `json:"id"`
}

type telegramChat struct {
	ID int64 `json:"id"`
}

type telegramMessage struct {
	MessageID int64        `json:"message_id"`
	From      telegramUser `json:"from"`
	Chat      telegramChat `json:"chat"`
	Text      string       `json:"text"`

	ReplyToMessage *telegramMessage `json:"reply_to_message,omitempty"`
}

type callbackQuery struct {
	ID      string          `json:"id"`
	From    telegramUser    `json:"from"`
	Message telegramMessage `json:"message"`
	Data    string          `json:"data"`
}

type telegramUpdate struct {
	UpdateID      int64            `json:"update_id"`
	Message       *telegramMessage `json:"message,omitempty"`
	CallbackQuery *callbackQuery   `json:"callback_query,omitempty"`
}

type telegramResponse[T any] struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
	Result      T      `json:"result"`
}

type inlineKeyboard struct {
	InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
}

type inlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

type telegramClient struct {
	token string
	http  *http.Client
	base  string // empty means https://api.telegram.org
}

func (t *telegramClient) getUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]telegramUpdate, error) {
	form := url.Values{}
	form.Set("offset", strconv.FormatInt(offset, 10))
	form.Set("timeout", strconv.Itoa(int(timeout.Seconds())))
	form.Set("allowed_updates", `["message","callback_query"]`)
	var out telegramResponse[[]telegramUpdate]
	if err := t.callForm(ctx, "getUpdates", form, &out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, errors.New(out.Description)
	}
	return out.Result, nil
}

func (t *telegramClient) sendMessage(ctx context.Context, chatID int64, text string, keyboard *inlineKeyboard) error {
	_, err := t.send(ctx, chatID, outgoing{Text: text, Keyboard: keyboard})
	return err
}

func (t *telegramClient) sendPhoto(ctx context.Context, chatID int64, data []byte, mediaType, caption string) error {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	_ = mw.WriteField("caption", clip(caption, 900))
	part, err := mw.CreateFormFile("photo", "frame.png")
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint("sendPhoto"), &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if mediaType != "" {
		req.Header.Set("X-PokePilot-Source-Media-Type", mediaType)
	}
	res, err := t.roundTrip(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var out telegramResponse[json.RawMessage]
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || !out.OK {
		return fmt.Errorf("telegram sendPhoto %s: %s", res.Status, out.Description)
	}
	return nil
}

func (t *telegramClient) answerCallback(ctx context.Context, id string) error {
	var out telegramResponse[bool]
	return t.callJSON(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id}, &out)
}

func (t *telegramClient) callForm(ctx context.Context, method string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint(method), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return t.do(req, out)
}

func (t *telegramClient) callJSON(ctx context.Context, method string, payload any, out any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint(method), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return t.do(req, out)
}

// roundTrip strips the request URL (which embeds the bot token) from transport errors.
func (t *telegramClient) roundTrip(req *http.Request) (*http.Response, error) {
	res, err := t.http.Do(req)
	var ue *url.Error
	if errors.As(err, &ue) {
		return nil, fmt.Errorf("telegram %s: %w", req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:], ue.Err)
	}
	return res, err
}

func (t *telegramClient) do(req *http.Request, out any) error {
	res, err := t.roundTrip(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var e struct {
			Description string `json:"description"`
		}
		_ = json.Unmarshal(data, &e)
		return classifyTelegramError(fmt.Errorf("telegram API %s: %s", res.Status, firstNonEmpty(e.Description, strings.TrimSpace(string(data)))))
	}
	return json.Unmarshal(data, out)
}

func (t *telegramClient) endpoint(method string) string {
	base := t.base
	if base == "" {
		base = "https://api.telegram.org"
	}
	return base + "/bot" + t.token + "/" + method
}

var (
	errMessageGone = errors.New("telegram: message cannot be edited")
	errNotModified = errors.New("telegram: message is not modified")
)

func classifyTelegramError(err error) error {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "message is not modified"):
		return fmt.Errorf("%w: %v", errNotModified, err)
	case strings.Contains(msg, "message to edit not found"), strings.Contains(msg, "message can't be edited"):
		return fmt.Errorf("%w: %v", errMessageGone, err)
	}
	return err
}

func h(s string) string { return html.EscapeString(s) }

type outgoing struct {
	Text     string
	HTML     bool
	Keyboard *inlineKeyboard
	ReplyTo  int64
	Silent   bool
}

func (m outgoing) payload(chatID int64) map[string]any {
	text := m.Text
	if !m.HTML { // clipping HTML could cut a tag or entity; renderers keep HTML short by construction
		text = clip(text, 3900)
	}
	p := map[string]any{
		"chat_id":              chatID,
		"text":                 text,
		"link_preview_options": map[string]any{"is_disabled": true},
	}
	if m.HTML {
		p["parse_mode"] = "HTML"
	}
	if m.Keyboard != nil {
		p["reply_markup"] = m.Keyboard
	}
	if m.ReplyTo != 0 {
		p["reply_parameters"] = map[string]any{"message_id": m.ReplyTo, "allow_sending_without_reply": true}
	}
	if m.Silent {
		p["disable_notification"] = true
	}
	return p
}

func (t *telegramClient) send(ctx context.Context, chatID int64, m outgoing) (int64, error) {
	var out telegramResponse[telegramMessage]
	if err := t.callJSON(ctx, "sendMessage", m.payload(chatID), &out); err != nil {
		return 0, err
	}
	if !out.OK {
		return 0, errors.New(out.Description)
	}
	return out.Result.MessageID, nil
}

func (t *telegramClient) edit(ctx context.Context, chatID, messageID int64, m outgoing) error {
	p := m.payload(chatID)
	p["message_id"] = messageID
	delete(p, "reply_parameters")
	delete(p, "disable_notification")
	var out telegramResponse[json.RawMessage]
	if err := t.callJSON(ctx, "editMessageText", p, &out); err != nil {
		return err
	}
	if !out.OK {
		return classifyTelegramError(errors.New(out.Description))
	}
	return nil
}

func (t *telegramClient) setCommands(ctx context.Context, cmds [][2]string) error {
	list := make([]map[string]string, 0, len(cmds))
	for _, c := range cmds {
		list = append(list, map[string]string{"command": c[0], "description": c[1]})
	}
	var out telegramResponse[bool]
	return t.callJSON(ctx, "setMyCommands", map[string]any{"commands": list}, &out)
}
