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
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInterpretAuthResponses(t *testing.T) {
	session := interpret(http.StatusOK, []byte(`{"accessToken":"app-jwt","requireEmailVerification":false}`))
	if !session.Authenticated() || session.NeedsVerification() || session.AccessToken != "app-jwt" {
		t.Fatalf("session = %#v", session)
	}

	pending := interpret(http.StatusOK, []byte(`{"requireEmailVerification":true}`))
	if pending.Authenticated() || !pending.NeedsVerification() {
		t.Fatalf("pending = %#v", pending)
	}

	forbidden := interpret(http.StatusForbidden, []byte(`{"message":"email is not verified"}`))
	if !forbidden.NeedsVerification() || forbidden.SafeMessage() != "email is not verified" {
		t.Fatalf("forbidden = %#v", forbidden)
	}

	nested := interpret(http.StatusOK, []byte(`{"data":{"accessToken":"nested"}}`))
	if nested.AccessToken != "nested" {
		t.Fatalf("nested token = %q", nested.AccessToken)
	}

	leaky := interpret(http.StatusBadRequest, []byte(`{"message":"bad sk-ve-v1-secret"}`))
	if leaky.SafeMessage() != "vectoree auth request failed" {
		t.Fatalf("safe message = %q", leaky.SafeMessage())
	}
}

func TestInterpretReadsTheVectoreeUser(t *testing.T) {
	session := interpret(http.StatusOK, []byte(`{"accessToken":"app-jwt","user":{"id":"vt_1","email":"a@example.com","emailVerified":false}}`))
	if session.Subject != "vt_1" || session.Email != "a@example.com" || session.EmailVerified {
		t.Fatalf("session = %#v", session)
	}

	nested := interpret(http.StatusOK, []byte(`{"data":{"accessToken":"t","user":{"id":"vt_2","email":"b@example.com"}}}`))
	if nested.Subject != "vt_2" || !nested.EmailVerified {
		t.Fatalf("nested = %#v", nested)
	}
}

func TestPostRecordsTheIssuer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"t","user":{"id":"vt_1","email":"a@example.com"}}`))
	}))
	defer server.Close()

	client := &Client{HTTP: server.Client(), Origin: server.URL + "/", APIKey: "sk-ve-v1-test"}
	result, err := client.post(context.Background(), loginPath, map[string]string{"email": "a@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Issuer != server.URL {
		t.Fatalf("issuer = %q, want %q", result.Issuer, server.URL)
	}
}

func TestPostSendsServerKeyAndQuery(t *testing.T) {
	var sawAuth, sawQuery, sawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		sawQuery = r.URL.RawQuery
		raw, _ := io.ReadAll(r.Body)
		sawBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"requireEmailVerification":true}`))
	}))
	defer server.Close()

	client := &Client{HTTP: server.Client(), Origin: server.URL, APIKey: "sk-ve-v1-test"}
	result, err := client.post(context.Background(), registerPath, map[string]string{
		"email":    "user@example.com",
		"password": "Passw0rd!",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.NeedsVerification() {
		t.Fatalf("result = %#v", result)
	}
	if sawAuth != "Bearer sk-ve-v1-test" {
		t.Fatalf("authorization = %q", sawAuth)
	}
	if !strings.Contains(sawQuery, "client_type=server") {
		t.Fatalf("query = %q", sawQuery)
	}
	if !strings.Contains(sawBody, "user@example.com") || strings.Contains(sawAuth, "Passw0rd") {
		t.Fatalf("body = %s", sawBody)
	}
}

func TestPostRequiresAPIKey(t *testing.T) {
	client := &Client{Origin: "https://vectoree.ai"}
	_, err := client.post(context.Background(), loginPath, map[string]string{"email": "a@b.co"})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v", err)
	}
}
