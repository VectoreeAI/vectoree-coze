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
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	"github.com/coze-dev/coze-studio/backend/bizpkg/config/modelmgr"
)

func TestExtractProjectID(t *testing.T) {
	id := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	got, err := ExtractProjectID("https://vectoree.ai/dashboard/project/" + id + "/settings")
	if err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Fatalf("got %s", got)
	}
	if _, err := ExtractProjectID("not-a-project"); err == nil {
		t.Fatal("expected uuid error")
	}
}

func TestNormalizeAPIURL(t *testing.T) {
	got, err := normalizeAPIURL(" https://vectoree.ai/ ")
	if err != nil || got != "https://vectoree.ai" {
		t.Fatalf("got %s err %v", got, err)
	}
	if _, err := normalizeAPIURL("https://vectoree.ai/api/v1"); err == nil {
		t.Fatal("expected origin-only error")
	}
	if _, err := normalizeAPIURL("https://user:pass@vectoree.ai"); err == nil {
		t.Fatal("expected credential error")
	}
}

func TestPublicStatusFailClosedWithoutSecrets(t *testing.T) {
	t.Setenv(envAPIKey, "")
	t.Setenv(envAPIURL, "")
	root := t.TempDir()
	svc := &Service{dataRoot: root}
	status := svc.PublicStatus()
	if status.Linked {
		t.Fatal("empty data root should be unlinked")
	}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "apiKey") || strings.Contains(string(raw), "accessToken") {
		t.Fatalf("status leaked secrets: %s", raw)
	}

	t.Setenv(envAPIKey, "")
	t.Setenv(envAPIURL, "")
	if err := os.WriteFile(filepath.Join(root, ".vectoree-placeholder"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := linkedConfig{
		APIURL:      "https://vectoree.ai",
		APIKey:      "sk-ve-v1-secret",
		ProjectName: "Demo",
		AccessToken: "jwt-secret",
	}
	if err := os.MkdirAll(filepath.Join(root, ".vectoree"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeLinkedConfig(root, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(configPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode %o", info.Mode().Perm())
	}
	linked := svc.PublicStatus()
	if !linked.Linked || linked.ProjectName != "Demo" || linked.APIURL != "https://vectoree.ai" {
		t.Fatalf("status %+v", linked)
	}
	encoded, _ := json.Marshal(linked)
	if strings.Contains(string(encoded), "sk-ve-") || strings.Contains(string(encoded), "jwt-secret") {
		t.Fatalf("linked status leaked secrets: %s", encoded)
	}
}

func TestLinkFlowPersistsAndRedacts(t *testing.T) {
	projectID := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	apiKey := "sk-ve-v1-test-key"
	var sawUA, sawProject string
	var tokenBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/system/auth/cli/token" && r.Method == http.MethodPost:
			_ = json.NewDecoder(r.Body).Decode(&tokenBody)
			_, _ = io.WriteString(w, `{"accessToken":"access-token","refreshToken":"refresh-token"}`)
		case r.URL.Path == "/api/projects" && r.Method == http.MethodGet:
			if r.Header.Get("Authorization") != "Bearer access-token" {
				http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `[{"id":"`+projectID+`","name":"Demo"}]`)
		case r.URL.Path == "/api/gateway/keys" && r.Method == http.MethodPost:
			sawUA = r.Header.Get("User-Agent")
			sawProject = r.Header.Get("X-Project-Id")
			_, _ = io.WriteString(w, `{"id":"key-1","apiKey":"`+apiKey+`"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	root := t.TempDir()
	t.Setenv(envAPIKey, "")
	t.Setenv(envAPIURL, "")
	t.Setenv(envAPIBase, "")
	t.Setenv(envProject, "")
	seeded := ""
	svc := &Service{
		dataRoot: root,
		client:   srv.Client(),
		timeout:  time.Minute,
		seed: func(_ context.Context, apiURL, key string) error {
			seeded = apiURL + "|" + key
			return nil
		},
	}
	view, err := svc.Start(srv.URL, "https://vectoree.ai/projects/"+projectID, "http://localhost:8888")
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != statusPending || !strings.Contains(view.AuthorizeURL, "/api/system/auth/cli/authorize") {
		t.Fatalf("start view %+v", view)
	}
	if strings.Contains(view.AuthorizeURL, "code_verifier") {
		t.Fatal("authorize URL included the verifier")
	}
	state := svc.pending.state
	if err := svc.Complete(context.Background(), "auth-code", state, ""); err != nil {
		t.Fatal(err)
	}
	if tokenBody["code"] != "auth-code" || tokenBody["redirect_uri"] != "http://localhost:8888/api/vectoree/link/callback" {
		t.Fatalf("token body %+v", tokenBody)
	}
	if sawUA != vectoreeUA || sawProject != projectID {
		t.Fatalf("ua %s project %s", sawUA, sawProject)
	}
	if seeded != srv.URL+"|"+apiKey {
		t.Fatalf("seed %s", seeded)
	}
	poll := svc.Poll()
	encoded, _ := json.Marshal(poll)
	if poll.Status != statusLinked || poll.ProjectName != "Demo" {
		t.Fatalf("poll %+v", poll)
	}
	if strings.Contains(string(encoded), apiKey) || strings.Contains(string(encoded), "access-token") {
		t.Fatalf("poll leaked secrets: %s", encoded)
	}
	if _, err := os.Stat(filepath.Join(root, ".vectoree", "device-id")); err != nil {
		t.Fatal(err)
	}
	envText, err := os.ReadFile(filepath.Join(root, ".vectoree", "env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(envText), "VECTOREE_API_BASE="+srv.URL+"/api/v1") {
		t.Fatalf("env copy %s", envText)
	}
	if err := svc.Complete(context.Background(), "auth-code", "wrong", ""); err == nil {
		t.Fatal("expected invalid callback")
	}
}

func TestSeedFailureStaysLinkedAndRedactsKey(t *testing.T) {
	projectID := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	apiKey := "sk-ve-v1-secret-value"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/system/auth/cli/token":
			_, _ = io.WriteString(w, `{"accessToken":"access-token"}`)
		case "/api/projects":
			_, _ = io.WriteString(w, `[{"id":"`+projectID+`","name":"Demo"}]`)
		case "/api/gateway/keys":
			_, _ = io.WriteString(w, `{"id":"key-1","apiKey":"`+apiKey+`"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	root := t.TempDir()
	t.Setenv(envAPIKey, "")
	svc := &Service{
		dataRoot: root,
		client:   srv.Client(),
		timeout:  time.Minute,
		seed: func(context.Context, string, string) error {
			return errWithKey("model save failed " + apiKey)
		},
	}
	if _, err := svc.Start(srv.URL, projectID, "http://localhost:8888"); err != nil {
		t.Fatal(err)
	}
	state := svc.pending.state
	if err := svc.Complete(context.Background(), "code", state, ""); err != nil {
		t.Fatal(err)
	}
	poll := svc.Poll()
	if poll.Status != statusLinked {
		t.Fatalf("status %s", poll.Status)
	}
	if strings.Contains(poll.Message, apiKey) || !strings.Contains(poll.Message, "[redacted]") {
		t.Fatalf("message %s", poll.Message)
	}
	if !svc.PublicStatus().Linked {
		t.Fatal("credentials should stay linked when seed fails")
	}
}

func TestIsVectoreeModel(t *testing.T) {
	item := &modelmgr.Model{Model: &adminconfig.Model{
		DisplayInfo: &adminconfig.DisplayInfo{Name: "Vectoree"},
		Connection: &adminconfig.Connection{BaseConnInfo: &adminconfig.BaseConnectionInfo{
			BaseURL: "https://vectoree.ai/api/v1",
			Model:   "other",
		}},
	}}
	if !isVectoreeModel(item, "https://vectoree.ai/api/v1") {
		t.Fatal("expected display name match")
	}
	item.Connection.BaseConnInfo.Model = vectoreeModelID
	item.DisplayInfo.Name = "Custom"
	item.Connection.BaseConnInfo.BaseURL = "https://example.com/api/v1"
	if !isVectoreeModel(item, "https://vectoree.ai/api/v1") {
		t.Fatal("expected model id match")
	}
}

type errWithKey string

func (e errWithKey) Error() string { return string(e) }

func TestUpsertEnvKeepsExportPrefix(t *testing.T) {
	got := upsertEnv("export MYSQL_USER=coze\nexport VECTOREE_API_KEY=\"\"\n", map[string]string{
		envAPIKey: "sk-ve-v1-next",
	})
	if !strings.Contains(got, "export VECTOREE_API_KEY=sk-ve-v1-next\n") {
		t.Fatalf("upsert %q", got)
	}
	if strings.Contains(got, `VECTOREE_API_KEY=""`) {
		t.Fatalf("left empty key %q", got)
	}
	if !strings.Contains(got, "export MYSQL_USER=coze\n") {
		t.Fatalf("dropped other keys %q", got)
	}
}

func TestResolveDataRootNotWritable(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VECTOREE_LINK_DATA_DIR", file)
	_, err := ResolveDataRoot()
	if err == nil || !strings.Contains(err.Error(), "not writable") {
		t.Fatalf("err %v", err)
	}
}
