// codebuddy.go CodeBuddy 模型配置适配:目标文件 ~/.codebuddy/models.json。
// 真实 schema(勘探见任务 09-30-codebuddy-agent-conf 的 research.md,提取自
// CodeBuddy IDE 程序代码:主进程 models.json IPC 服务与内置 Agent 运行时
// LocalCustomModelStoreImpl):顶层是对象形态 {"models": [...],
// "availableModels": [...]}——与 WorkBuddy 同源代码,但 CodeBuddy 读取侧对
// 裸数组取不到模型、写回侧会丢数据,仅对象形态可用,裸数组一律拒绝(4402);
// 对象缺 models 数组是"新建后未添加模型"的正常态,按空清单处理(对齐程序
// buildResponse)。availableModels 只承载模型选择器可见性,本适配器零接触。
//
// 读侧:models 数组遍历为模型清单,字段脱敏映射(id/name/vendor/url/能力
// 开关/数字项/reasoning);无字符串 id 的条目按运行时口径跳过;apiKey 绝不
// 读取进输出结构。缺省语义对齐 CodeBuddy 运行时:disabled/onlyReasoning/
// 能力开关缺省 false,reasoning.canDisableThinking 缺省 true(仅 false 才
// 落盘),defaultEffort 兜底 legacy 键 effort,数字项文件未写时该键不出现。
//
// 写侧:白名单修改 name、url、disabled、supportsToolCall、supportsImages、
// supportsReasoning、onlyReasoning、maxInputTokens、maxOutputTokens、
// temperature、reasoning.defaultEffort、reasoning.supportedEfforts、
// reasoning.canDisableThinking(CodeBuddy 无 useCustomProtocol 键);按 id
// 首个匹配定位数组元素,reasoning 节点缺失时创建;id、vendor、apiKey、
// availableModels 及未知键逐条原样保留。顶层形态恒为对象,写回时把(元素
// 被原地修改的)数组回填 models 键,其余顶层键零丢失。
//
// 模型删除(RemoveModels)与添加(AddModels):删除按定位过滤数组元素(非
// 对象元素原样保留),顶层 availableModels 同步移除命中的 id;添加按来源接口
// 追加条目(恰含 id/name/vendor/url/apiKey 五键,url 为完整 endpoint,由
// handler 从 Base URL 派生),apiKey 来自 ModelMeter 数据库装配的 Source,属
// "新增写入凭据"的显式例外(经用户界面确认),既有条目的凭据仍零接触;
// availableModels 为数组且不含该 id 时追加,缺失不创建。日志只记数量与定位键,
// 绝不记 url/key 值。
//
// CodeBuddy 无"默认模型"概念:能力位恒 false,不实现 DefaultModelSetter
// (handler 对其默认模型请求报 4405)。
package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/service/agentconf"
)

// CodeBuddyAgent CodeBuddy 的 Agent 接口实现;homeDir 构造注入,便于测试。
type CodeBuddyAgent struct {
	homeDir string
}

// NewCodeBuddyAgent 构造 CodeBuddy 适配器。
func NewCodeBuddyAgent(homeDir string) *CodeBuddyAgent {
	return &CodeBuddyAgent{homeDir: homeDir}
}

// Name 实现 Agent 接口;同时用作 URL 标识与备份目录名。
func (c *CodeBuddyAgent) Name() string { return "codebuddy" }

// DisplayName 实现 Agent 接口;界面展示名。
func (c *CodeBuddyAgent) DisplayName() string { return "CodeBuddy" }

// modelsPath 目标配置文件路径,读与写回的唯一目标。
func (c *CodeBuddyAgent) modelsPath() string {
	return filepath.Join(c.homeDir, ".codebuddy", "models.json")
}

// cbManageHint 管理边界提示,卡片说明共用。CodeBuddy 运行时对 models.json
// 做文件监听(1s debounce)热同步,通常无需重启,仅兜底提示。
const cbManageHint = "既有条目的 API Key 请在 CodeBuddy 原生工具中管理,本页支持模型清单的增删与配置调整;" +
	"修改后 CodeBuddy 会自动加载,若未生效请重启 CodeBuddy"

// 白名单字段键:Fields 与 patch 中允许出现的键。
const (
	cbFieldName               = "name"
	cbFieldURL                = "url"
	cbFieldDisabled           = "disabled"
	cbFieldSupportsToolCall   = "supports_tool_call"
	cbFieldSupportsImages     = "supports_images"
	cbFieldSupportsReasoning  = "supports_reasoning"
	cbFieldOnlyReasoning      = "only_reasoning"
	cbFieldDefaultEffort      = "default_effort"
	cbFieldSupportedEfforts   = "supported_efforts"
	cbFieldCanDisableThinking = "can_disable_thinking"
	cbFieldMaxInputTokens     = "max_input_tokens"
	cbFieldMaxOutputTokens    = "max_output_tokens"
	cbFieldTemperature        = "temperature"
)

// 文件侧键名:白名单字段到 models.json 元素键的映射(其余布尔/数字键与
// 文件键同名,apply 分支内直接写字面量)。
const (
	cbFileReasoning = "reasoning"
	cbFileVendor    = "vendor"
	cbFileModels    = "models" // 承载模型数组的顶层键
)

// Snapshot 实现 Agent 接口:配置文件存在即 found;default_effort 下拉选项取
// 全部条目 supportedEfforts 的并集(保序去重),无选项时降级为文本输入。
// 增删模型能力位恒置 true(能力是工具属性,文件缺失时同样成立);CodeBuddy
// 无"默认模型"概念,该能力位恒 false(零值)。
func (c *CodeBuddyAgent) Snapshot(_ context.Context) agentconf.Snapshot {
	path := c.modelsPath()
	if !agentconf.FileExists(path) {
		return agentconf.Snapshot{
			Name:                 c.Name(),
			DisplayName:          c.DisplayName(),
			Status:               agentconf.StatusNotFound,
			ConfigPath:           path,
			Columns:              codeBuddyColumns(nil),
			SupportsAddModels:    true,
			SupportsRemoveModels: true,
			Message: "未找到 CodeBuddy 配置文件(~/.codebuddy/models.json)," +
				"请确认 CodeBuddy 已安装并至少运行过一次",
		}
	}
	return agentconf.Snapshot{
		Name:                 c.Name(),
		DisplayName:          c.DisplayName(),
		Status:               agentconf.StatusFound,
		ConfigPath:           path,
		Columns:              codeBuddyColumns(c.collectEfforts()),
		SupportsAddModels:    true,
		SupportsRemoveModels: true,
		Message:              cbManageHint,
	}
}

// collectEfforts 汇总全部条目的 supportedEfforts(保序去重),作为
// default_effort 列的下拉选项;文件缺失、解析失败或顶层形态不受支持时
// 返回空,由调用方降级为文本(收集方式对齐 WorkBuddy 的 collectEfforts)。
func (c *CodeBuddyAgent) collectEfforts() []string {
	root, err := c.readDoc()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	efforts := make([]string, 0, 4)
	for _, e := range codeBuddyModelsOf(root) {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		for _, effort := range stringSliceOf(mapGetObj(m, cbFileReasoning)["supportedEfforts"]) {
			if !seen[effort] {
				seen[effort] = true
				efforts = append(efforts, effort)
			}
		}
	}
	return efforts
}

// codeBuddyColumns 模型表格列描述;efforts 非空时 default_effort 为下拉列。
// supported_efforts 为 list 列(白名单与 apply 逻辑本就支持,此列使其可见
// 可编辑),选项留空由前端自由输入;can_disable_thinking 的文件缺省即 true
// (仅 false 才落盘);CodeBuddy 无"默认模型"概念,能力位恒为 false(零值)。
func codeBuddyColumns(efforts []string) []agentconf.FieldSpec {
	effortField := agentconf.FieldSpec{Key: cbFieldDefaultEffort, Label: "默认推理强度", Type: "text"}
	if len(efforts) > 0 {
		effortField.Type = "select"
		effortField.Options = make([]agentconf.FieldOption, 0, len(efforts))
		for _, e := range efforts {
			effortField.Options = append(effortField.Options, agentconf.FieldOption{Value: e, Label: e})
		}
	}
	return []agentconf.FieldSpec{
		{Key: "model_id", Label: "模型 ID", Type: "text", Readonly: true},
		{Key: cbFieldName, Label: "显示名", Type: "text"},
		{Key: cbFieldURL, Label: "接口地址", Type: "text"},
		{Key: cbFieldDisabled, Label: "禁用", Type: "bool"},
		{Key: cbFieldSupportsToolCall, Label: "工具调用", Type: "bool"},
		{Key: cbFieldSupportsImages, Label: "图像输入", Type: "bool"},
		{Key: cbFieldSupportsReasoning, Label: "推理模式", Type: "bool"},
		effortField,
		{Key: cbFieldSupportedEfforts, Label: "支持档位", Type: "list"},
		{Key: cbFieldCanDisableThinking, Label: "可关闭思考", Type: "bool"},
		{Key: cbFieldOnlyReasoning, Label: "仅推理", Type: "bool"},
		{Key: cbFieldMaxInputTokens, Label: "最大输入 Token", Type: "number"},
		{Key: cbFieldMaxOutputTokens, Label: "最大输出 Token", Type: "number"},
		{Key: cbFieldTemperature, Label: "采样温度", Type: "number"},
		{Key: cbFileVendor, Label: "供应商", Type: "text", Readonly: true},
	}
}

// Models 实现 Agent 接口:返回模型清单(凭据脱敏)。
func (c *CodeBuddyAgent) Models(_ context.Context) ([]agentconf.ModelEntry, error) {
	root, err := c.readDoc()
	if err != nil {
		return nil, err
	}
	return codeBuddyEntries(codeBuddyModelsOf(root)), nil
}

// ApplyModels 实现 Agent 接口:先整体校验全部 patch,再一次备份、树编辑、
// 原子写回,返回重新读取的最新清单。白名单外的键报 4403,定位不存在的
// id 报 4404。写回恒为对象形态,日志只记改动键名,不记任何值。
func (c *CodeBuddyAgent) ApplyModels(ctx context.Context, patches []agentconf.ModelPatch, dataDir string) ([]agentconf.ModelEntry, error) {
	root, err := c.readDoc()
	if err != nil {
		return nil, err
	}
	models := codeBuddyModelsOf(root)
	if err := validateCodeBuddyPatches(models, patches); err != nil {
		return nil, err
	}
	if len(patches) == 0 {
		// 空提交视为只读:不备份、不落盘,直接返回现状
		return codeBuddyEntries(models), nil
	}
	// 备份先于写回:任何写回动作前必须先留一份可还原的副本
	if _, err := agentconf.Backup(c.modelsPath(), c.Name(), dataDir); err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "备份 CodeBuddy 配置失败", err)
	}
	for _, p := range patches {
		applyCodeBuddyPatch(models, p)
	}
	// 元素被原地修改后把数组回填 models 键;availableModels 等其余顶层键
	// 不触碰即零丢失
	root[cbFileModels] = models
	if err := agentconf.WriteJSONFile(c.modelsPath(), root); err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "写回 CodeBuddy 配置失败", err)
	}
	slog.Info("CodeBuddy 模型配置已更新", "patches", len(patches), "fields", patchChangedKeys(patches))
	return c.Models(ctx)
}

// RemoveModels 实现 Agent 接口:先整体校验全部定位(按 id 首个匹配口径,任一
// 不存在报 4404,整批拒绝避免半批生效),再一次备份、过滤数组、原子写回,返回
// 最新清单。空 refs 视为只读:不备份、不落盘,直接返回现状。顶层 availableModels
// 同步移除命中的 id(该键缺失或不是数组则不触碰)。
func (c *CodeBuddyAgent) RemoveModels(ctx context.Context, refs []agentconf.ModelRef, dataDir string) ([]agentconf.ModelEntry, error) {
	root, err := c.readDoc()
	if err != nil {
		return nil, err
	}
	models := codeBuddyModelsOf(root)
	if len(refs) == 0 {
		// 空删除视为只读:不备份、不落盘,直接返回现状
		return codeBuddyEntries(models), nil
	}
	for _, ref := range refs {
		if locateCodeBuddy(models, ref.ModelID) == nil {
			return nil, apperr.New(apperr.CodeAgentNotFound, "模型不存在:"+ref.ModelID)
		}
	}
	// 备份先于写回:任何写回动作前必须先留一份可还原的副本
	if _, err := agentconf.Backup(c.modelsPath(), c.Name(), dataDir); err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "备份 CodeBuddy 配置失败", err)
	}
	for _, ref := range refs {
		models = removeCodeBuddyEntry(models, ref.ModelID)
	}
	// 过滤后的数组回填 models 键,并按 id 同步 availableModels(存在则移除命中项,
	// 缺失或不是数组则不创建不触碰);其余顶层键不触碰即零丢失
	root[cbFileModels] = models
	if arr, ok := root["availableModels"].([]any); ok {
		for _, ref := range refs {
			arr = removeStringValues(arr, ref.ModelID)
		}
		root["availableModels"] = arr
	}
	if err := agentconf.WriteJSONFile(c.modelsPath(), root); err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "写回 CodeBuddy 配置失败", err)
	}
	// 日志只记删除数量与定位键,不记任何配置值
	slog.Info("CodeBuddy 模型已删除", "count", len(refs), "targets", refKeys(refs))
	return c.Models(ctx)
}

// removeCodeBuddyEntry 过滤模型数组:移除对象元素中 id 等于 modelID 的条目,
// 非对象元素原样保留。
func removeCodeBuddyEntry(arr []any, modelID string) []any {
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
// 数据库装配的 Source,属"新增写入凭据"的显式例外),其余字段交给 CodeBuddy
// 缺省语义与用户后续编辑。顶层 availableModels 为数组且不含该 id 时追加
// (缺失不创建)。日志只记数量,不记 url/key 值。
func (c *CodeBuddyAgent) AddModels(ctx context.Context, req agentconf.AddModelsRequest, dataDir string) (agentconf.AddModelsResult, error) {
	root, err := c.readDoc()
	if err != nil {
		return agentconf.AddModelsResult{}, err
	}
	models := codeBuddyModelsOf(root)
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
		return agentconf.AddModelsResult{Entries: codeBuddyEntries(models), Added: added, Skipped: skipped}, nil
	}
	// 备份先于写回:任何写回动作前必须先留一份可还原的副本
	if _, err := agentconf.Backup(c.modelsPath(), c.Name(), dataDir); err != nil {
		return agentconf.AddModelsResult{}, apperr.Wrap(apperr.CodeAgentFileIO, "备份 CodeBuddy 配置失败", err)
	}
	for _, id := range added {
		models = append(models, newFlatModelEntry(id, req.Source))
	}
	// 数组回填 models 键,并按 id 同步 availableModels(存在且不含该 id 时追加,
	// 缺失不创建);其余顶层键不触碰即零丢失
	root[cbFileModels] = models
	if arr, ok := root["availableModels"].([]any); ok {
		for _, id := range added {
			if !stringArrayContains(arr, id) {
				arr = append(arr, id)
			}
		}
		root["availableModels"] = arr
	}
	if err := agentconf.WriteJSONFile(c.modelsPath(), root); err != nil {
		return agentconf.AddModelsResult{}, apperr.Wrap(apperr.CodeAgentFileIO, "写回 CodeBuddy 配置失败", err)
	}
	slog.Info("CodeBuddy 模型已添加", "count", len(added), "skipped", len(skipped))
	entries, err := c.Models(ctx)
	if err != nil {
		return agentconf.AddModelsResult{}, err
	}
	return agentconf.AddModelsResult{Entries: entries, Added: added, Skipped: skipped}, nil
}

// readDoc 读取配置文件并用 json.Decoder+UseNumber 解析为根对象(数字经
// UseNumber 零精度丢失)。文件缺失报 4404;读取失败或非法 JSON 报 4402;
// 顶层非对象报 4402——对齐程序 loadFile 的 "Invalid models.json format",
// 无法定位模型清单载体的文件不能当作 CodeBuddy 配置编辑。
func (c *CodeBuddyAgent) readDoc() (map[string]any, error) {
	path := c.modelsPath()
	if !agentconf.FileExists(path) {
		return nil, apperr.New(apperr.CodeAgentNotFound,
			"未找到 CodeBuddy 配置文件(~/.codebuddy/models.json),请确认 CodeBuddy 已安装并至少运行过一次")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "读取 CodeBuddy 配置失败", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return nil, apperr.Wrap(apperr.CodeAgentFileIO, "CodeBuddy 配置文件不是合法 JSON,请先在其原生工具中修复", err)
	}
	obj, ok := root.(map[string]any)
	if !ok {
		// CodeBuddy 仅支持对象形态:裸数组读不到模型、写回会丢数据,拒绝编辑
		return nil, apperr.New(apperr.CodeAgentFileIO,
			"CodeBuddy 配置文件顶层不是对象(CodeBuddy 仅支持 {\"models\": [...]} 形态),无法定位模型清单")
	}
	return obj, nil
}

// codeBuddyModelsOf 从根对象取模型数组:缺失或不是数组返回空切片——对齐
// 程序 buildResponse 的 Array.isArray 口径,"新建后未添加模型"的文件本就
// 没有 models 键(空清单下任何 patch 定位必然 4404,无误写风险)。
func codeBuddyModelsOf(root map[string]any) []any {
	return anySlice(root[cbFileModels])
}

// codeBuddyEntries 由数组树构造模型清单;跳过非对象元素与无有效 id 的元素
// (对齐 CodeBuddy 运行时加载过滤:!!ir?.id && typeof ir.id === "string")。
func codeBuddyEntries(arr []any) []agentconf.ModelEntry {
	entries := make([]agentconf.ModelEntry, 0, len(arr))
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if id := stringOf(m["id"]); strings.TrimSpace(id) != "" {
			entries = append(entries, codeBuddyEntry(m, id))
		}
	}
	return entries
}

// codeBuddyEntry 单个元素的非凭据字段映射;reasoning 节点缺失时相应键取缺省。
// canDisableThinking 缺省 true:CodeBuddy 运行时仅在该值为 false 时视为思考
// 恒开,文件缺省即 true;数字键文件未写时不出现(与 WorkBuddy 数字列口径
// 一致)。
func codeBuddyEntry(m map[string]any, id string) agentconf.ModelEntry {
	name := stringOf(m["name"])
	reasoning := mapGetObj(m, cbFileReasoning)
	fields := map[string]any{
		"model_id":                id,
		cbFieldName:               name,
		cbFieldURL:                stringOf(m["url"]),
		cbFieldDisabled:           boolDefault(m["disabled"]),
		cbFieldSupportsToolCall:   boolDefault(m["supportsToolCall"]),
		cbFieldSupportsImages:     boolDefault(m["supportsImages"]),
		cbFieldSupportsReasoning:  boolDefault(m["supportsReasoning"]),
		cbFieldOnlyReasoning:      boolDefault(m["onlyReasoning"]),
		cbFieldDefaultEffort:      stringOf(reasoning["defaultEffort"]),
		cbFieldSupportedEfforts:   stringSliceOf(reasoning["supportedEfforts"]),
		cbFieldCanDisableThinking: true,
		cbFileVendor:              stringOf(m[cbFileVendor]),
	}
	if b, ok := boolOf(reasoning["canDisableThinking"]); ok {
		fields[cbFieldCanDisableThinking] = b
	}
	// default_effort 兜底 legacy 键 effort:运行时 resolveReasoningEffort
	// 在 defaultEffort 缺失时同样回退读取 effort
	if fields[cbFieldDefaultEffort] == "" {
		if legacy := stringOf(reasoning["effort"]); legacy != "" {
			fields[cbFieldDefaultEffort] = legacy
		}
	}
	if v, ok := m["maxInputTokens"]; ok {
		fields[cbFieldMaxInputTokens] = v // json.Number 原样透传,零精度丢失
	}
	if v, ok := m["maxOutputTokens"]; ok {
		fields[cbFieldMaxOutputTokens] = v
	}
	if v, ok := m["temperature"]; ok {
		fields[cbFieldTemperature] = v
	}
	return agentconf.ModelEntry{
		ModelID:     id,
		DisplayName: name,
		Fields:      fields,
	}
}

// locateCodeBuddy 按 id 首个匹配定位数组元素;找不到返回 nil。
func locateCodeBuddy(arr []any, modelID string) map[string]any {
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok && stringOf(m["id"]) == modelID {
			return m
		}
	}
	return nil
}

// validateCodeBuddyPatches 一次性校验全部 patch:定位必须存在,字段键必须在
// 白名单内且值类型正确。先整体校验再落盘,避免半批生效。
func validateCodeBuddyPatches(arr []any, patches []agentconf.ModelPatch) error {
	for _, p := range patches {
		if locateCodeBuddy(arr, p.ModelID) == nil {
			return apperr.New(apperr.CodeAgentNotFound, "模型不存在:"+p.ModelID)
		}
		for k, v := range p.Fields {
			switch k {
			case cbFieldName, cbFieldURL, cbFieldDefaultEffort:
				if _, ok := v.(string); !ok {
					return apperr.New(apperr.CodeAgentInvalid, "字段 "+k+" 必须为字符串")
				}
			case cbFieldDisabled, cbFieldSupportsToolCall, cbFieldSupportsImages,
				cbFieldSupportsReasoning, cbFieldOnlyReasoning, cbFieldCanDisableThinking:
				if _, ok := v.(bool); !ok {
					return apperr.New(apperr.CodeAgentInvalid, "字段 "+k+" 必须为布尔值")
				}
			case cbFieldMaxInputTokens, cbFieldMaxOutputTokens, cbFieldTemperature:
				if _, ok := numberValue(v); !ok {
					return apperr.New(apperr.CodeAgentInvalid, "字段 "+k+" 必须为数字")
				}
			case cbFieldSupportedEfforts:
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

// applyCodeBuddyPatch 定位数组元素并应用白名单修改;reasoning 节点缺失时创建。
// id、vendor、apiKey、availableModels 及未知键不在白名单内,原样保留;数字经
// numberValue 规范(整数不落科学计数法)。
func applyCodeBuddyPatch(arr []any, p agentconf.ModelPatch) {
	m := locateCodeBuddy(arr, p.ModelID) // 校验阶段已保证存在
	for k, v := range p.Fields {
		switch k {
		case cbFieldName:
			m["name"] = v
		case cbFieldURL:
			m["url"] = v
		case cbFieldDisabled:
			m["disabled"] = v
		case cbFieldSupportsToolCall:
			m["supportsToolCall"] = v
		case cbFieldSupportsImages:
			m["supportsImages"] = v
		case cbFieldSupportsReasoning:
			m["supportsReasoning"] = v
		case cbFieldOnlyReasoning:
			m["onlyReasoning"] = v
		case cbFieldDefaultEffort:
			ensureMap(m, cbFileReasoning)["defaultEffort"] = v
		case cbFieldSupportedEfforts:
			ensureMap(m, cbFileReasoning)["supportedEfforts"] = stringSliceValue(v)
		case cbFieldCanDisableThinking:
			ensureMap(m, cbFileReasoning)["canDisableThinking"] = v
		case cbFieldMaxInputTokens:
			n, _ := numberValue(v)
			m["maxInputTokens"] = n
		case cbFieldMaxOutputTokens:
			n, _ := numberValue(v)
			m["maxOutputTokens"] = n
		case cbFieldTemperature:
			n, _ := numberValue(v)
			m["temperature"] = n
		}
	}
}
