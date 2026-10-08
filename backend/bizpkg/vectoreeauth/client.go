/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package vectoreeauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/bizpkg/vectoreelink"
)

const (
	registerPath = "/api/auth/users"
	loginPath    = "/api/auth/sessions"
	verifyPath   = "/api/auth/email/verify"
	resendPath   = "/api/auth/email/send-verification"
)

// ErrNotConfigured means this server has no project API key with auth:*.
var ErrNotConfigured = errors.New("vectoree auth is not configured")

// Result is a Vectoree App Auth response. AccessToken is the app JWT (aud=app).
// Issuer is the Vectoree origin that answered; Subject is its user id.
type Result struct {
	StatusCode               int
	AccessToken              string
	RequireEmailVerification bool
	Message                  string
	Issuer                   string
	Subject                  string
	Email                    string
	EmailVerified            bool
}

func (r Result) Authenticated() bool {
	return r.AccessToken != ""
}

func (r Result) NeedsVerification() bool {
	if r.Authenticated() {
		return false
	}
	return r.RequireEmailVerification || r.StatusCode == http.StatusForbidden
}

func (r Result) SafeMessage() string {
	msg := strings.TrimSpace(r.Message)
	if len(msg) > 180 {
		msg = msg[:180]
	}
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "sk-ve") || strings.Contains(lower, "api key") || strings.Contains(lower, "authorization") {
		return "vectoree auth request failed"
	}
	return msg
}

// Client calls Vectoree App Auth with the project API key. The key never leaves the server.
type Client struct {
	HTTP   *http.Client
	Origin string
	APIKey string
}

func DefaultClient() *Client {
	origin, apiKey := vectoreelink.ProjectAPICredentials()
	return &Client{
		HTTP:   &http.Client{Timeout: 20 * time.Second},
		Origin: origin,
		APIKey: apiKey,
	}
}

func Register(ctx context.Context, email, password string) (Result, error) {
	return DefaultClient().post(ctx, registerPath, map[string]string{
		"email":    email,
		"password": password,
	})
}

func Login(ctx context.Context, email, password string) (Result, error) {
	return DefaultClient().post(ctx, loginPath, map[string]string{
		"email":    email,
		"password": password,
	})
}

func Verify(ctx context.Context, email, otp string) (Result, error) {
	return DefaultClient().post(ctx, verifyPath, map[string]string{
		"email": email,
		"otp":   otp,
	})
}

func SendVerification(ctx context.Context, email string) (Result, error) {
	return DefaultClient().post(ctx, resendPath, map[string]string{
		"email": email,
	})
}

func (c *Client) post(ctx context.Context, path string, body any) (Result, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return Result{}, ErrNotConfigured
	}
	origin := strings.TrimRight(strings.TrimSpace(c.Origin), "/")
	if origin == "" {
		origin = "https://vectoree.ai"
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Result{}, err
	}
	endpoint, err := url.Parse(origin + path)
	if err != nil {
		return Result{}, err
	}
	query := endpoint.Query()
	query.Set("client_type", "server")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("User-Agent", "coze-studio")

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, err
	}
	result := interpret(resp.StatusCode, raw)
	result.Issuer = origin
	return result, nil
}

func interpret(status int, raw []byte) Result {
	result := Result{StatusCode: status}
	if len(bytes.TrimSpace(raw)) == 0 {
		return result
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		result.Message = "vectoree auth request failed"
		return result
	}
	result.AccessToken = firstToken(payload)
	result.RequireEmailVerification = boolField(payload, "requireEmailVerification")
	result.Message = messageFrom(payload)
	if user := sessionUser(payload); user != nil {
		result.Subject = stringField(user, "id")
		result.Email = stringField(user, "email")
		verified, ok := user["emailVerified"].(bool)
		result.EmailVerified = !ok || verified
	}
	return result
}

func sessionUser(payload map[string]any) map[string]any {
	if user, ok := payload["user"].(map[string]any); ok {
		return user
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return nil
	}
	user, _ := data["user"].(map[string]any)
	return user
}

func firstToken(payload map[string]any) string {
	if token := stringField(payload, "accessToken"); token != "" {
		return token
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return ""
	}
	return stringField(data, "accessToken")
}

func stringField(payload map[string]any, key string) string {
	value, ok := payload[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func boolField(payload map[string]any, key string) bool {
	value, ok := payload[key].(bool)
	return ok && value
}

func messageFrom(payload map[string]any) string {
	for _, key := range []string{"message", "msg", "error"} {
		switch value := payload[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		case map[string]any:
			if nested := messageFrom(value); nested != "" {
				return nested
			}
		}
	}
	return ""
}
