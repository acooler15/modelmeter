// modelcatalog.go 模型列表业务:经 llmclient 代理拉取上游 /v1/models,
// 实时透传给前端、不落库。
package service

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/model"
	"github.com/acooler15/modelmeter/internal/service/llmclient"
)

// ModelInfo 上游 /v1/models 的单个模型条目。
type ModelInfo struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	OwnedBy string          `json:"owned_by"`
	Created int64           `json:"created"`
	Raw     json.RawMessage `json:"raw"` // 上游原始条目透传,前端可展示完整元信息
}

// ListModels 用指定接口配置的 Base URL + API Key 拉取上游模型列表。
// 解析容忍降级:条目仅要求有 id,owned_by/created 等字段缺失时按零值展示;
// 空 id 或非对象条目直接跳过。配置不存在报 1404,上游错误归 1500/1501/1502。
func ListModels(ctx context.Context, db *gorm.DB, providerID uint) ([]ModelInfo, error) {
	var p model.Provider
	if err := db.WithContext(ctx).First(&p, providerID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.New(apperr.CodeProviderNotFound, "接口配置不存在")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "查询接口配置失败", err)
	}

	// data 用指针区分「字段缺失」与「空数组」:缺失视为结构异常,空数组是合法结果
	var resp struct {
		Data *[]json.RawMessage `json:"data"`
	}
	cfg := llmclient.UpstreamConfig{BaseURL: p.BaseURL, APIKey: p.APIKey}
	if err := llmclient.GetJSON(ctx, cfg, "/v1/models", &resp); err != nil {
		return nil, err
	}
	if resp.Data == nil {
		return nil, apperr.New(apperr.CodeUpstreamBadResponse, "上游返回的模型列表格式异常")
	}

	models := make([]ModelInfo, 0, len(*resp.Data))
	for _, item := range *resp.Data {
		var entry struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
			Created int64  `json:"created"`
		}
		// 非对象条目或缺少 id 的条目都跳过,尽量把可用部分展示出来
		if err := json.Unmarshal(item, &entry); err != nil || entry.ID == "" {
			continue
		}
		models = append(models, ModelInfo{
			ID:      entry.ID,
			Object:  entry.Object,
			OwnedBy: entry.OwnedBy,
			Created: entry.Created,
			Raw:     item,
		})
	}
	return models, nil
}
