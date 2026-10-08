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
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const defaultAPIOrigin = "https://vectoree.ai"

var projectUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var projectUUIDFind = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
var secretKeyPattern = regexp.MustCompile(`sk-ve-[A-Za-z0-9_\-]+`)

func pkcePair() (verifier string, challenge string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func ExtractProjectID(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", fmt.Errorf("projectId must be a UUID from the Vectoree console")
	}
	if projectUUID.MatchString(trimmed) {
		return strings.ToLower(trimmed), nil
	}
	found := projectUUIDFind.FindString(trimmed)
	if found == "" {
		return "", fmt.Errorf("projectId must be a UUID from the Vectoree console")
	}
	return strings.ToLower(found), nil
}

func normalizeAPIURL(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		trimmed = defaultAPIOrigin
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("apiUrl must be an http(s) origin, for example https://vectoree.ai")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("apiUrl must use http or https")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("apiUrl must not include credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("apiUrl must be an origin, without a query or hash")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("use the API origin only, for example https://vectoree.ai")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func redact(message string) string {
	return secretKeyPattern.ReplaceAllString(message, "[redacted]")
}
