package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type API struct {
	httpClient *http.Client
	baseURL    string
	username   string
}

type User struct {
	ID       int64  `json:"id"`
	UserName string `json:"username"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

func (c Chat) IsPrivate() bool {
	return c.Type == "private"
}

type Message struct {
	MessageID int64  `json:"message_id"`
	From      User   `json:"from"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

type apiResp[T any] struct {
	OK     bool   `json:"ok"`
	Result T      `json:"result"`
	Desc   string `json:"description"`
}

func New(token string) *API {
	return &API{
		httpClient: &http.Client{Timeout: 70 * time.Second},
		baseURL:    "https://api.telegram.org/bot" + token,
	}
}

func (a *API) Username() string {
	return a.username
}

func (a *API) GetMe(ctx context.Context) error {
	var out User
	if err := a.call(ctx, "getMe", nil, &out); err != nil {
		return err
	}
	a.username = out.UserName
	return nil
}

func (a *API) GetUpdates(ctx context.Context, offset int64, timeout int) ([]Update, error) {
	q := url.Values{}
	if offset != 0 {
		q.Set("offset", strconv.FormatInt(offset, 10))
	}
	if timeout > 0 {
		q.Set("timeout", strconv.Itoa(timeout))
	}
	var out []Update
	if err := a.call(ctx, "getUpdates", q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *API) SendMessage(ctx context.Context, chatID int64, text string) error {
	q := url.Values{}
	q.Set("chat_id", strconv.FormatInt(chatID, 10))
	q.Set("text", text)
	q.Set("disable_web_page_preview", "true")
	var out Message
	return a.call(ctx, "sendMessage", q, &out)
}

func (a *API) call(ctx context.Context, method string, params url.Values, out any) error {
	endpoint := strings.TrimRight(a.baseURL, "/") + "/" + method
	var req *http.Request
	var err error
	if params == nil {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(params.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if err != nil {
		return err
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram api status %d: %s", resp.StatusCode, string(b))
	}

	wrapper := apiResp[json.RawMessage]{}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return err
	}
	if !wrapper.OK {
		return fmt.Errorf("telegram api error: %s", wrapper.Desc)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(wrapper.Result, out); err != nil {
		return err
	}
	return nil
}
