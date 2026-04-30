package upscale

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// 图像清晰度增强：POST .../image_quality_enhance（与控制台/示例一致）。
const (
	baiduTokenURL    = "https://aip.baidubce.com/oauth/2.0/token"
	baiduEnhanceURL  = "https://aip.baidubce.com/rest/2.0/image-process/v1/image_quality_enhance"
	tokenReuseMargin = 2 * time.Minute
)

var (
	baiduTokenMu       sync.Mutex
	baiduCachedToken   string
	baiduTokenDeadline time.Time
)

func baiduHTTPClient(hc *http.Client) *http.Client {
	if hc != nil {
		return hc
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func fetchBaiduAccessToken(ctx context.Context, apiKey, secretKey string, hc *http.Client) (string, int, error) {
	hc = baiduHTTPClient(hc)
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", apiKey)
	form.Set("client_secret", secretKey)
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		baiduTokenURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", 0, fmt.Errorf("baidu token: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := hc.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("baidu token: request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", 0, fmt.Errorf("baidu token: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("baidu token: http %d: %s", resp.StatusCode, truncateForErr(body, 400))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", 0, fmt.Errorf("baidu token: parse: %w body=%s", err, truncateForErr(body, 300))
	}
	if out.AccessToken == "" {
		if out.Error != "" {
			return "", 0, fmt.Errorf("baidu token: %s: %s", out.Error, out.ErrorDesc)
		}
		return "", 0, fmt.Errorf("baidu token: 无 access_token: %s", truncateForErr(body, 400))
	}
	return out.AccessToken, out.ExpiresIn, nil
}

func getBaiduAccessToken(ctx context.Context, apiKey, secretKey string, hc *http.Client) (string, error) {
	baiduTokenMu.Lock()
	defer baiduTokenMu.Unlock()
	if baiduCachedToken != "" && time.Now().Add(tokenReuseMargin).Before(baiduTokenDeadline) {
		return baiduCachedToken, nil
	}
	tok, expSec, err := fetchBaiduAccessToken(ctx, apiKey, secretKey, hc)
	if err != nil {
		return "", err
	}
	if expSec <= 0 {
		expSec = 30 * 24 * 3600
	}
	baiduCachedToken = tok
	baiduTokenDeadline = time.Now().Add(time.Duration(expSec) * time.Second)
	log.Printf("[baidu] access_token 已刷新，约 %d 秒后过期", expSec)
	return baiduCachedToken, nil
}

func callDefinitionEnhance(ctx context.Context, accessToken, imageB64 string, hc *http.Client) ([]byte, error) {
	hc = baiduHTTPClient(hc)
	endpoint := baiduEnhanceURL + "?access_token=" + url.QueryEscape(accessToken)
	form := url.Values{}
	form.Set("image", imageB64)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("baidu enhance: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	log.Printf("[baidu] POST image_quality_enhance, image_b64_len=%d", len(imageB64))
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("baidu enhance: request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("baidu enhance: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("baidu enhance: http %d: %s", resp.StatusCode, truncateForErr(body, 500))
	}
	var loose struct {
		Image     string `json:"image"`
		ErrorCode int    `json:"error_code"`
		ErrorMsg  string `json:"error_msg"`
	}
	if err := json.Unmarshal(body, &loose); err != nil {
		return nil, fmt.Errorf("baidu enhance: parse json: %w body=%s", err, truncateForErr(body, 400))
	}
	if loose.ErrorCode != 0 {
		return nil, fmt.Errorf("baidu enhance: error_code=%d %s", loose.ErrorCode, loose.ErrorMsg)
	}
	if loose.Image == "" {
		return nil, fmt.Errorf("baidu enhance: 响应无有效 image 字段: %s", truncateForErr(body, 400))
	}
	raw, err := base64.StdEncoding.DecodeString(loose.Image)
	if err != nil {
		return nil, fmt.Errorf("baidu enhance: decode result base64: %w", err)
	}
	return raw, nil
}

func truncateForErr(b []byte, max int) string {
	s := string(b)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
