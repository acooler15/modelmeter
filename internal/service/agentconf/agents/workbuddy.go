// workbuddy.go WorkBuddy 模型配置适配:目标文件 ~/.workbuddy/models.json。
// 真实 schema(勘探见任务 09-30-workbuddy-schema-realign 的 research.md):
// 顶层是裸数组,或兼容对象形态 {"models": [...], "availableModels": [...]}
// (存储层 DesktopModelsRepo 明示两种形态皆合法);每个元素一个模型,各自
// 携带独立的明文 apiKey(新版可整段替换为 WBEF1 密文,本适配器零接触即
// 天然兼容两种形态)。
//
// 读侧:两种顶层形态统一取 models 数组遍历为模型清单,字段脱敏映射(id/
// name/vendor/url/能力开关/数字项/reasoning);apiKey 绝不读取进输出结构。
// 缺省语义对齐 WorkBuddy 面板:disabled/onlyReasoning/useCustomProtocol
// 缺省 false,reasoning.canDisableThinking 缺省 true(仅 false 才落盘),
// defaultEffort 兜底 legacy 键 effort,数字项文件未写时该键不出现。
//
// 写侧:白名单修改 name、url、disabled、supportsToolCall、supportsImages、
// supportsReasoning、onlyReasoning、useCustomProtocol、maxInputTokens、
// maxOutputTokens、temperature、reasoning.defaultEffort、reasoning.supportedEfforts、
// reasoning.canDisableThinking;三个数字键接受 null 表示显式清除(移除该键,
// 恢复"文件未写"的工具缺省态);按 modelId 首个匹配定位数组元素,reasoning
// 节点缺失时创建;id、vendor、apiKey、tags 及未知键逐条原样保留。顶层形态
// 写回保持不变——对象形态只原地回填 models 数组,availableModels 等顶层键
// 零丢失(WorkBuddy 自己会把对象重写回裸数组,本适配器更保守)。
//
// 模型删除(RemoveModels)与添加(AddModels):删除按定位过滤数组元素(非
// 对象元素原样保留),对象形态同步移除顶层 availableModels 中命中的 id;添加
// 按来源接口追加条目(恰含 id/name/vendor/url/apiKey 五键,url 为完整 endpoint,
// 由 handler 从 Base URL 派生),apiKey 来自 ModelMeter 数据库装配的 Source,
// 属"新增写入凭据"的显式例外(经用户界面确认),既有条目的凭据仍零接触;
// 对象形态下 availableModels 为数组且不含该 id 时追加,缺失不创建。日志只记
// 数量与定位键,绝不记 url/key 值。
//
// 整体重编码时 map 键按字母序重排,JSON 语义零变化(沿用既有取舍)。
// WorkBuddy 无"默认模型"概念:能力位恒 false,不实现
// DefaultModelSetter(handler 对其默认模型请求报 4405)。
package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/service/agentconf"
)

// WorkBuddyAgent WorkBuddy 的 Agent 接口实现;homeDir 构造注入,便于测试。
type WorkBuddyAgent struct {
	homeDir string
}

// NewWorkBuddyAgent 构造 WorkBuddy 适配器。
func NewWorkBuddyAgent(homeDir string) *WorkBuddyAgent {
	return &WorkBuddyAgent{homeDir: homeDir}
}

// Name 实现 Agent 接口;同时用作 URL 标识与备份目录名。
func (w *WorkBuddyAgent) Name() string { return "workbuddy" }

// DisplayName 实现 Agent 接口;界面展示名。
func (w *WorkBuddyAgent) DisplayName() string { return "WorkBuddy" }

// modelsPath 目标配置文件路径,读与写回的唯一目标。
func (w *WorkBuddyAgent) modelsPath() string {
	return filepath.Join(w.homeDir, ".workbuddy", "models.json")
}

// wbManageHint 管理边界提示,卡片说明共用。
const wbManageHint = "既有条目的 API Key 请在 WorkBuddy 原生工具中管理,本页支持模型清单的增删与配置调整;修改后建议重启 WorkBuddy 使配置生效"

// 白名单字段键:Fields 与 patch 中允许出现的键。
const (
	wbFieldName               = "name"
	wbFieldURL                = "url"
	wbFieldDisabled           = "disabled"
	wbFieldSupportsToolCall   = "supports_tool_call"
	wbFieldSupportsImages     = "supports_images"
	wbFieldSupportsReasoning  = "supports_reasoning"
	wbFieldOnlyReasoning      = "only_reasoning"
	wbFieldUseCustomProtocol  = "use_custom_protocol"
	wbFieldDefaultEffort      = "default_effort"
	wbFieldSupportedEfforts   = "supported_efforts"
	wbFieldCanDisableThinking = "can_disable_thinking"
	wbFieldMaxInputTokens     = "max_input_tokens"
	wbFieldMaxOutputTokens    = "max_output_tokens"
	wbFieldTemperature        = "temperature"
)

// 文件侧键名:白名单字段到 models.json 元素键的映射(其余布尔/数字键与
// 文件键同名,apply 分支内直接写字面量)。
const (
	wbFileReasoning = "reasoning"
	wbFileVendor    = "vendor"
	wbFileModels    = "models" // 对象形态承载模型数组的顶层键
)

// Snapshot 实现 Agent 接口:配置文件存在即 found;default_effort 下拉选项为
// 标准五档合并文件内自定义档位(effortOptions)。增删模型能力位恒置 true
// (能力是工具属性,文件缺失时同样成立);WorkBuddy 无"默认模型"概念,
// 该能力位恒 false(零值)。
func (w *WorkBuddyAgent) Snapshot(_ context.Context) agentconf.Snapshot {
	path := w.modelsPath()
	if !agentconf.FileExists(path) {
		return agentconf.Snapshot{
			Name:                 w.Name(),
			DisplayName:          w.DisplayName(),
			Status:               agentconf.StatusNotFound,
			ConfigPath:           path,
			Columns:              workBuddyColumns(nil),
			SupportsAddModels:    true,
			SupportsRemoveModels: true,
			Message: "未找到 WorkBuddy 配置文件(~/.workbuddy/models.json)," +
				"请确认 WorkBuddy 已安装并至少运行过一次",
		}
	}
	return agentconf.Snapshot{
		Name:                 w.Name(),
		DisplayName:          w.DisplayName(),
		Status:               agentconf.StatusFound,
		ConfigPath:           path,
		Columns:              workBuddyColumns(w.collectEfforts()),
		SupportsAddModels:    true,
		SupportsRemoveModels: true,
		Message:              wbManageHint,
	}
}

// collectEfforts 汇总全部条目的 supportedEfforts(保序去重),与标准五档合并
// 后作为档位两列的候选项;文件缺失、解析失败或形态不受支持时返回空,此时
// 候选项仅含标准五档(effortOptions 恒非空)。
func (w *WorkBuddyAgent) collectEfforts() []string {
	root, err := w.readDoc()
	if err != nil {
		return nil
	}
	models, err := workBuddyModelsOf(root)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	efforts := make([]string, 0, 4)
	for _, e := range models {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		for _, effort := range stringSliceOf(mapGetObj(m, wbFileReasoning)["supportedEfforts"]) {
			if !seen[effort] {
				seen[effort] = true
				efforts = append(efforts, effort)
			}
		}
	}
	return efforts
}

// workBuddyColumns 模型表格列描述;档位两列候选项恒含标准五档并合并文件内
// 自定义档位(effortOptions,恒非空,default_effort 不再有 text 降级形态)。
// supported_efforts 为 list 列(白名单与 apply 逻辑本就支持,此列使其可见可编辑),
// 选项外的档位由前端自由输入;can_disable_thinking 的文件缺省即 true(仅 false
// 才落盘);WorkBuddy 无"默认模型"概念,能力位恒为 false(零值)。
func workBuddyColumns(efforts []string) []agentconf.FieldSpec {
	effortField := agentconf.FieldSpec{
		Key:     wbFieldDefaultEffort,
		Label:   "默认推理强度",
		Type:    "select",
		Options: effortOptions(efforts),
	}
	return []agentconf.FieldSpec{
		{Key: "model_id", Label: "模型 ID", Type: "text", Readonly: true},
		{Key: wbFieldName, Label: "显示名", Type: "text"},
		{Key: wbFieldURL, Label: "接口地址", Type: "text"},
		{Key: wbFieldDisabled, Label: "禁用", Type: "bool"},
		{Key: wbFieldSupportsToolCall, Label: "工具调用", Type: "bool"},
		{Key: wbFieldSupportsImages, Label: "图像输入", Type: "bool"},
		{Key: wbFieldSupportsReasoning, Label: "推理模式", Type: "bool"},
		effortField,
		{Key: wbFieldSupportedEfforts, Label: "支持档位", Type: "list", Options: effortOptions(efforts)},
		{Key: wbFieldCanDisableThinking, Label: "可关闭思考", Type: "bool"},
		{Key: wbFieldOnlyReasoning, Label: "仅推理", Type: "bool"},
		{Key: wbFieldUseCustomProtocol, Label: "URL 原样请求", Type: "bool"},
		{Key: wbFieldMaxInputTokens, Label: "最大输入 Token", Type: "number"},
		{Key: wbFieldMaxOutputTokens, Label: "最大输出 Token", Type: "number"},
		{Key: wbFieldTemperature, Label: "采样温度", Type: "number"},
		{Key: wbFileVendor, Label: "供应商", Type: "text", Readonly: true},
	}
}

// Models 实现 Agent 接口:返回模型清单(凭据脱敏)。
func (w *WorkBuddyAgent) Models(_ context.Context) ([]agentconf.ModelEntry, error) {
	root, err := w.readDoc()
	if err != nil {
		return nil, err
	}
	models, err := workBuddyModelsOf(root)
	if err != nil {
		return nil, err
	}
	return workBuddyEntries(models), nil
}

// ApplyModels 实现 Agent 接口:先整体校验全部 patch,再一次备份、树编辑、
// 原子写回,返回重新读取的最新清单。白名单外的键报 4403,定位不存在的
// modelId 报 4404。写回保持原顶层形态,日志只记改动键名,不记任何值。
func (w *WorkBuddyAgent) ApplyModels(ctx context.Context, patches []agentconf.ModelPatch, dataDir string) ([]agentconf.ModelEntry, error) {
	root, err := w.readDoc()
	if err != nil {
		return nil, err
	}
	models, err := workBuddyModelsOf(root)
	if err != nil {
		return nil, err
	}
	if err := validateWorkBuddyPatches(models, patches); err != nil {
		return nil, err
	}
	if len(patches) == 0 {
		// 空提交视为只读:不备份、不落盘,直接返回现状
		return workBuddyEntries(models), nil
	}
	// 备份先于写回:任何写回动作前必须先留一份可还原的副本
	if _, err := agentconf.Backup(w.modelsPath(), w.Name(), dataDir); err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "备份 WorkBuddy 配置失败", err)
	}
	for _, p := range patches {
		applyWorkBuddyPatch(models, p)
	}
	// 对象形态把(元素被原地修改的)数组回填 models 键,其余顶层键零丢失;
	// 裸数组形态直接整树写回
	if obj, ok := root.(map[string]any); ok {
		obj[wbFileModels] = models
	}
	if err := agentconf.WriteJSONFile(w.modelsPath(), root); err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "写回 WorkBuddy 配置失败", err)
	}
	slog.Info("WorkBuddy 模型配置已更新", "patches", len(patches), "fields", patchChangedKeys(patches))
	return w.Models(ctx)
}

// RemoveModels 实现 Agent 接口:先整体校验全部定位(按 modelId 首个匹配口径,
// 任一不存在报 4404,整批拒绝避免半批生效),再一次备份、过滤数组、原子写回,
// 返回最新清单。空 refs 视为只读:不备份、不落盘,直接返回现状。对象形态同步
// 移除顶层 availableModels 中命中的 id(该键缺失或不是数组则不触碰)。
func (w *WorkBuddyAgent) RemoveModels(ctx context.Context, refs []agentconf.ModelRef, dataDir string) ([]agentconf.ModelEntry, error) {
	root, err := w.readDoc()
	if err != nil {
		return nil, err
	}
	models, err := workBuddyModelsOf(root)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		// 空删除视为只读:不备份、不落盘,直接返回现状
		return workBuddyEntries(models), nil
	}
	for _, ref := range refs {
		if locateWorkBuddy(models, ref.ModelID) == nil {
			return nil, apperr.New(apperr.CodeAgentNotFound, "模型不存在:"+ref.ModelID)
		}
	}
	// 备份先于写回:任何写回动作前必须先留一份可还原的副本
	if _, err := agentconf.Backup(w.modelsPath(), w.Name(), dataDir); err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "备份 WorkBuddy 配置失败", err)
	}
	for _, ref := range refs {
		models = removeWorkBuddyEntry(models, ref.ModelID)
	}
	// 过滤会产生新切片:裸数组形态直接把过滤结果作为根值;对象形态回填
	// models 键,并按 id 同步 availableModels(存在则移除命中项,缺失或不是
	// 数组则不创建不触碰)
	if obj, ok := root.(map[string]any); ok {
		obj[wbFileModels] = models
		if arr, ok := obj["availableModels"].([]any); ok {
			for _, ref := range refs {
				arr = removeStringValues(arr, ref.ModelID)
			}
			obj["availableModels"] = arr
		}
	} else {
		root = models
	}
	if err := agentconf.WriteJSONFile(w.modelsPath(), root); err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "写回 WorkBuddy 配置失败", err)
	}
	// 日志只记删除数量与定位键,不记任何配置值
	slog.Info("WorkBuddy 模型已删除", "count", len(refs), "targets", refKeys(refs))
	return w.Models(ctx)
}

// removeWorkBuddyEntry 过滤模型数组:移除对象元素中 id 等于 modelID 的条目,
// 非对象元素原样保留。
func removeWorkBuddyEntry(arr []any, modelID string) []any {
	out := make([]any, 0, len(arr))
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok && stringOf(m["id"]) == modelID {
			continue
		}
		out = append(out, e)
	}
	return out
}

// AddModels 实现 Agent 接口:按来源接口逐条追加模型,数组内已有同 id 条目的
// 逐条跳过,全部跳过时不备份不落盘。新条目恰含 id/name/vendor/url/apiKey 五键
// (url 为完整 endpoint 语义,由 handler 从 Base URL 派生;apiKey 来自 ModelMeter
// 数据库装配的 Source,属"新增写入凭据"的显式例外),其余字段交给 WorkBuddy
// 缺省语义与用户后续编辑。对象形态下顶层 availableModels 为数组且不含该 id 时
// 追加(缺失不创建);裸数组形态无该概念,自然跳过。日志只记数量,不记 url/key 值。
func (w *WorkBuddyAgent) AddModels(ctx context.Context, req agentconf.AddModelsRequest, dataDir string) (agentconf.AddModelsResult, error) {
	root, err := w.readDoc()
	if err != nil {
		return agentconf.AddModelsResult{}, err
	}
	models, err := workBuddyModelsOf(root)
	if err != nil {
		return agentconf.AddModelsResult{}, err
	}
	if len(req.ModelIDs) == 0 {
		return agentconf.AddModelsResult{}, apperr.New(apperr.CodeAgentInvalid, "model_ids 不能为空")
	}
	// 逐 id 判重:数组内已有同 id 条目的跳过;新增 id 顺带登记,处理请求内重复
	existing := make(map[string]bool, len(models))
	for _, e := range models {
		if m, ok := e.(map[string]any); ok {
			if id := stringOf(m["id"]); id != "" {
				existing[id] = true
			}
		}
	}
	added, skipped := splitAddIDs(req.ModelIDs, existing)
	if len(added) == 0 {
		// 全部已存在:零落盘直接返回,不产生备份
		return agentconf.AddModelsResult{Entries: workBuddyEntries(models), Added: added, Skipped: skipped}, nil
	}
	// 备份先于写回:任何写回动作前必须先留一份可还原的副本
	if _, err := agentconf.Backup(w.modelsPath(), w.Name(), dataDir); err != nil {
		return agentconf.AddModelsResult{}, apperr.Wrap(apperr.CodeAgentFileIO, "备份 WorkBuddy 配置失败", err)
	}
	for _, id := range added {
		models = append(models, newFlatModelEntry(id, req.Source))
	}
	// 追加可能产生新切片:裸数组形态直接把追加结果作为根值;对象形态回填
	// models 键并按 id 同步 availableModels(存在且不含该 id 时追加,缺失不创建)
	if obj, ok := root.(map[string]any); ok {
		obj[wbFileModels] = models
		if arr, ok := obj["availableModels"].([]any); ok {
			for _, id := range added {
				if !stringArrayContains(arr, id) {
					arr = append(arr, id)
				}
			}
			obj["availableModels"] = arr
		}
	} else {
		root = models
	}
	if err := agentconf.WriteJSONFile(w.modelsPath(), root); err != nil {
		return agentconf.AddModelsResult{}, apperr.Wrap(apperr.CodeAgentFileIO, "写回 WorkBuddy 配置失败", err)
	}
	slog.Info("WorkBuddy 模型已添加", "count", len(added), "skipped", len(skipped))
	return w.addResult(ctx, added, skipped)
}

// addResult 重新读取最新清单并组装添加结果;写回已成功,重读失败按原样返回
// (与 ApplyModels 返回最新清单的口径一致)。
func (w *WorkBuddyAgent) addResult(ctx context.Context, added, skipped []string) (agentconf.AddModelsResult, error) {
	entries, err := w.Models(ctx)
	if err != nil {
		return agentconf.AddModelsResult{}, err
	}
	return agentconf.AddModelsResult{Entries: entries, Added: added, Skipped: skipped}, nil
}

// readDoc 读取配置文件并用 json.Decoder+UseNumber 解析为根值(裸数组或
// 对象形态,数字经 UseNumber 零精度丢失)。文件缺失报 4404;读取失败或
// 非法 JSON 报 4402;顶层形态的判定交给 workBuddyModelsOf。
func (w *WorkBuddyAgent) readDoc() (any, error) {
	path := w.modelsPath()
	if !agentconf.FileExists(path) {
		return nil, apperr.New(apperr.CodeAgentNotFound,
			"未找到 WorkBuddy 配置文件(~/.workbuddy/models.json),请确认 WorkBuddy 已安装并至少运行过一次")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "读取 WorkBuddy 配置失败", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "WorkBuddy 配置文件不是合法 JSON,请先在其原生工具中修复", err)
	}
	return root, nil
}

// workBuddyModelsOf 从根值取模型数组:裸数组返回自身;对象形态取 models 键
// (WorkBuddy 存储层兼容的两种顶层形态)。两者皆非时报 4402——无法定位模型
// 数组的文件不能当作 WorkBuddy 配置编辑,避免误改无关文件。
func workBuddyModelsOf(root any) ([]any, error) {
	switch v := root.(type) {
	case []any:
		return v, nil
	case map[string]any:
		arr, ok := v[wbFileModels].([]any)
		if !ok {
			return nil, apperr.New(apperr.CodeAgentFileIO,
				"WorkBuddy 配置文件顶层对象缺少 models 数组,无法定位模型清单")
		}
		return arr, nil
	default:
		return nil, apperr.New(apperr.CodeAgentFileIO,
			"WorkBuddy 配置文件顶层既不是数组也不含 models 数组,无法定位模型清单")
	}
}

// workBuddyEntries 由数组树构造模型清单;容忍非对象元素(跳过)。
func workBuddyEntries(arr []any) []agentconf.ModelEntry {
	entries := make([]agentconf.ModelEntry, 0, len(arr))
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			entries = append(entries, workBuddyEntry(m))
		}
	}
	return entries
}

// workBuddyEntry 单个元素的非凭据字段映射;reasoning 节点缺失时相应键取缺省。
// canDisableThinking 缺省 true:WorkBuddy 面板仅在该值为 false 时落盘,文件
// 缺省即 true;数字键文件未写时不出现(与 ZCode 数字列口径一致)。
func workBuddyEntry(m map[string]any) agentconf.ModelEntry {
	id := stringOf(m["id"])
	name := stringOf(m["name"])
	reasoning := mapGetObj(m, wbFileReasoning)
	fields := map[string]any{
		"model_id":                id,
		wbFieldName:               name,
		wbFieldURL:                stringOf(m["url"]),
		wbFieldDisabled:           boolDefault(m["disabled"]),
		wbFieldSupportsToolCall:   boolDefault(m["supportsToolCall"]),
		wbFieldSupportsImages:     boolDefault(m["supportsImages"]),
		wbFieldSupportsReasoning:  boolDefault(m["supportsReasoning"]),
		wbFieldOnlyReasoning:      boolDefault(m["onlyReasoning"]),
		wbFieldUseCustomProtocol:  boolDefault(m["useCustomProtocol"]),
		wbFieldDefaultEffort:      stringOf(reasoning["defaultEffort"]),
		wbFieldSupportedEfforts:   stringSliceOf(reasoning["supportedEfforts"]),
		wbFieldCanDisableThinking: true,
		wbFileVendor:              stringOf(m[wbFileVendor]),
	}
	if b, ok := boolOf(reasoning["canDisableThinking"]); ok {
		fields[wbFieldCanDisableThinking] = b
	}
	// default_effort 兜底 legacy 键 effort:面板读取顺序为 defaultEffort ?? effort
	if fields[wbFieldDefaultEffort] == "" {
		if legacy := stringOf(reasoning["effort"]); legacy != "" {
			fields[wbFieldDefaultEffort] = legacy
		}
	}
	if v, ok := m["maxInputTokens"]; ok {
		fields[wbFieldMaxInputTokens] = v // json.Number 原样透传,零精度丢失
	}
	if v, ok := m["maxOutputTokens"]; ok {
		fields[wbFieldMaxOutputTokens] = v
	}
	if v, ok := m["temperature"]; ok {
		fields[wbFieldTemperature] = v
	}
	return agentconf.ModelEntry{
		ModelID:     id,
		DisplayName: name,
		Fields:      fields,
	}
}

// locateWorkBuddy 按 modelId 首个匹配定位数组元素;找不到返回 nil。
func locateWorkBuddy(arr []any, modelID string) map[string]any {
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok && stringOf(m["id"]) == modelID {
			return m
		}
	}
	return nil
}

// validateWorkBuddyPatches 一次性校验全部 patch:定位必须存在,字段键必须在
// 白名单内且值类型正确。先整体校验再落盘,避免半批生效。
func validateWorkBuddyPatches(arr []any, patches []agentconf.ModelPatch) error {
	for _, p := range patches {
		if locateWorkBuddy(arr, p.ModelID) == nil {
			return apperr.New(apperr.CodeAgentNotFound, "模型不存在:"+p.ModelID)
		}
		for k, v := range p.Fields {
			switch k {
			case wbFieldName, wbFieldURL, wbFieldDefaultEffort:
				if _, ok := v.(string); !ok {
					return apperr.New(apperr.CodeAgentInvalid, "字段 "+k+" 必须为字符串")
				}
			case wbFieldDisabled, wbFieldSupportsToolCall, wbFieldSupportsImages,
				wbFieldSupportsReasoning, wbFieldOnlyReasoning, wbFieldUseCustomProtocol,
				wbFieldCanDisableThinking:
				if _, ok := v.(bool); !ok {
					return apperr.New(apperr.CodeAgentInvalid, "字段 "+k+" 必须为布尔值")
				}
			case wbFieldMaxInputTokens, wbFieldMaxOutputTokens, wbFieldTemperature:
				// nil 为显式清除:移除该数字键,恢复"文件未写"缺省态
				if v == nil {
					continue
				}
				if _, ok := numberValue(v); !ok {
					return apperr.New(apperr.CodeAgentInvalid, "字段 "+k+" 必须为数字")
				}
			case wbFieldSupportedEfforts:
				if !isStringSlice(v) {
					return apperr.New(apperr.CodeAgentInvalid, "字段 supported_efforts 必须为字符串数组")
				}
			default:
				return apperr.New(apperr.CodeAgentInvalid, "不支持修改字段 "+k+",仅允许名称、接口地址、能力开关与推理强度")
			}
		}
	}
	return nil
}

// applyWorkBuddyPatch 定位数组元素并应用白名单修改;reasoning 节点缺失时创建。
// id、vendor、apiKey、tags 及未知键不在白名单内,原样保留;数字经 numberValue
// 规范(整数不落科学计数法),nil 清除该键恢复缺省。
func applyWorkBuddyPatch(arr []any, p agentconf.ModelPatch) {
	m := locateWorkBuddy(arr, p.ModelID) // 校验阶段已保证存在
	for k, v := range p.Fields {
		switch k {
		case wbFieldName:
			m["name"] = v
		case wbFieldURL:
			m["url"] = v
		case wbFieldDisabled:
			m["disabled"] = v
		case wbFieldSupportsToolCall:
			m["supportsToolCall"] = v
		case wbFieldSupportsImages:
			m["supportsImages"] = v
		case wbFieldSupportsReasoning:
			m["supportsReasoning"] = v
		case wbFieldOnlyReasoning:
			m["onlyReasoning"] = v
		case wbFieldUseCustomProtocol:
			m["useCustomProtocol"] = v
		case wbFieldDefaultEffort:
			ensureMap(m, wbFileReasoning)["defaultEffort"] = v
		case wbFieldSupportedEfforts:
			ensureMap(m, wbFileReasoning)["supportedEfforts"] = stringSliceValue(v)
		case wbFieldCanDisableThinking:
			ensureMap(m, wbFileReasoning)["canDisableThinking"] = v
		case wbFieldMaxInputTokens:
			if v == nil {
				delete(m, "maxInputTokens") // 显式清除:恢复工具缺省语义
				continue
			}
			n, _ := numberValue(v)
			m["maxInputTokens"] = n
		case wbFieldMaxOutputTokens:
			if v == nil {
				delete(m, "maxOutputTokens")
				continue
			}
			n, _ := numberValue(v)
			m["maxOutputTokens"] = n
		case wbFieldTemperature:
			if v == nil {
				delete(m, "temperature")
				continue
			}
			n, _ := numberValue(v)
			m["temperature"] = n
		}
	}
}
