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
	"fmt"
	"strings"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	"github.com/coze-dev/coze-studio/backend/api/model/app/developer_api"
	bizconfig "github.com/coze-dev/coze-studio/backend/bizpkg/config"
	"github.com/coze-dev/coze-studio/backend/bizpkg/config/modelmgr"
)

const vectoreeModelID = "vectoree/auto"

func seedVectoreeModel(ctx context.Context, apiURL, apiKey string) error {
	mc := bizconfig.ModelConf()
	if mc == nil {
		return fmt.Errorf("model config is not initialized")
	}
	return seedWithModelConfig(ctx, mc, apiURL, apiKey)
}

func seedWithModelConfig(ctx context.Context, mc *modelmgr.ModelConfig, apiURL, apiKey string) error {
	baseURL := strings.TrimRight(apiURL, "/") + "/api/v1"
	conn := &adminconfig.Connection{
		BaseConnInfo: &adminconfig.BaseConnectionInfo{
			BaseURL: baseURL,
			APIKey:  apiKey,
			Model:   vectoreeModelID,
		},
	}

	useOld, err := mc.UseOldModelConf(ctx)
	if err != nil {
		return err
	}
	if !useOld {
		list, err := mc.GetOnlineModelList(ctx)
		if err != nil {
			return err
		}
		updated := false
		for _, item := range list {
			if !isVectoreeModel(item, baseURL) {
				continue
			}
			if err := mc.UpdateModelConnection(ctx, item.ID, conn); err != nil {
				return err
			}
			updated = true
		}
		if updated {
			return nil
		}
	}

	_, err = mc.CreateModel(ctx, developer_api.ModelClass_GPT, "Vectoree", conn, nil)
	return err
}

func isVectoreeModel(item *modelmgr.Model, baseURL string) bool {
	if item == nil || item.Model == nil || item.Connection == nil || item.Connection.BaseConnInfo == nil {
		return false
	}
	info := item.Connection.BaseConnInfo
	if strings.TrimSpace(info.Model) == vectoreeModelID {
		return true
	}
	name := ""
	if item.DisplayInfo != nil {
		name = strings.ToLower(item.DisplayInfo.Name)
	}
	sameBase := strings.TrimRight(info.BaseURL, "/") == strings.TrimRight(baseURL, "/")
	return sameBase && strings.Contains(name, "vectoree")
}
