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

package coze

import (
	"context"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol"

	"github.com/coze-dev/coze-studio/backend/application/user"
	"github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/hertzutil/domain"
	"github.com/coze-dev/coze-studio/backend/pkg/i18n"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

// RegisterVectoreeAuth adds App Auth routes that the generated passport IDL does not include.
func RegisterVectoreeAuth(r *server.Hertz) {
	r.POST("/api/passport/web/email/verify/", func(ctx context.Context, c *app.RequestContext) {
		var body struct {
			Email string `json:"email"`
			OTP   string `json:"otp"`
		}
		if err := c.BindJSON(&body); err != nil {
			invalidParamRequestResponse(c, "invalid json")
			return
		}

		resp, sessionKey, err := user.UserApplicationSVC.PassportWebEmailVerify(ctx, string(i18n.GetLocale(ctx)), body.Email, body.OTP)
		if err != nil {
			internalServerErrorResponse(ctx, c, err)
			return
		}
		setSessionCookie(c, sessionKey)
		c.JSON(http.StatusOK, resp)
	})

	r.POST("/api/passport/web/email/send-verification/", func(ctx context.Context, c *app.RequestContext) {
		var body struct {
			Email string `json:"email"`
		}
		if err := c.BindJSON(&body); err != nil {
			invalidParamRequestResponse(c, "invalid json")
			return
		}
		if err := user.UserApplicationSVC.PassportWebEmailSendVerification(ctx, body.Email); err != nil {
			internalServerErrorResponse(ctx, c, err)
			return
		}
		c.JSON(http.StatusOK, map[string]any{"code": 0, "msg": ""})
	})
}

func setSessionCookie(c *app.RequestContext, sessionKey string) {
	c.SetCookie(entity.SessionKey,
		sessionKey,
		consts.SessionMaxAgeSecond,
		"/", domain.GetOriginHost(c),
		protocol.CookieSameSiteDefaultMode,
		false, true)
}
