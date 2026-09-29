// zcode.go ZCode 模型配置适配:目标文件 ~/.zcode/v2/provider_config.json,
// 即 ZCode 实际使用的唯一模型配置。
//
// 读侧:供应商清单(providerOrder + providerRules)与模型清单(modelOrder ∪
// personalModelIds,保序去重)合并为模型条目,模型规则按 (providerId, modelId)
// 关联;无规则节点的模型照常列出,可编辑字段取缺省值。schemaVersion 必须为
// 数字 1(真实 schema 的 z.literal(1)),否则拒绝读写;defaultModelSelection
// 作为"默认模型"在 Snapshot 中宽容解析(缺失/残缺一律视为未设置)。
//
// 写侧:仅白名单修改模型规则的 enabled / properties.contextWindow /
// properties.inputFormat.supportsImage / properties.supportsJsonSchemaOutput /
// optionSpecs.maxOutputTokens.max / optionSpecs.reasoningLevel.values。整体流程为
// json.Decoder+UseNumber 解析成 map 树 → 只动命中节点 → 原子写回:凭据字段
// (access.apiKey)、templateId、manualProviderModelRules 及其他未知键原样保留,
// 数字经 UseNumber 零精度丢失。追加规则节点前先查 manualProviderModelRules,
// 同 (providerId, modelId) 已存在时改编辑该节点(zod 不允许两数组重复落键)。
// map 键重编码后按字母序重排,JSON 语义零变化(沿用既有取舍)。供应商与
// API Key 一律引导用户到 ZCode 原生工具管理。
//
// 默认模型写回(ApplyDefaultModel):provider_id+model_id 均空=清除、均非空=
// 设置,写回前校验模型存在(4404)与推理档位取值(4403,规则未定义候选值则
// 放行)。defaultModelSelection 节点整体替换为规范形状,是"未知键零丢失"红线
// 的唯一显式例外——根对象与 config 均 .strict() 校验,节点内未知键只会是坏
// 文件,保留会让 ZCode 拒载整份配置。
package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	zcodeFieldEnabled            = "enabled"
	zcodeFieldContextWindow      = "context_window"
	zcodeFieldMaxOutputTokens    = "max_output_tokens"
	zcodeFieldSupportsImage      = "supports_image"
	zcodeFieldSupportsJSONSchema = "supports_json_schema_output"
	zcodeFieldReasoningLevels    = "reasoning_levels"
)

// Snapshot 实现 Agent 接口:配置文件存在即 found,列描述的 reasoning_levels
// 选项取自文件;ZCode 有"默认模型"概念,能力位恒置 true(能力是工具属性,
// 文件缺失时同样成立),默认模型现状从 config.defaultModelSelection 宽容解析。
func (z *ZCodeAgent) Snapshot(_ context.Context) agentconf.Snapshot {
	path := z.configPath()
	if !agentconf.FileExists(path) {
		return agentconf.Snapshot{
			Name:                 z.Name(),
			DisplayName:          z.DisplayName(),
			Status:               agentconf.StatusNotFound,
			ConfigPath:           path,
			Columns:              zcodeColumns(nil),
			SupportsDefaultModel: true,
			Message: "未找到 ZCode 配置文件(~/.zcode/v2/provider_config.json)," +
				"请确认 ZCode 已安装并至少运行过一次",
		}
	}
	return agentconf.Snapshot{
		Name:                 z.Name(),
		DisplayName:          z.DisplayName(),
		Status:               agentconf.StatusFound,
		ConfigPath:           path,
		Columns:              zcodeColumns(z.collectReasoningLevels()),
		SupportsDefaultModel: true,
		DefaultModel:         z.readDefaultModel(),
		Message:              zcodeManageHint,
	}
}

// zcodeColumns 模型表格列描述:可编辑列为启用开关、两个数字项与三个能力键,
// 其余只读展示。levels 为全部规则 reasoningLevel.values 的保序并集,作
// reasoning_levels 列的候选项(空则由前端自由输入)。provider_name/model_id
// 两列同时冗余在 Fields 中,便于前端按列键统一取值。
func zcodeColumns(levels []string) []agentconf.FieldSpec {
	levelsField := agentconf.FieldSpec{Key: zcodeFieldReasoningLevels, Label: "推理档位", Type: "list"}
	if len(levels) > 0 {
		levelsField.Options = make([]agentconf.FieldOption, 0, len(levels))
		for _, lv := range levels {
			levelsField.Options = append(levelsField.Options, agentconf.FieldOption{Value: lv, Label: lv})
		}
	}
	return []agentconf.FieldSpec{
		{Key: "provider_name", Label: "供应商", Type: "text", Readonly: true},
		{Key: "model_id", Label: "模型 ID", Type: "text", Readonly: true},
		{Key: zcodeFieldEnabled, Label: "启用", Type: "bool"},
		{Key: zcodeFieldContextWindow, Label: "上下文窗口", Type: "number"},
		{Key: zcodeFieldMaxOutputTokens, Label: "最大输出 Token", Type: "number"},
		{Key: zcodeFieldSupportsImage, Label: "图像输入", Type: "bool"},
		{Key: zcodeFieldSupportsJSONSchema, Label: "JSON Schema 输出", Type: "bool"},
		levelsField,
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

// 默认模型写回动作(action 取值)。
const (
	defaultModelActionNone  = "none"  // 无实际变化:不备份不落盘
	defaultModelActionSet   = "set"   // 设置默认模型
	defaultModelActionClear = "clear" // 清除默认模型
)

// ApplyDefaultModel 实现 DefaultModelSetter 接口:读取-校验-备份-树编辑-原子写,
// 返回写回后的最新 Snapshot。provider_id+model_id 均空=清除、均非空=设置、
// 混合报 4403;设置时 (providerId, modelId) 必须在模型清单内(否则 4404),
// reasoning_level 在目标规则定义了候选 values 时必须取值于其中(否则 4403,
// 未定义 values 则放行——schema 层面任意字符串合法)。
func (z *ZCodeAgent) ApplyDefaultModel(ctx context.Context, patch agentconf.DefaultModelPatch, dataDir string) (agentconf.Snapshot, error) {
	doc, err := z.readTree()
	if err != nil {
		return agentconf.Snapshot{}, err
	}
	cfg := mapGetObj(doc, "config")
	_, hasCurrent := cfg["defaultModelSelection"]
	action, sel := defaultModelActionNone, map[string]any(nil)

	switch setting, clearing := patch.ProviderID != "" && patch.ModelID != "", patch.ProviderID == "" && patch.ModelID == ""; {
	case clearing:
		if !hasCurrent {
			// 本无该字段:不备份不落盘,直接返回现状
			return z.Snapshot(ctx), nil
		}
		action = defaultModelActionClear
	case setting:
		if err := validateZCodeDefaultModelTarget(doc, patch); err != nil {
			return agentconf.Snapshot{}, err
		}
		if dm := defaultModelOf(doc); dm != nil &&
			dm.ProviderID == patch.ProviderID && dm.ModelID == patch.ModelID && dm.ReasoningLevel == patch.ReasoningLevel {
			// 与现状完全一致:无实际变化,零落盘(不产生第二次备份)
			return z.Snapshot(ctx), nil
		}
		action = defaultModelActionSet
		sel = map[string]any{"providerId": patch.ProviderID, "modelId": patch.ModelID}
		if patch.ReasoningLevel != "" {
			sel["options"] = map[string]any{"reasoningLevel": patch.ReasoningLevel}
		}
	default:
		return agentconf.Snapshot{}, apperr.New(apperr.CodeAgentInvalid,
			"provider_id 与 model_id 必须同时提供(设置)或同时留空(清除)")
	}

	// 备份先于写回:任何写回动作前必须先留一份可还原的副本
	if _, err := agentconf.Backup(z.configPath(), z.Name(), dataDir); err != nil {
		return agentconf.Snapshot{}, apperr.Wrap(apperr.CodeAgentFileIO, "备份 ZCode 配置失败", err)
	}
	if cfg == nil {
		cfg = ensureMap(doc, "config")
	}
	if action == defaultModelActionClear {
		delete(cfg, "defaultModelSelection")
	} else {
		// 红线显式例外:defaultModelSelection 节点(含 options)整体替换为规范
		// 形状,不保留节点内未知键——根对象与 config 均 .strict() 校验,节点内
		// 未知键只会是坏文件,保留反而会让 ZCode 拒载整份配置(research.md 第
		// 6 节);"未知键零丢失"红线在其余节点上仍严格成立。
		cfg["defaultModelSelection"] = sel
	}
	if err := agentconf.WriteJSONFile(z.configPath(), doc); err != nil {
		return agentconf.Snapshot{}, apperr.Wrap(apperr.CodeAgentFileIO, "写回 ZCode 配置失败", err)
	}
	// 日志只记定位键与动作,不记任何配置值
	slog.Info("ZCode 默认模型已更新", "action", action, "provider_id", patch.ProviderID, "model_id", patch.ModelID)
	return z.Snapshot(ctx), nil
}

// validateZCodeDefaultModelTarget 校验设置动作的定位与推理档位:模型必须存在
// (4404);目标规则(providerModelRules 优先,manual 冲突守护同款口径)定义了
// reasoningLevel.values 且不含该档位时报 4403,未定义 values 则放行。
func validateZCodeDefaultModelTarget(doc map[string]any, patch agentconf.DefaultModelPatch) error {
	found := false
	for _, p := range collectZCodeProviders(doc) {
		if p.id != patch.ProviderID {
			continue
		}
		for _, id := range zcodeModelIDs(p) {
			if id == patch.ModelID {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return apperr.New(apperr.CodeAgentNotFound, "模型不存在:"+patch.ProviderID+" / "+patch.ModelID)
	}
	if patch.ReasoningLevel == "" {
		return nil
	}
	if rule := locateZCodeModelRule(doc, patch.ProviderID, patch.ModelID); rule != nil {
		values := stringSliceOf(mapGetObj(rule, "config", "optionSpecs", "reasoningLevel")["values"])
		if len(values) > 0 {
			for _, v := range values {
				if v == patch.ReasoningLevel {
					return nil
				}
			}
			return apperr.New(apperr.CodeAgentInvalid,
				"推理档位不在该模型的候选档位内:"+strings.Join(values, ", "))
		}
	}
	return nil
}

// readTree 读取配置文件并用 json.Decoder+UseNumber 解析为 map 树。
// 文件缺失报 4404;读取失败或非法 JSON 报 4402;schemaVersion 存在且不是
// 数字 1 时报 4402(真实 schema 为 z.literal(1),未知版本的文件不能当作
// ZCode 配置编辑;缺失则容忍,残缺文件仍可安全编辑)。
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
	if v, ok := doc["schemaVersion"]; ok {
		if n, isNum := v.(json.Number); !isNum || n.String() != "1" {
			return nil, apperr.New(apperr.CodeAgentFileIO,
				"ZCode 配置 schemaVersion 不受支持(仅支持 1),请升级 ModelMeter 或在 ZCode 中重新生成配置")
		}
	}
	return doc, nil
}

// readDefaultModel 宽容解析 config.defaultModelSelection;文件读取失败
// (缺失/schemaVersion 不支持/非法 JSON)时返回 nil,维持 Snapshot 不返回
// 错误的约定。
func (z *ZCodeAgent) readDefaultModel() *agentconf.DefaultModel {
	doc, err := z.readTree()
	if err != nil {
		return nil
	}
	return defaultModelOf(doc)
}

// defaultModelOf 从解析树提取默认模型现状;节点缺失、字段残缺(providerId 或
// modelId 缺失/非字符串)时返回 nil 不报错——未设置过默认模型的文件本就
// 没有该字段,残缺节点按未设置处理。
func defaultModelOf(doc map[string]any) *agentconf.DefaultModel {
	sel := mapGetObj(doc, "config", "defaultModelSelection")
	if sel == nil {
		return nil
	}
	providerID := stringOf(sel["providerId"])
	modelID := stringOf(sel["modelId"])
	if providerID == "" || modelID == "" {
		return nil
	}
	dm := &agentconf.DefaultModel{ProviderID: providerID, ModelID: modelID}
	if level := stringOf(mapGetObj(sel, "options")["reasoningLevel"]); level != "" {
		dm.ReasoningLevel = level
	}
	return dm
}

// collectReasoningLevels 汇总全部模型规则 optionSpecs.reasoningLevel.values
// 的保序去重并集,作为 reasoning_levels 列的候选项;文件缺失、解析失败或
// schemaVersion 不支持时返回空,由调用方降级为自由输入(收集方式对齐
// WorkBuddy 的 collectEfforts)。
func (z *ZCodeAgent) collectReasoningLevels() []string {
	doc, err := z.readTree()
	if err != nil {
		return nil
	}
	seen := make(map[string]bool, 8)
	levels := make([]string, 0, 8)
	for _, e := range zcodeRulesSlice(doc) {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		for _, lv := range stringSliceOf(mapGetObj(m, "config", "optionSpecs", "reasoningLevel")["values"]) {
			if !seen[lv] {
				seen[lv] = true
				levels = append(levels, lv)
			}
		}
	}
	return levels
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
// true,数字与能力键缺省不出现(文件里没有覆盖即保持 ZCode 缺省);数字以
// json.Number 原样透出,reasoning_levels 保序透出。
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
		if b, ok := boolOf(mapGetObj(rule, "config", "properties", "inputFormat")["supportsImage"]); ok {
			fields[zcodeFieldSupportsImage] = b
		}
		if b, ok := boolOf(mapGetObj(rule, "config", "properties")["supportsJsonSchemaOutput"]); ok {
			fields[zcodeFieldSupportsJSONSchema] = b
		}
		if v, ok := mapGet(rule, "config", "optionSpecs", "maxOutputTokens", "max"); ok {
			fields[zcodeFieldMaxOutputTokens] = v
		}
		if values := stringSliceOf(mapGetObj(rule, "config", "optionSpecs", "reasoningLevel")["values"]); len(values) > 0 {
			fields[zcodeFieldReasoningLevels] = values
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
			case zcodeFieldSupportsImage, zcodeFieldSupportsJSONSchema:
				if _, ok := v.(bool); !ok {
					return apperr.New(apperr.CodeAgentInvalid, "字段 "+k+" 必须为布尔值")
				}
			case zcodeFieldContextWindow, zcodeFieldMaxOutputTokens:
				if _, ok := numberValue(v); !ok {
					return apperr.New(apperr.CodeAgentInvalid, "字段 "+k+" 必须为数字")
				}
			case zcodeFieldReasoningLevels:
				if !isNonEmptyStringSlice(v) {
					return apperr.New(apperr.CodeAgentInvalid, "字段 reasoning_levels 必须为非空字符串数组")
				}
			default:
				return apperr.New(apperr.CodeAgentInvalid,
					"不支持修改字段 "+k+",仅允许 enabled、context_window、max_output_tokens、"+
						zcodeFieldSupportsImage+"、"+zcodeFieldSupportsJSONSchema+"、"+zcodeFieldReasoningLevels)
			}
		}
	}
	return nil
}

// locateZCodeManualRule 在 manualProviderModelRules 中按 (providerId, modelId)
// 查找规则节点;数组缺失或未命中返回 nil。manual 数组本身不展示、不主动编辑,
// 仅用于写侧冲突守护定位。
func locateZCodeManualRule(doc map[string]any, providerID, modelID string) map[string]any {
	for _, e := range anySlice(mapGetObj(doc, "config", "modelConfigRules")["manualProviderModelRules"]) {
		m, ok := e.(map[string]any)
		if ok && stringOf(m["providerId"]) == providerID && stringOf(m["modelId"]) == modelID {
			return m
		}
	}
	return nil
}

// locateZCodeModelRule 按 ApplyModels 的落点口径定位 (providerId, modelId)
// 规则节点:providerModelRules 优先,其次 manualProviderModelRules(冲突守护
// 同款口径);都不存在返回 nil(写回时将新建节点,无 values 约束)。
func locateZCodeModelRule(doc map[string]any, providerID, modelID string) map[string]any {
	for _, e := range zcodeRulesSlice(doc) {
		m, ok := e.(map[string]any)
		if ok && stringOf(m["providerId"]) == providerID && stringOf(m["modelId"]) == modelID {
			return m
		}
	}
	return locateZCodeManualRule(doc, providerID, modelID)
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
		// manual 冲突守护:同 (providerId, modelId) 已在 manualProviderModelRules
		// 时必须改编辑该节点——zod superRefine 不允许两个数组重复落同一
		// (providerId, modelId),追加 providerModelRules 会写出 ZCode 拒载的
		// 整份配置;manual 节点的 config 能力集与 providerModelRules 相同,
		// 可安全落白名单键。
		if manual := locateZCodeManualRule(doc, p.ProviderID, p.ModelID); manual != nil {
			rule = manual
		}
	}
	if rule == nil {
		// 无任何规则节点:追加最小节点,config 内只写入 patch 涉及的键
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
		case zcodeFieldSupportsImage:
			ensureMap(ensureMap(cfg, "properties"), "inputFormat")["supportsImage"] = v
		case zcodeFieldSupportsJSONSchema:
			ensureMap(cfg, "properties")["supportsJsonSchemaOutput"] = v
		case zcodeFieldMaxOutputTokens:
			n, _ := numberValue(v)
			ensureMap(ensureMap(cfg, "optionSpecs"), "maxOutputTokens")["max"] = n
		case zcodeFieldReasoningLevels:
			ensureMap(ensureMap(cfg, "optionSpecs"), "reasoningLevel")["values"] = stringSliceValue(v)
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
