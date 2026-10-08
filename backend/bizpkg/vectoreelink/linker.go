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

package vectoreelink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	linkTimeout    = 3 * time.Minute
	requestTimeout = 30 * time.Second
	vectoreeUA     = "VectoreeCLI"
	statusIdle     = "idle"
	statusPending  = "pending"
	statusLinked   = "linked"
	statusError    = "error"
)

var keyScopes = []string{
	"gateway:chat",
	"gateway:models",
	"tools:*",
	"database:*",
	"storage:*",
	"auth:*",
}

type PollView struct {
	Status       string `json:"status"`
	AuthorizeURL string `json:"authorizeUrl,omitempty"`
	Message      string `json:"message,omitempty"`
	ProjectName  string `json:"projectName,omitempty"`
	APIURL       string `json:"apiUrl,omitempty"`
}

type pendingAuth struct {
	generation  int
	apiURL      string
	projectID   string
	verifier    string
	state       string
	redirectURI string
}

type seedFunc func(ctx context.Context, apiURL, apiKey string) error

type Service struct {
	mu         sync.Mutex
	generation int
	pending    *pendingAuth
	snapshot   PollView
	dataRoot   string
	client     *http.Client
	seed       seedFunc
	timeout    time.Duration
}

func NewService() *Service {
	return &Service{
		client:  &http.Client{Timeout: requestTimeout},
		seed:    seedVectoreeModel,
		timeout: linkTimeout,
	}
}

func (s *Service) root() (string, error) {
	if s.dataRoot != "" {
		return s.dataRoot, nil
	}
	return ResolveDataRoot()
}

func (s *Service) Poll() PollView {
	s.mu.Lock()
	snap := s.snapshot
	s.mu.Unlock()
	if snap.Status == statusPending || snap.Status == statusError {
		return snap
	}
	pub := s.PublicStatus()
	if pub.Linked {
		view := PollView{
			Status:      statusLinked,
			APIURL:      pub.APIURL,
			ProjectName: pub.ProjectName,
		}
		if snap.Status == statusLinked {
			view.Message = snap.Message
		}
		return view
	}
	if snap.Status == "" {
		return PollView{Status: statusIdle}
	}
	return snap
}

func (s *Service) Start(apiURLInput, projectInput, publicOrigin string) (PollView, error) {
	apiURL, err := normalizeAPIURL(apiURLInput)
	if err != nil {
		return PollView{}, err
	}
	projectID, err := ExtractProjectID(projectInput)
	if err != nil {
		return PollView{}, err
	}
	origin := strings.TrimRight(strings.TrimSpace(publicOrigin), "/")
	if origin == "" {
		return PollView{}, fmt.Errorf("public origin is required for the OAuth callback")
	}
	if _, err := s.root(); err != nil {
		return PollView{}, err
	}
	verifier, challenge, err := pkcePair()
	if err != nil {
		return PollView{}, err
	}
	state, err := randomState()
	if err != nil {
		return PollView{}, err
	}
	redirectURI := origin + "/api/vectoree/link/callback"
	authorize, err := url.Parse(apiURL + "/api/system/auth/cli/authorize")
	if err != nil {
		return PollView{}, err
	}
	query := authorize.Query()
	query.Set("response_type", "code")
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)
	authorize.RawQuery = query.Encode()

	s.mu.Lock()
	s.generation++
	gen := s.generation
	s.pending = &pendingAuth{
		generation:  gen,
		apiURL:      apiURL,
		projectID:   projectID,
		verifier:    verifier,
		state:       state,
		redirectURI: redirectURI,
	}
	s.snapshot = PollView{
		Status:       statusPending,
		AuthorizeURL: authorize.String(),
		APIURL:       apiURL,
	}
	view := s.snapshot
	timeout := s.timeout
	s.mu.Unlock()

	time.AfterFunc(timeout, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.pending != nil && s.pending.generation == gen {
			s.pending = nil
			s.snapshot = PollView{
				Status:  statusError,
				APIURL:  apiURL,
				Message: "Console login timed out. Try connecting again.",
			}
		}
	})
	return view, nil
}

func (s *Service) Complete(ctx context.Context, code, state, oauthError string) error {
	if strings.TrimSpace(oauthError) != "" {
		s.failCurrent("Vectoree login was cancelled")
		return fmt.Errorf("Vectoree login was cancelled")
	}
	code = strings.TrimSpace(code)
	s.mu.Lock()
	if s.pending == nil || s.pending.state != state || code == "" {
		s.mu.Unlock()
		return fmt.Errorf("invalid OAuth callback")
	}
	job := *s.pending
	s.pending = nil
	s.mu.Unlock()

	session, err := s.exchangeCode(ctx, job, code)
	if err != nil {
		return s.fail(job, err)
	}
	projectName, err := s.lookupProjectName(ctx, job, session.AccessToken)
	if err != nil {
		return s.fail(job, err)
	}
	root, err := s.root()
	if err != nil {
		return s.fail(job, err)
	}
	minted, err := s.mintProjectKey(ctx, job, session.AccessToken, root)
	if err != nil {
		return s.fail(job, err)
	}
	cfg := linkedConfig{
		APIURL:       job.apiURL,
		AccessToken:  session.AccessToken,
		RefreshToken: session.RefreshToken,
		APIKey:       minted.APIKey,
		ProjectID:    job.projectID,
		ProjectName:  projectName,
		KeyID:        minted.KeyID,
	}
	if err := writeLinkedConfig(root, cfg); err != nil {
		return s.fail(job, err)
	}
	syncCredentialsEnv(root, cfg)

	message := ""
	if s.seed != nil {
		if err := s.seed(ctx, job.apiURL, minted.APIKey); err != nil {
			message = "Linked, but saving the Vectoree model failed: " + redact(err.Error())
			logs.Warnf("vectoree model seed failed: %s", redact(err.Error()))
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation != job.generation {
		return nil
	}
	s.snapshot = PollView{
		Status:      statusLinked,
		APIURL:      job.apiURL,
		ProjectName: projectName,
		Message:     message,
	}
	logs.Infof("vectoree link succeeded for project %s", job.projectID)
	return nil
}

func (s *Service) fail(job pendingAuth, err error) error {
	message := redact(err.Error())
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation == job.generation {
		s.snapshot = PollView{
			Status:  statusError,
			APIURL:  job.apiURL,
			Message: message,
		}
	}
	logs.Warnf("vectoree link failed: %s", message)
	return fmt.Errorf("%s", message)
}

func (s *Service) failCurrent(message string) {
	message = redact(message)
	s.mu.Lock()
	defer s.mu.Unlock()
	apiURL := s.snapshot.APIURL
	if s.pending != nil {
		apiURL = s.pending.apiURL
		s.pending = nil
	}
	s.snapshot = PollView{Status: statusError, APIURL: apiURL, Message: message}
}

type sessionTokens struct {
	AccessToken  string
	RefreshToken string
}

type mintedKey struct {
	APIKey string
	KeyID  string
}

func (s *Service) exchangeCode(ctx context.Context, job pendingAuth, code string) (sessionTokens, error) {
	raw, err := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"code_verifier": job.verifier,
		"redirect_uri":  job.redirectURI,
	})
	if err != nil {
		return sessionTokens{}, err
	}
	data, err := s.postJSON(ctx, job.apiURL+"/api/system/auth/cli/token", raw, nil, "Console login failed")
	if err != nil {
		return sessionTokens{}, err
	}
	record, _ := data.(map[string]any)
	token, _ := record["accessToken"].(string)
	if strings.TrimSpace(token) == "" {
		return sessionTokens{}, fmt.Errorf("console login did not return a session")
	}
	refresh, _ := record["refreshToken"].(string)
	return sessionTokens{AccessToken: token, RefreshToken: refresh}, nil
}

func (s *Service) lookupProjectName(ctx context.Context, job pendingAuth, accessToken string) (string, error) {
	data, err := s.getJSON(ctx, job.apiURL+"/api/projects", accessToken, nil, "Could not list projects")
	if err != nil {
		return "", err
	}
	projects, ok := data.([]any)
	if !ok {
		return "", fmt.Errorf("could not list projects")
	}
	for _, item := range projects {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := record["id"].(string)
		if id != job.projectID {
			continue
		}
		name, _ := record["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			return job.projectID, nil
		}
		return name, nil
	}
	return "", fmt.Errorf("that project was not found on this Vectoree account")
}

func (s *Service) mintProjectKey(ctx context.Context, job pendingAuth, accessToken, root string) (mintedKey, error) {
	deviceID, err := readDeviceID(root)
	if err != nil {
		return mintedKey{}, err
	}
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "coze"
	}
	name := "CLI — " + host
	if len(name) > 128 {
		name = name[:128]
	}
	raw, err := json.Marshal(map[string]any{
		"name":            name,
		"projectId":       job.projectID,
		"deviceId":        deviceID,
		"localFolderPath": root,
		"scopes":          keyScopes,
	})
	if err != nil {
		return mintedKey{}, err
	}
	data, err := s.postJSON(ctx, job.apiURL+"/api/gateway/keys", raw, map[string]string{
		"Authorization": "Bearer " + accessToken,
		"X-Project-Id":  job.projectID,
		"User-Agent":    vectoreeUA,
	}, "Could not create a project key")
	if err != nil {
		return mintedKey{}, err
	}
	record, _ := data.(map[string]any)
	apiKey, _ := record["apiKey"].(string)
	if !strings.HasPrefix(apiKey, "sk-ve-") {
		return mintedKey{}, fmt.Errorf("Vectoree did not return a project API key")
	}
	keyID, _ := record["id"].(string)
	return mintedKey{APIKey: apiKey, KeyID: keyID}, nil
}

func (s *Service) postJSON(ctx context.Context, endpoint string, body []byte, headers map[string]string, fallback string) (any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", vectoreeUA)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return s.do(req, fallback)
}

func (s *Service) getJSON(ctx context.Context, endpoint, accessToken string, headers map[string]string, fallback string) (any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", vectoreeUA)
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return s.do(req, fallback)
}

func (s *Service) do(req *http.Request, fallback string) (any, error) {
	client := s.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s", redact(err.Error()))
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	data := decodeBody(payload)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", readErrorMessage(data, fmt.Sprintf("%s (%d)", fallback, resp.StatusCode)))
	}
	return data, nil
}

func decodeBody(payload []byte) any {
	text := strings.TrimSpace(string(payload))
	if text == "" {
		return nil
	}
	var data any
	if err := json.Unmarshal(payload, &data); err != nil {
		if len(text) > 500 {
			text = text[:500]
		}
		return map[string]any{"message": text}
	}
	return data
}

func readErrorMessage(data any, fallback string) string {
	record, ok := data.(map[string]any)
	if !ok {
		return fallback
	}
	if message, ok := record["message"].(string); ok && strings.TrimSpace(message) != "" {
		return redact(message)
	}
	switch errValue := record["error"].(type) {
	case string:
		if strings.TrimSpace(errValue) != "" {
			return redact(errValue)
		}
	case map[string]any:
		if message, ok := errValue["message"].(string); ok && strings.TrimSpace(message) != "" {
			return redact(message)
		}
	}
	return fallback
}
