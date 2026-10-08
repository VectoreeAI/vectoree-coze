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

package middleware

import (
	"context"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/ut"

	appuser "github.com/coze-dev/coze-studio/backend/application/user"
	"github.com/coze-dev/coze-studio/backend/domain/user/entity"
	userservice "github.com/coze-dev/coze-studio/backend/domain/user/service"
)

type noSessionDomain struct{ userservice.User }

func (noSessionDomain) ValidateSession(context.Context, string) (*entity.Session, bool, error) {
	return nil, false, nil
}

func runRelinkGuard(t *testing.T, linked bool, cookie string) (status int, reachedHandler bool) {
	t.Helper()
	ctx := ut.CreateUtRequestContext(http.MethodPost, "/api/vectoree/link/start", nil)
	if cookie != "" {
		ctx.Request.Header.SetCookie(entity.SessionKey, cookie)
	}
	ctx.SetHandlers(app.HandlersChain{
		VectoreeRelinkGuardMW(func() bool { return linked }),
		func(context.Context, *app.RequestContext) { reachedHandler = true },
	})
	ctx.Next(context.Background())
	return ctx.Response.StatusCode(), reachedHandler
}

func TestFirstVectoreeLinkIsOpen(t *testing.T) {
	if _, reached := runRelinkGuard(t, false, ""); !reached {
		t.Fatal("the first link must not need a session")
	}
}

func TestAnonymousRelinkIsRejected(t *testing.T) {
	status, reached := runRelinkGuard(t, true, "")
	if reached || status != http.StatusUnauthorized {
		t.Fatalf("status = %d, reached = %v", status, reached)
	}
}

func TestRelinkWithAnInvalidSessionIsRejected(t *testing.T) {
	previous := appuser.UserApplicationSVC.DomainSVC
	appuser.UserApplicationSVC.DomainSVC = noSessionDomain{}
	t.Cleanup(func() { appuser.UserApplicationSVC.DomainSVC = previous })

	status, reached := runRelinkGuard(t, true, "forged")
	if reached || status != http.StatusUnauthorized {
		t.Fatalf("status = %d, reached = %v", status, reached)
	}
}
