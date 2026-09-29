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
// maxOutputTokens、temperature、reasoning.defaultEffort、
// reasoning.supportedEfforts、reasoning.canDisableThinking;按 modelId 首
// 个匹配定位数组元素,reasoning 节点缺失时创建;id、vendor、apiKey、tags
// 及未知键逐条原样保留。顶层形态写回保持不变——对象形态只原地回填 models
// 数组,availableModels 等顶层键零丢失(WorkBuddy 自己会把对象重写回裸数组,
// 本适配器更保守)。模型条目的新增/删除与 apiKey 管理一律引导用户到
// WorkBuddy 原生工具操作。整体重编码时 map 键按字母序重排,JSON 语义零变化
// (沿用既有取舍)。WorkBuddy 无"默认模型"概念:能力位恒 false,不实现
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
const wbManageHint = "API Key 与模型条目的增删请在 WorkBuddy 原生工具中管理,本页仅调整模型配置;修改后建议重启 WorkBuddy 使配置生效"

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

// Snapshot 实现 Agent 接口:配置文件存在即 found;default_effort 下拉选项取
// 全部条目 supportedEfforts 的并集(保序去重),无选项时降级为文本输入。
func (w *WorkBuddyAgent) Snapshot(_ context.Context) agentconf.Snapshot {
	path := w.modelsPath()
	if !agentconf.FileExists(path) {
		return agentconf.Snapshot{
			Name:        w.Name(),
			DisplayName: w.DisplayName(),
			Status:      agentconf.StatusNotFound,
			ConfigPath:  path,
			Columns:     workBuddyColumns(nil),
			Message: "未找到 WorkBuddy 配置文件(~/.workbuddy/models.json)," +
				"请确认 WorkBuddy 已安装并至少运行过一次",
		}
	}
	return agentconf.Snapshot{
		Name:        w.Name(),
		DisplayName: w.DisplayName(),
		Status:      agentconf.StatusFound,
		ConfigPath:  path,
		Columns:     workBuddyColumns(w.collectEfforts()),
		Message:     wbManageHint,
	}
}

// collectEfforts 汇总全部条目的 supportedEfforts(保序去重),作为
// default_effort 列的下拉选项;文件缺失、解析失败或形态不受支持时返回空,
// 由调用方降级为文本。
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

// workBuddyColumns 模型表格列描述;efforts 非空时 default_effort 为下拉列。
// supported_efforts 为 list 列(白名单与 apply 逻辑本就支持,此列使其可见可编辑),
// 选项留空由前端自由输入;can_disable_thinking 的文件缺省即 true(仅 false
// 才落盘);WorkBuddy 无"默认模型"概念,能力位恒为 false(零值)。
func workBuddyColumns(efforts []string) []agentconf.FieldSpec {
	effortField := agentconf.FieldSpec{Key: wbFieldDefaultEffort, Label: "默认推理强度", Type: "text"}
	if len(efforts) > 0 {
		effortField.Type = "select"
		effortField.Options = make([]agentconf.FieldOption, 0, len(efforts))
		for _, e := range efforts {
			effortField.Options = append(effortField.Options, agentconf.FieldOption{Value: e, Label: e})
		}
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
		{Key: wbFieldSupportedEfforts, Label: "支持档位", Type: "list"},
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
// 规范(整数不落科学计数法)。
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
			n, _ := numberValue(v)
			m["maxInputTokens"] = n
		case wbFieldMaxOutputTokens:
			n, _ := numberValue(v)
			m["maxOutputTokens"] = n
		case wbFieldTemperature:
			n, _ := numberValue(v)
			m["temperature"] = n
		}
	}
}
