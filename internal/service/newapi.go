// newapi.go New API(one-api 系)模型费率业务:配置存取(settings 表,
// 令牌脱敏展示、留空沿用)、费率表实时拉取(不落库)与单次调用成本估算。
// 上游请求统一经 llmclient 代理,错误按 3xxx 段归类(见任务 design.md)。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/model"
	"github.com/acooler15/modelmeter/internal/service/llmclient"
)

// newAPISettingKey New API 配置在 settings 表中的存储键。
const newAPISettingKey = "newapi"

// quotaPerUSD one-api 系约定的 quota 与美元换算比:500000 quota = 1 美元。
const quotaPerUSD = 500000

// newAPICodes New API 3xxx 段错误码组:复用 llmclient 的请求与归类模板,
// 文案明确指向「New API 服务」,凭据名称用「访问令牌」。
var newAPICodes = llmclient.ErrCodes{
	Network:     apperr.CodeNewAPINetwork,
	Auth:        apperr.CodeNewAPIAuth,
	BadResponse: apperr.CodeNewAPIBadResponse,
	ServiceName: "New API 服务",
	AuthHint:    "New API 访问令牌",
}

// NewAPIConfigInput 保存 New API 配置的输入;Token 留空表示沿用原值。
type NewAPIConfigInput struct {
	BaseURL string `json:"base_url"`
	Token   string `json:"token"`
}

// NewAPIConfigView New API 配置的对外视图:令牌只含脱敏值,不含明文。
type NewAPIConfigView struct {
	Configured  bool   `json:"configured"` // 是否已保存过配置
	BaseURL     string `json:"base_url"`
	TokenMasked string `json:"token_masked"`
}

// storedConfig settings 表中 newapi 键的 JSON 载荷;Token 为明文,只在
// service 内部流转,任何对外视图都不携带。
type storedConfig struct {
	BaseURL string `json:"base_url"`
	Token   string `json:"token"`
}

// RateEntry 上游 /api/pricing 的单个模型费率条目。
// 上游不同版本字段有多有少(可能多 cache_ratio/分组字段),解析时未知字段
// 直接忽略、缺失字段按零值降级,尽量把可用部分展示出来。
type RateEntry struct {
	ModelName       string  `json:"model_name"`
	QuotaType       int     `json:"quota_type"` // 0=倍率计费,1=按次计费
	ModelRatio      float64 `json:"model_ratio"`
	CompletionRatio float64 `json:"completion_ratio"`
	ModelPrice      float64 `json:"model_price"`
}

// CostEstimate 单次调用成本估算结果;Formula 为中文口径说明,前端直接展示。
type CostEstimate struct {
	Model     string  `json:"model"`
	QuotaType int     `json:"quota_type"`
	USD       float64 `json:"usd"`
	Quota     float64 `json:"quota"` // 倍率型才有意义,按次型为 0
	Formula   string  `json:"formula"`
}

// EstimateInput 成本估算请求输入;测试页在测试成功并拿到 usage 后调用。
type EstimateInput struct {
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
}

// readStoredConfig 读取 settings 表中的 New API 配置;不存在或内容损坏时
// 返回 found=false(损坏只记日志并按未配置降级,等待用户重新保存覆盖)。
func readStoredConfig(ctx context.Context, db *gorm.DB) (storedConfig, bool, error) {
	var s model.Setting
	err := db.WithContext(ctx).First(&s, "key = ?", newAPISettingKey).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return storedConfig{}, false, nil
	}
	if err != nil {
		return storedConfig{}, false, apperr.Wrap(apperr.CodeInternal, "查询 New API 配置失败", err)
	}
	var cfg storedConfig
	if err := json.Unmarshal([]byte(s.Value), &cfg); err != nil {
		slog.Warn("New API 配置内容损坏,按未配置处理", "err", err)
		return storedConfig{}, false, nil
	}
	return cfg, true, nil
}

// GetNewAPIConfig 返回 New API 配置视图;令牌只回脱敏值,任何情况不回明文。
func GetNewAPIConfig(ctx context.Context, db *gorm.DB) (NewAPIConfigView, error) {
	cfg, found, err := readStoredConfig(ctx, db)
	if err != nil {
		return NewAPIConfigView{}, err
	}
	if !found {
		return NewAPIConfigView{Configured: false}, nil
	}
	return NewAPIConfigView{
		Configured:  true,
		BaseURL:     cfg.BaseURL,
		TokenMasked: MaskKey(cfg.Token),
	}, nil
}

// SaveNewAPIConfig 保存 New API 配置:地址必须合法 http(s);首次保存令牌必填,
// 已有配置时令牌留空表示沿用原值(与接口配置编辑语义一致)。
func SaveNewAPIConfig(ctx context.Context, db *gorm.DB, in NewAPIConfigInput) (NewAPIConfigView, error) {
	base := strings.TrimSpace(in.BaseURL)
	if base == "" {
		return NewAPIConfigView{}, apperr.New(apperr.CodeNewAPINotConfigured, "New API 地址不能为空")
	}
	if !isValidHTTPURL(base) {
		return NewAPIConfigView{}, apperr.New(apperr.CodeNewAPINotConfigured, "New API 地址不是合法的 http(s) 地址")
	}
	old, found, err := readStoredConfig(ctx, db)
	if err != nil {
		return NewAPIConfigView{}, err
	}
	// 令牌留空(含纯空白)表示沿用原值;首次保存没有原值可沿用,必须填写
	token := strings.TrimSpace(in.Token)
	if token == "" {
		if !found || old.Token == "" {
			return NewAPIConfigView{}, apperr.New(apperr.CodeNewAPINotConfigured, "访问令牌不能为空")
		}
		token = old.Token
	}
	data, err := json.Marshal(storedConfig{BaseURL: base, Token: token})
	if err != nil {
		return NewAPIConfigView{}, apperr.Wrap(apperr.CodeInternal, "保存 New API 配置失败", err)
	}
	// settings 以 key 为主键整体覆盖;OnConflict 让首次插入与后续更新走同一语句
	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&model.Setting{Key: newAPISettingKey, Value: string(data)}).Error; err != nil {
		return NewAPIConfigView{}, apperr.Wrap(apperr.CodeInternal, "保存 New API 配置失败", err)
	}
	// 日志只记地址与操作结果,令牌不进任何日志
	slog.Info("New API 配置保存成功", "base_url", base, "token_updated", strings.TrimSpace(in.Token) != "")
	return NewAPIConfigView{Configured: true, BaseURL: base, TokenMasked: MaskKey(token)}, nil
}

// ListRates 实时拉取 New API 的模型费率表并解析,不落库。
// 配置缺失/不完整报 3401;上游错误按 3500/3501/3502 归类。
// 解析容忍降级:缺字段的条目按零值保留,model_name 为空或非对象条目跳过,
// 结果按模型名排序方便前端浏览。
func ListRates(ctx context.Context, db *gorm.DB) ([]RateEntry, error) {
	cfg, found, err := readStoredConfig(ctx, db)
	if err != nil {
		return nil, err
	}
	if !found || strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Token) == "" {
		return nil, apperr.New(apperr.CodeNewAPINotConfigured, "请先配置 New API 地址与令牌")
	}

	// data 用指针区分「字段缺失」与「空数组」:缺失视为结构异常,空数组是合法结果
	var resp struct {
		Data *[]json.RawMessage `json:"data"`
	}
	upstream := llmclient.UpstreamConfig{BaseURL: cfg.BaseURL, APIKey: cfg.Token}
	// 定价接口是公开路径,带 Bearer 头多余但无害,统一走同一客户端封装
	if err := llmclient.GetJSONWithCodes(ctx, upstream, "/api/pricing", &resp, newAPICodes); err != nil {
		return nil, err
	}
	if resp.Data == nil {
		return nil, apperr.New(apperr.CodeNewAPIBadResponse, "New API 返回的费率数据格式异常")
	}

	entries := make([]RateEntry, 0, len(*resp.Data))
	for _, item := range *resp.Data {
		var entry RateEntry
		// 非对象条目或 model_name 为空的条目跳过,容忍上游字段缺失与多余字段
		if err := json.Unmarshal(item, &entry); err != nil || entry.ModelName == "" {
			continue
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ModelName < entries[j].ModelName })
	return entries, nil
}

// EstimateCost 按费率表估算单次调用成本(one-api 口径):
//   - 倍率型(quota_type=0):quota = 输入倍率×prompt + 输入倍率×完成倍率×completion,
//     USD = quota / 500000;
//   - 按次型(quota_type=1):USD = model_price(model_price 为美元/次)。
//
// 未配置/拉取失败/未命中/用量为 0 一律返回 (CostEstimate{}, false) 并只记日志,
// 不向调用方外抛 —— 成本估算只是测试页的增强展示,失败不能干扰测试结果。
func EstimateCost(ctx context.Context, db *gorm.DB, modelName string, promptTokens, completionTokens int) (CostEstimate, bool) {
	if strings.TrimSpace(modelName) == "" || (promptTokens <= 0 && completionTokens <= 0) {
		return CostEstimate{}, false
	}
	entries, err := ListRates(ctx, db)
	if err != nil {
		// 3xxx 业务错误(未配置/网络/鉴权/响应异常)在此降噪为日志,不外抛
		slog.Warn("成本估算未执行:拉取 New API 费率失败", "model", modelName, "err", err)
		return CostEstimate{}, false
	}
	for _, e := range entries {
		if e.ModelName != modelName {
			continue
		}
		estimate, ok := buildEstimate(e, promptTokens, completionTokens)
		if !ok {
			return CostEstimate{}, false
		}
		return estimate, true
	}
	// 未命中费率表属正常降级,Debug 级别即可
	slog.Debug("成本估算未命中费率表", "model", modelName)
	return CostEstimate{}, false
}

// buildEstimate 按计费类型计算单次成本与中文口径说明;未知 quota_type 视为
// 不支持估算(返回 false),避免给出错误口径的结果。
func buildEstimate(e RateEntry, promptTokens, completionTokens int) (CostEstimate, bool) {
	switch e.QuotaType {
	case 0: // 倍率计费:完成倍率作用于完成部分
		quota := e.ModelRatio*float64(promptTokens) +
			e.ModelRatio*e.CompletionRatio*float64(completionTokens)
		return CostEstimate{
			Model:     e.ModelName,
			QuotaType: e.QuotaType,
			USD:       quota / quotaPerUSD,
			Quota:     quota,
			Formula:   "估算值:倍率计费,quota = 输入倍率×输入 tokens + 输入倍率×完成倍率×输出 tokens,500000 quota = 1 美元",
		}, true
	case 1: // 按次计费:model_price 为美元/次,与单次用量无关
		return CostEstimate{
			Model:     e.ModelName,
			QuotaType: e.QuotaType,
			USD:       e.ModelPrice,
			Quota:     0,
			Formula:   "估算值:按次计费,每次调用按固定价格(美元/次)计,与用量无关",
		}, true
	default:
		slog.Warn("成本估算不支持该计费类型", "model", e.ModelName, "quota_type", e.QuotaType)
		return CostEstimate{}, false
	}
}
