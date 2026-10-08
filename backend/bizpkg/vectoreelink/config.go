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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const (
	envAPIURL  = "VECTOREE_API_URL"
	envAPIKey  = "VECTOREE_API_KEY"
	envAPIBase = "VECTOREE_API_BASE"
	envProject = "VECTOREE_PROJECT_ID"
)

type linkedConfig struct {
	APIURL       string `json:"apiUrl,omitempty"`
	AccessToken  string `json:"accessToken,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	APIKey       string `json:"apiKey,omitempty"`
	ProjectID    string `json:"projectId,omitempty"`
	ProjectName  string `json:"projectName,omitempty"`
	KeyID        string `json:"keyId,omitempty"`
}

type PublicStatus struct {
	Linked      bool   `json:"linked"`
	ProjectName string `json:"projectName,omitempty"`
	APIURL      string `json:"apiUrl,omitempty"`
}

func ResolveDataRoot() (string, error) {
	dir := strings.TrimSpace(os.Getenv("VECTOREE_LINK_DATA_DIR"))
	if dir == "" {
		dir = "/app/data"
	}
	if err := ensureWritable(dir); err != nil {
		return "", fmt.Errorf("vectoree link data directory %s is not writable: %w. Set VECTOREE_LINK_DATA_DIR to a writable path and mount it in Docker (for example ./data/vectoree:/app/data)", dir, err)
	}
	return dir, nil
}

func ensureWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	probe := filepath.Join(dir, ".write-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return err
	}
	return os.Remove(probe)
}

func configPath(root string) string {
	return filepath.Join(root, ".vectoree", "config.json")
}

// ProjectAPICredentials returns the linked project origin and API key.
// The key is server-only and must never be sent to the browser.
func ProjectAPICredentials() (apiURL, apiKey string) {
	apiURL = strings.TrimSpace(os.Getenv(envAPIURL))
	apiKey = strings.TrimSpace(os.Getenv(envAPIKey))

	roots := make([]string, 0, 2)
	if root := strings.TrimSpace(os.Getenv("VECTOREE_LINK_DATA_DIR")); root != "" {
		roots = append(roots, root)
	}
	roots = append(roots, "/app/data", ".")
	for _, root := range roots {
		if apiKey != "" && apiURL != "" {
			break
		}
		cfg := readConfig(root)
		if apiKey == "" {
			apiKey = cfg.APIKey
		}
		if apiURL == "" {
			apiURL = cfg.APIURL
		}
	}

	normalized, err := normalizeAPIURL(apiURL)
	if err != nil {
		return defaultAPIOrigin, apiKey
	}
	return normalized, apiKey
}

func readConfig(root string) linkedConfig {
	raw, err := os.ReadFile(configPath(root))
	if err != nil {
		return linkedConfig{}
	}
	var record map[string]any
	if err := json.Unmarshal(raw, &record); err != nil {
		return linkedConfig{}
	}
	return linkedConfig{
		APIURL:       firstString(record, "apiUrl", "api_url"),
		AccessToken:  firstString(record, "accessToken", "access_token"),
		RefreshToken: firstString(record, "refreshToken", "refresh_token"),
		APIKey:       firstString(record, "apiKey", "api_key"),
		ProjectID:    firstString(record, "projectId", "project_id"),
		ProjectName:  firstString(record, "projectName", "project_name"),
		KeyID:        firstString(record, "keyId", "key_id"),
	}
}

func firstString(record map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := record[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func writeLinkedConfig(root string, cfg linkedConfig) error {
	dir := filepath.Join(root, ".vectoree")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	target := configPath(root)
	tmp := filepath.Join(dir, fmt.Sprintf(".config.json.%d.tmp", os.Getpid()))
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Chmod(target, 0o600); err != nil {
		return err
	}
	return nil
}

func readDeviceID(root string) (string, error) {
	file := filepath.Join(root, ".vectoree", "device-id")
	if raw, err := os.ReadFile(file); err == nil {
		existing := strings.TrimSpace(string(raw))
		if len(existing) >= 8 {
			if len(existing) > 128 {
				existing = existing[:128]
			}
			return existing, nil
		}
	}
	id := uuid.NewString()
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(file, []byte(id+"\n"), 0o600); err != nil {
		return "", err
	}
	_ = os.Chmod(dir, 0o700)
	_ = os.Chmod(file, 0o600)
	return id, nil
}

func syncCredentialsEnv(root string, cfg linkedConfig) {
	updates := map[string]string{
		envAPIURL:  cfg.APIURL,
		envAPIKey:  cfg.APIKey,
		envAPIBase: strings.TrimRight(cfg.APIURL, "/") + "/api/v1",
		envProject: cfg.ProjectID,
	}
	_ = os.Setenv(envAPIURL, updates[envAPIURL])
	_ = os.Setenv(envAPIKey, updates[envAPIKey])
	_ = os.Setenv(envAPIBase, updates[envAPIBase])
	_ = os.Setenv(envProject, updates[envProject])

	envCopy := filepath.Join(root, ".vectoree", "env")
	_ = upsertEnvFile(envCopy, updates)
	if fileWritable("/app/.env") {
		_ = upsertEnvFile("/app/.env", updates)
	}
}

func fileWritable(file string) bool {
	info, err := os.Stat(file)
	if err != nil || info.IsDir() {
		return false
	}
	f, err := os.OpenFile(file, os.O_RDWR, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func upsertEnvFile(file string, updates map[string]string) error {
	current := ""
	if raw, err := os.ReadFile(file); err == nil {
		current = string(raw)
	}
	next := upsertEnv(current, updates)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(file, []byte(next), 0o600); err != nil {
		return err
	}
	_ = os.Chmod(file, 0o600)
	return nil
}

func upsertEnv(content string, updates map[string]string) string {
	lines := strings.Split(content, "\n")
	seen := map[string]bool{}
	next := make([]string, 0, len(lines)+len(updates))
	for _, line := range lines {
		key, ok := envAssignmentKey(line)
		if !ok {
			next = append(next, line)
			continue
		}
		value, exists := updates[key]
		if !exists {
			next = append(next, line)
			continue
		}
		seen[key] = true
		prefix := ""
		if strings.HasPrefix(strings.TrimSpace(line), "export ") {
			prefix = "export "
		}
		next = append(next, prefix+key+"="+value)
	}
	extra := make([]string, 0, len(updates))
	for key := range updates {
		if !seen[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	for _, key := range extra {
		next = append(next, key+"="+updates[key])
	}
	for len(next) > 0 && next[len(next)-1] == "" {
		next = next[:len(next)-1]
	}
	return strings.Join(next, "\n") + "\n"
}

func envAssignmentKey(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	trimmed = strings.TrimPrefix(trimmed, "export ")
	key, _, ok := strings.Cut(trimmed, "=")
	if !ok {
		return "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false
	}
	return key, true
}

func parseEnvFile(file string) map[string]string {
	raw, err := os.ReadFile(file)
	if err != nil {
		return map[string]string{}
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, ok := envAssignmentKey(line)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimPrefix(trimmed, "export ")
		_, value, _ := strings.Cut(trimmed, "=")
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
				(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}
		}
		if key != "" {
			out[key] = value
		}
	}
	return out
}

func nonEmptyKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return value
}

func (s *Service) PublicStatus() PublicStatus {
	if cfg, ok := s.linkedFromConfig(); ok {
		return PublicStatus{
			Linked:      true,
			ProjectName: cfg.ProjectName,
			APIURL:      cfg.APIURL,
		}
	}
	if key, apiURL := linkedFromEnv(s.roots()); key != "" {
		status := PublicStatus{Linked: true}
		if apiURL != "" {
			status.APIURL = apiURL
		}
		return status
	}
	return PublicStatus{Linked: false}
}

func (s *Service) linkedFromConfig() (linkedConfig, bool) {
	for _, root := range s.roots() {
		cfg := readConfig(root)
		if nonEmptyKey(cfg.APIKey) != "" {
			return cfg, true
		}
	}
	return linkedConfig{}, false
}

func (s *Service) roots() []string {
	if s != nil && s.dataRoot != "" {
		return []string{s.dataRoot}
	}
	if dir := strings.TrimSpace(os.Getenv("VECTOREE_LINK_DATA_DIR")); dir != "" {
		return []string{dir}
	}
	return []string{"/app/data"}
}

func linkedFromEnv(roots []string) (apiKey string, apiURL string) {
	if key := nonEmptyKey(os.Getenv(envAPIKey)); key != "" {
		return key, nonEmptyKey(os.Getenv(envAPIURL))
	}
	files := []string{"/app/.env", ".env"}
	for _, root := range roots {
		files = append(files, filepath.Join(root, ".vectoree", "env"))
	}
	for _, file := range files {
		env := parseEnvFile(file)
		if key := nonEmptyKey(env[envAPIKey]); key != "" {
			return key, nonEmptyKey(env[envAPIURL])
		}
	}
	return "", ""
}
