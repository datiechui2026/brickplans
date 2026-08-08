package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"brickplans/internal/config"
)

// WeChatClient exchanges a wx.login() code for the user's openid/unionid.
// The mini program calls wx.login() (silent, no user consent prompt) to obtain
// a short-lived code, then posts it to /api/auth/wechat-login. Only the server
// knows the AppSecret, so the exchange happens here - never in the client.
type WeChatClient interface {
	Code2Session(code string) (openid, unionid string, err error)
}

// jscode2sessionResponse mirrors WeChat's response. Errcode is 0 on success;
// a nonzero errcode means the code was invalid/expired or the config is wrong.
type jscode2sessionResponse struct {
	OpenID     string `json:"openid"`
	SessionKey string `json:"session_key"`
	UnionID    string `json:"unionid"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

type realWeChatClient struct {
	appID     string
	appSecret string
	http      *http.Client
}

func (c *realWeChatClient) Code2Session(code string) (string, string, error) {
	const baseURL = "https://api.weixin.qq.com/sns/jscode2session"
	url := fmt.Sprintf("%s?appid=%s&secret=%s&js_code=%s&grant_type=authorization_code",
		baseURL, c.appID, c.appSecret, code)
	resp, err := c.http.Get(url)
	if err != nil {
		return "", "", fmt.Errorf("wechat: call jscode2session: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", fmt.Errorf("wechat: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("wechat: jscode2session returned HTTP %d: %s", resp.StatusCode, string(body))
	}
	var r jscode2sessionResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return "", "", fmt.Errorf("wechat: parse response: %w", err)
	}
	if r.ErrCode != 0 || r.OpenID == "" {
		return "", "", fmt.Errorf("wechat: jscode2session error %d: %s", r.ErrCode, r.ErrMsg)
	}
	return r.OpenID, r.UnionID, nil
}

// NewWeChatClient builds the real WeChat client, or returns nil when WeChat
// login is not configured (no AppID/AppSecret). It is a package-level variable
// so tests can swap in a fake client without touching the real network.
var NewWeChatClient = func(cfg *config.Config) WeChatClient {
	if cfg.WeChatAppID == "" || cfg.WeChatAppSecret == "" {
		return nil
	}
	return &realWeChatClient{
		appID:     cfg.WeChatAppID,
		appSecret: cfg.WeChatAppSecret,
		http:      &http.Client{Timeout: 10 * time.Second},
	}
}
