// zcode.go ZCode 模型配置适配:目标文件 ~/.zcode/v2/provider_config.json,
// 即 ZCode 实际使用的唯一模型配置。
//
// 读侧:供应商清单(providerOrder + providerRules)与模型清单(modelOrder ∪
// personalModelIds,保序去重)合并为模型条目,模型规则按 (providerId, modelId)
// 关联;无规则节点的模型照常列出,可编辑字段取缺省值。
//
// 写侧:仅白名单修改模型规则的 enabled / properties.contextWindow /
// optionSpecs.maxOutputTokens.max。整体流程为 json.Decoder+UseNumber 解析成
// map 树 → 只动命中节点 → 原子写回:凭据字段(access.apiKey)、templateId、
// manualProviderModelRules 及其他未知键原样保留,数字经 UseNumber 零精度丢失。
// map 键重编码后按字母序重排,JSON 语义零变化(沿用既有取舍)。供应商与
// API Key 一律引导用户到 ZCode 原生工具管理。
package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/service/agentconf"
)

// ZCodeAgent ZCode 的 Agent 接口实现;homeDir 构造注入,便于测试。
type ZCodeAgent struct {
	homeDir string
}

// NewZCodeAgent 构造 ZCode 适配器;homeDir 为用户主目录(对应 ~/.zcode)。
func NewZCodeAgent(homeDir string) *ZCodeAgent {
	return &ZCodeAgent{homeDir: homeDir}
}

// Name 实现 Agent 接口;同时用作 URL 标识与备份目录名。
func (z *ZCodeAgent) Name() string { return "zcode" }

// DisplayName 实现 Agent 接口;界面展示名。
func (z *ZCodeAgent) DisplayName() string { return "ZCode" }

// configPath 目标配置文件路径,读与写回的唯一目标。
func (z *ZCodeAgent) configPath() string {
	return filepath.Join(z.homeDir, ".zcode", "v2", "provider_config.json")
}

// zcodeManageHint 供应商与密钥的管理边界提示,卡片说明共用。
const zcodeManageHint = "供应商与 API Key 请在 ZCode 原生工具中管理,本页仅调整模型配置;修改后建议重启 ZCode 使配置生效"

// 白名单字段键:Fields 与 patch 中允许出现的键。
const (
	zcodeFieldEnabled         = "enabled"
	zcodeFieldContextWindow   = "context_window"
	zcodeFieldMaxOutputTokens = "max_output_tokens"
)

// Snapshot 实现 Agent 接口:配置文件存在即 found,列描述静态给出。
func (z *ZCodeAgent) Snapshot(_ context.Context) agentconf.Snapshot {
	path := z.configPath()
	if !agentconf.FileExists(path) {
		return agentconf.Snapshot{
			Name:        z.Name(),
			DisplayName: z.DisplayName(),
			Status:      agentconf.StatusNotFound,
			ConfigPath:  path,
			Columns:     zcodeColumns(),
			Message: "未找到 ZCode 配置文件(~/.zcode/v2/provider_config.json)," +
				"请确认 ZCode 已安装并至少运行过一次",
		}
	}
	return agentconf.Snapshot{
		Name:        z.Name(),
		DisplayName: z.DisplayName(),
		Status:      agentconf.StatusFound,
		ConfigPath:  path,
		Columns:     zcodeColumns(),
		Message:     zcodeManageHint,
	}
}

// zcodeColumns 模型表格列描述:可编辑列为启用开关与两个数字项,其余只读展示。
// provider_name/model_id 两列同时冗余在 Fields 中,便于前端按列键统一取值。
func zcodeColumns() []agentconf.FieldSpec {
	return []agentconf.FieldSpec{
		{Key: "provider_name", Label: "供应商", Type: "text", Readonly: true},
		{Key: "model_id", Label: "模型 ID", Type: "text", Readonly: true},
		{Key: zcodeFieldEnabled, Label: "启用", Type: "bool"},
		{Key: zcodeFieldContextWindow, Label: "上下文窗口", Type: "number"},
		{Key: zcodeFieldMaxOutputTokens, Label: "最大输出 Token", Type: "number"},
		{Key: "provider_enabled", Label: "供应商启用", Type: "bool", Readonly: true},
		{Key: "api_type", Label: "API 协议", Type: "text", Readonly: true},
		{Key: "base_url", Label: "接口地址", Type: "text", Readonly: true},
	}
}

// Models 实现 Agent 接口:返回模型清单(凭据脱敏)。
func (z *ZCodeAgent) Models(_ context.Context) ([]agentconf.ModelEntry, error) {
	doc, err := z.readTree()
	if err != nil {
		return nil, err
	}
	return buildZCodeEntries(doc), nil
}

// ApplyModels 实现 Agent 接口:先整体校验全部 patch,再一次备份、树编辑、
// 原子写回,返回重新读取的最新清单。白名单外的键报 4403,定位不存在的
// (供应商, 模型) 报 4404。日志只记改动键名,不记任何值。
func (z *ZCodeAgent) ApplyModels(ctx context.Context, patches []agentconf.ModelPatch, dataDir string) ([]agentconf.ModelEntry, error) {
	doc, err := z.readTree()
	if err != nil {
		return nil, err
	}
	if err := validateZCodePatches(doc, patches); err != nil {
		return nil, err
	}
	if len(patches) == 0 {
		// 空提交视为只读:不备份、不落盘,直接返回现状
		return buildZCodeEntries(doc), nil
	}
	// 备份先于写回:任何写回动作前必须先留一份可还原的副本
	if _, err := agentconf.Backup(z.configPath(), z.Name(), dataDir); err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "备份 ZCode 配置失败", err)
	}
	for _, p := range patches {
		applyZCodePatch(doc, p)
	}
	if err := agentconf.WriteJSONFile(z.configPath(), doc); err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "写回 ZCode 配置失败", err)
	}
	slog.Info("ZCode 模型配置已更新", "patches", len(patches), "fields", patchChangedKeys(patches))
	return z.Models(ctx)
}

// readTree 读取配置文件并用 json.Decoder+UseNumber 解析为 map 树。
// 文件缺失报 4404;读取失败或非法 JSON 报 4402。
func (z *ZCodeAgent) readTree() (map[string]any, error) {
	path := z.configPath()
	if !agentconf.FileExists(path) {
		return nil, apperr.New(apperr.CodeAgentNotFound,
			"未找到 ZCode 配置文件(~/.zcode/v2/provider_config.json),请确认 ZCode 已安装并至少运行过一次")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "读取 ZCode 配置失败", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "ZCode 配置文件不是合法 JSON,请先在 ZCode 原生工具中修复", err)
	}
	return doc, nil
}

// providerInfo 从 providerRules 提取的供应商非敏感信息(不携带凭据)。
type providerInfo struct {
	id      string
	name    string
	enabled bool
	apiType string
	baseURL string
	config  map[string]any // 供应商 config 节点,仅用于取 modelOrder/personalModelIds
}

// collectZCodeProviders 解析供应商清单:providerOrder 保序遍历,providerRules
// 建立映射(缺名回退 id);残缺条目(无 config/api)容忍为空值。
// providerOrder 未列出的规则条目按 id 排序补充在尾部,保证输出稳定。
func collectZCodeProviders(doc map[string]any) []providerInfo {
	cfg := mapGetObj(doc, "config")
	rules := map[string]map[string]any{}
	for _, e := range anySlice(mapGetObj(cfg, "providerConfigRules")["providerRules"]) {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if id := stringOf(m["providerId"]); id != "" {
			rules[id] = m
		}
	}
	order := make([]string, 0, len(rules))
	seen := make(map[string]bool, len(rules))
	for _, e := range anySlice(cfg["providerOrder"]) {
		if id := stringOf(e); id != "" && rules[id] != nil && !seen[id] {
			order = append(order, id)
			seen[id] = true
		}
	}
	extra := make([]string, 0, len(rules))
	for id := range rules {
		if !seen[id] {
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	order = append(order, extra...)

	providers := make([]providerInfo, 0, len(order))
	for _, id := range order {
		p := providerInfo{id: id, name: id, enabled: true}
		rule := rules[id]
		if s := stringOf(rule["providerName"]); s != "" {
			p.name = s
		}
		if b, ok := boolOf(rule["enabled"]); ok {
			p.enabled = b
		}
		p.config = mapGetObj(rule, "config")
		if api := mapGetObj(rule, "config", "api"); api != nil {
			p.apiType = stringOf(api["type"])
			p.baseURL = stringOf(api["baseUrl"])
		}
		providers = append(providers, p)
	}
	return providers
}

// zcodeModelIDs 供应商的模型清单:modelOrder ∪ personalModelIds,保序去重。
func zcodeModelIDs(p providerInfo) []string {
	ids := make([]string, 0, 8)
	seen := make(map[string]bool, 8)
	for _, key := range []string{"modelOrder", "personalModelIds"} {
		for _, e := range anySlice(p.config[key]) {
			if id := stringOf(e); id != "" && !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
	}
	return ids
}

// zcodeRuleKey 模型规则的定位键:(providerId, modelId)。
type zcodeRuleKey struct{ providerID, modelID string }

// zcodeRulesSlice 取 config.modelConfigRules.providerModelRules 数组;缺失返回 nil。
func zcodeRulesSlice(doc map[string]any) []any {
	v, ok := mapGet(doc, "config", "modelConfigRules", "providerModelRules")
	if !ok {
		return nil
	}
	return anySlice(v)
}

// zcodeRulesIndex 按 (providerId, modelId) 建立规则节点映射;同键重复时取首个。
func zcodeRulesIndex(doc map[string]any) map[zcodeRuleKey]map[string]any {
	idx := make(map[zcodeRuleKey]map[string]any)
	for _, e := range zcodeRulesSlice(doc) {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		k := zcodeRuleKey{stringOf(m["providerId"]), stringOf(m["modelId"])}
		if _, dup := idx[k]; !dup {
			idx[k] = m
		}
	}
	return idx
}

// buildZCodeEntries 由解析树构造模型清单(脱敏)。
func buildZCodeEntries(doc map[string]any) []agentconf.ModelEntry {
	providers := collectZCodeProviders(doc)
	rules := zcodeRulesIndex(doc)
	entries := make([]agentconf.ModelEntry, 0, len(providers))
	for _, p := range providers {
		for _, modelID := range zcodeModelIDs(p) {
			entries = append(entries, agentconf.ModelEntry{
				ProviderID:   p.id,
				ProviderName: p.name,
				ModelID:      modelID,
				DisplayName:  modelID,
				Fields:       zcodeEntryFields(p, modelID, rules[zcodeRuleKey{p.id, modelID}]),
			})
		}
	}
	return entries
}

// zcodeEntryFields 汇总一个条目的非凭据字段。规则节点缺失时 enabled 取缺省
// true,数字字段缺省不出现;数字以 json.Number 原样透出。
func zcodeEntryFields(p providerInfo, modelID string, rule map[string]any) map[string]any {
	fields := map[string]any{
		"provider_name":    p.name,
		"model_id":         modelID,
		"provider_enabled": p.enabled,
		"api_type":         p.apiType,
		"base_url":         p.baseURL,
		"enabled":          true, // 规则节点缺省视为启用
	}
	if rule != nil {
		if b, ok := boolOf(mapGetObj(rule, "config")["enabled"]); ok {
			fields[zcodeFieldEnabled] = b
		}
		if v, ok := mapGet(rule, "config", "properties", "contextWindow"); ok {
			fields[zcodeFieldContextWindow] = v
		}
		if v, ok := mapGet(rule, "config", "optionSpecs", "maxOutputTokens", "max"); ok {
			fields[zcodeFieldMaxOutputTokens] = v
		}
	}
	return fields
}

// validateZCodePatches 一次性校验全部 patch:定位必须存在于供应商模型清单,
// 字段键必须在白名单内且值类型正确。先整体校验再落盘,避免半批生效。
func validateZCodePatches(doc map[string]any, patches []agentconf.ModelPatch) error {
	valid := make(map[zcodeRuleKey]bool)
	for _, p := range collectZCodeProviders(doc) {
		for _, id := range zcodeModelIDs(p) {
			valid[zcodeRuleKey{p.id, id}] = true
		}
	}
	for _, p := range patches {
		if !valid[zcodeRuleKey{p.ProviderID, p.ModelID}] {
			return apperr.New(apperr.CodeAgentNotFound, "模型不存在:"+p.ProviderID+" / "+p.ModelID)
		}
		for k, v := range p.Fields {
			switch k {
			case zcodeFieldEnabled:
				if _, ok := v.(bool); !ok {
					return apperr.New(apperr.CodeAgentInvalid, "字段 enabled 必须为布尔值")
				}
			case zcodeFieldContextWindow, zcodeFieldMaxOutputTokens:
				if _, ok := numberValue(v); !ok {
					return apperr.New(apperr.CodeAgentInvalid, "字段 "+k+" 必须为数字")
				}
			default:
				return apperr.New(apperr.CodeAgentInvalid, "不支持修改字段 "+k+",仅允许 enabled、context_window、max_output_tokens")
			}
		}
	}
	return nil
}

// applyZCodePatch 在树上定位 (providerId, modelId) 规则节点并应用白名单修改;
// 规则节点或中间节点缺失时创建最小节点,树中其余节点原样保留。
func applyZCodePatch(doc map[string]any, p agentconf.ModelPatch) {
	rules := ensureZCodeRulesSlice(doc)
	var rule map[string]any
	for _, e := range rules {
		m, ok := e.(map[string]any)
		if ok && stringOf(m["providerId"]) == p.ProviderID && stringOf(m["modelId"]) == p.ModelID {
			rule = m
			break
		}
	}
	if rule == nil {
		// 无规则节点:追加最小节点,config 内只写入 patch 涉及的键
		rule = map[string]any{"providerId": p.ProviderID, "modelId": p.ModelID}
		rules = append(rules, rule)
		setZCodeRulesSlice(doc, rules)
	}
	cfg := ensureMap(rule, "config")
	for k, v := range p.Fields {
		switch k {
		case zcodeFieldEnabled:
			cfg[zcodeFieldEnabled] = v
		case zcodeFieldContextWindow:
			n, _ := numberValue(v)
			ensureMap(cfg, "properties")["contextWindow"] = n
		case zcodeFieldMaxOutputTokens:
			n, _ := numberValue(v)
			ensureMap(ensureMap(cfg, "optionSpecs"), "maxOutputTokens")["max"] = n
		}
	}
}

// ensureZCodeRulesSlice 取 config.modelConfigRules.providerModelRules 数组,
// 中间节点或数组缺失时创建,返回可追加的切片。
func ensureZCodeRulesSlice(doc map[string]any) []any {
	mcr := ensureMap(ensureMap(doc, "config"), "modelConfigRules")
	arr, ok := mcr["providerModelRules"].([]any)
	if !ok {
		arr = []any{}
		mcr["providerModelRules"] = arr
	}
	return arr
}

// setZCodeRulesSlice 回写规则数组;追加节点可能触发切片扩容,需重新落树。
func setZCodeRulesSlice(doc map[string]any, rules []any) {
	ensureMap(ensureMap(doc, "config"), "modelConfigRules")["providerModelRules"] = rules
}
