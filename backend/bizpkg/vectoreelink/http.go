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
	"html"
	"net/http"
	"os"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
)

var defaultService = NewService()

// Linked reports whether this server already holds a Vectoree project key.
func Linked() bool {
	return defaultService.PublicStatus().Linked
}

// Register adds the link routes. startGuards run before link/start, so the
// caller can require an admin session once a project is linked.
func Register(r *server.Hertz, startGuards ...app.HandlerFunc) {
	r.GET("/api/vectoree/link-status", func(_ context.Context, ctx *app.RequestContext) {
		writeJSON(ctx, http.StatusOK, defaultService.PublicStatus())
	})
	r.GET("/api/vectoree/link", func(_ context.Context, ctx *app.RequestContext) {
		writeJSON(ctx, http.StatusOK, defaultService.Poll())
	})
	r.POST("/api/vectoree/link/start", append(startGuards, func(_ context.Context, ctx *app.RequestContext) {
		var body struct {
			APIURL    string `json:"apiUrl"`
			ProjectID string `json:"projectId"`
		}
		if err := ctx.BindJSON(&body); err != nil {
			writeError(ctx, http.StatusBadRequest, "Expected a JSON object")
			return
		}
		view, err := defaultService.Start(body.APIURL, body.ProjectID, publicOrigin(ctx))
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not writable") {
				status = http.StatusInternalServerError
			}
			writeError(ctx, status, err.Error())
			return
		}
		writeJSON(ctx, http.StatusOK, view)
	})...)
	r.GET("/api/vectoree/link/callback", func(c context.Context, ctx *app.RequestContext) {
		err := defaultService.Complete(
			c,
			string(ctx.Query("code")),
			string(ctx.Query("state")),
			string(ctx.Query("error")),
		)
		if err != nil {
			writeCallbackHTML(ctx, http.StatusBadRequest, "Vectoree login failed", err.Error())
			return
		}
		writeCallbackHTML(ctx, http.StatusOK, "Vectoree connected", "You can close this window and return to Coze Studio.")
	})
}

func publicOrigin(ctx *app.RequestContext) string {
	if origin := strings.TrimRight(strings.TrimSpace(os.Getenv("VECTOREE_LINK_PUBLIC_ORIGIN")), "/"); origin != "" {
		return origin
	}
	scheme := "http"
	if forwarded := strings.TrimSpace(string(ctx.GetHeader("X-Forwarded-Proto"))); forwarded != "" {
		scheme = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host := strings.TrimSpace(string(ctx.Host()))
	if host == "" {
		host = "localhost:8888"
	}
	return scheme + "://" + host
}

func writeJSON(ctx *app.RequestContext, status int, payload any) {
	ctx.JSON(status, payload)
}

func writeError(ctx *app.RequestContext, status int, message string) {
	ctx.JSON(status, map[string]string{"message": redact(message)})
}

func writeCallbackHTML(ctx *app.RequestContext, status int, title, message string) {
	body := "<!doctype html><html><head><meta charset=\"utf-8\"><title>" + html.EscapeString(title) +
		"</title></head><body style=\"font-family:sans-serif\"><h1>" + html.EscapeString(title) +
		"</h1><p>" + html.EscapeString(redact(message)) + "</p></body></html>"
	ctx.Data(status, "text/html; charset=utf-8", []byte(body))
}
