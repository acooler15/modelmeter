// Package agentconf 提供本机 Agent 工具(ZCode、WorkBuddy 等)模型配置的
// 读取、写回与备份还原、模型条目的批量删除(RemoveModels)与添加(AddModels)
// 能力,以及"默认模型"(如 ZCode 的 config.defaultModelSelection)的读取与
// 设置/清除——后者为可选能力,由 DefaultModelSetter 接口表达,handler 按类型
// 断言探测。扩展点为 Agent 接口 + 注册表:具体实现放在 agents/ 子包中,实现
// Agent 接口并在其包内 init 自注册(由 cmd/server/main.go 空导入触发),
// handler 与前端无需为单个工具改动。
//
// 安全红线(所有实现共同遵守):绝不读取、回传或修改既有配置的凭据字段
// (如 API Key);日志不得出现任何凭据内容;只对明确的配置文件做最小化修改。
// 显式例外有二:(1) ZCode 的 defaultModelSelection 节点整体替换
// (见 agents/zcode.go);(2) 添加模型路径可把 handler 从本系统数据库装配的
// 凭据(Source.APIKey)写入新增条目或 ZCode 新建供应商节点——该写入经用户
// 界面显式确认,凭据只在 handler → ModelSource → 适配器落盘的单一通路上流动,
// 既有条目的凭据仍零接触。
package agentconf

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"sort"
	"sync"
)

// 配置现状状态值(Snapshot.Status)。
const (
	StatusFound    = "found"     // 配置文件已找到
	StatusNotFound = "not_found" // 未找到配置文件
)

// FieldOption select 字段的一个选项:Value 为写回值,Label 为展示名。
type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// FieldSpec 字段描述,既描述模型表格的"列",也承载该列的可编辑性。
type FieldSpec struct {
	Key      string        `json:"key"`
	Label    string        `json:"label"`
	Type     string        `json:"type"`              // "text" | "select" | "number" | "bool" | "list"
	Options  []FieldOption `json:"options,omitempty"` // select 为单选下拉;list 为多选(可自由添加)
	Required bool          `json:"required"`
	Help     string        `json:"help,omitempty"`     // 字段说明,前端展示
	Readonly bool          `json:"readonly,omitempty"` // 只读列仅展示,不接受提交
}

// Snapshot Agent 配置现状视图(不回传任何凭据)。
type Snapshot struct {
	Name        string      `json:"name"`
	DisplayName string      `json:"display_name"`
	Status      string      `json:"status"` // StatusFound | StatusNotFound
	ConfigPath  string      `json:"config_path"`
	Columns     []FieldSpec `json:"columns"`           // 模型表格的列描述
	Message     string      `json:"message,omitempty"` // 指引或管理边界说明
	// SupportsDefaultModel 能力位:该工具是否有"默认模型"概念。能力是工具
	// 属性,配置文件缺失(not_found)时同样置位;必须与 DefaultModelSetter
	// 接口实现保持一致,由 agents 包一致性测试兜底。
	SupportsDefaultModel bool `json:"supports_default_model"`
	// DefaultModel 当前默认模型,nil=未设置或不支持该能力。
	DefaultModel *DefaultModel `json:"default_model,omitempty"`
	// SupportsAddModels / SupportsRemoveModels 能力位:该工具是否支持添加/
	// 删除模型条目。能力是工具属性,配置文件缺失(not_found)时同样置位;
	// 必须与 Agent 接口的 AddModels/RemoveModels 实现保持一致,由 agents 包
	// 一致性测试兜底。
	SupportsAddModels    bool `json:"supports_add_models"`
	SupportsRemoveModels bool `json:"supports_remove_models"`
	// AddTargets 添加模型的可挂靠供应商清单(非敏感字段);仅 ZCode 且配置
	// 文件 found 时填充,其余工具为空。
	AddTargets []AddTarget `json:"add_targets,omitempty"`
}

// DefaultModel "默认模型"现状:对应 ZCode config.defaultModelSelection。
type DefaultModel struct {
	ProviderID     string `json:"provider_id"`
	ModelID        string `json:"model_id"`
	ReasoningLevel string `json:"reasoning_level,omitempty"`
}

// DefaultModelPatch 默认模型修改:provider_id+model_id 均空=清除,均非空=设置,
// 一空一非空由实现报 4403。
type DefaultModelPatch struct {
	ProviderID     string `json:"provider_id"`
	ModelID        string `json:"model_id"`
	ReasoningLevel string `json:"reasoning_level,omitempty"` // 仅设置时有效,空=不写 options
}

// ModelEntry 模型清单中的一个模型条目(凭据绝不包含)。
type ModelEntry struct {
	ProviderID   string         `json:"provider_id,omitempty"` // ZCode 有供应商层;WorkBuddy 为空
	ProviderName string         `json:"provider_name,omitempty"`
	ModelID      string         `json:"model_id"`
	DisplayName  string         `json:"display_name,omitempty"`
	Fields       map[string]any `json:"fields"` // 非凭据字段:展示 + 可编辑,键集=Columns 的 key 子集
}

// ModelPatch 对一个模型条目的白名单局部修改。
type ModelPatch struct {
	ProviderID string         `json:"provider_id,omitempty"` // ZCode 定位键;WorkBuddy 为空
	ModelID    string         `json:"model_id"`
	Fields     map[string]any `json:"fields"` // 仅白名单键;含白名单外键 → 4403
}

// ModelRef 删除目标定位:provider_id 仅 ZCode 有,WorkBuddy/CodeBuddy 为空。
type ModelRef struct {
	ProviderID string `json:"provider_id,omitempty"`
	ModelID    string `json:"model_id"`
}

// AddTarget ZCode 添加模型的可挂靠供应商(非敏感字段,Snapshot 携带)。
type AddTarget struct {
	ProviderID   string `json:"provider_id"`
	ProviderName string `json:"provider_name,omitempty"`
}

// ModelSource 添加模型的来源接口信息;handler 从数据库的 provider 记录装配,
// 凭据只进不出(不回显、不进日志,仅由适配器写入新增条目/新建供应商节点)。
type ModelSource struct {
	ProviderName string // vendor/显示名缺省值
	BaseURL      string // ZCode 新建供应商的 api.baseUrl
	EndpointURL  string // WorkBuddy/CodeBuddy 新条目的 url(完整 endpoint,handler 派生)
	APIKey       string // 仅"新增写入凭据"路径落盘;ZCode 挂已有供应商不使用
}

// AddTargetSpec ZCode 落点规格;WorkBuddy/CodeBuddy 整体忽略。
// json 标签对齐 handler 请求体 target 节点的键(snake_case)。
type AddTargetSpec struct {
	Mode         string `json:"mode"`          // "existing" | "new"(空按 existing 处理)
	ProviderID   string `json:"provider_id"`   // Mode=existing 必填
	ProviderName string `json:"provider_name"` // Mode=new 显示名,空回退 Source.ProviderName
	APIType      string `json:"api_type"`      // Mode=new 取 "openai-chat-completions"|"openai-responses",空缺省前者
}

// AddModelsRequest 一次添加请求;ModelIDs 为显式清单("全部"由前端全选产生)。
type AddModelsRequest struct {
	ModelIDs []string
	Source   ModelSource
	Target   AddTargetSpec
}

// AddModelsResult 添加结果:最新清单 + 逐项成败,前端据此反馈"成功 N/跳过 M"。
type AddModelsResult struct {
	Entries []ModelEntry `json:"entries"`
	Added   []string     `json:"added"`
	Skipped []string     `json:"skipped"` // 已存在而跳过
}

// Agent 扩展点:本机 Agent 工具的模型配置适配器。
// 新工具在 agents/ 子包实现本接口并 init 注册即可接入,调用方无需修改。
type Agent interface {
	// Name 稳定标识:用于 URL 路径与备份目录名,小写英文。
	Name() string
	// DisplayName 界面展示名。
	DisplayName() string
	// Snapshot 返回配置现状;实现需自行降级(如文件缺失),不返回错误。
	Snapshot(ctx context.Context) Snapshot
	// Models 返回模型清单(凭据脱敏);文件缺失报 4404,非法 JSON 报 4402。
	Models(ctx context.Context) ([]ModelEntry, error)
	// ApplyModels 按白名单批量局部修改:多 patch 一次备份、一次读、一次写;
	// dataDir 为运行数据目录,备份落在 dataDir/agent-backups/<Name>/ 下。
	// 返回重新读取的最新清单。
	ApplyModels(ctx context.Context, patches []ModelPatch, dataDir string) ([]ModelEntry, error)
	// RemoveModels 按定位批量删除:整体校验(不存在报 4404)→ 一次备份 →
	// 树编辑 → 原子写回;空 refs 只读返回现状(不备份不落盘),口径同
	// ApplyModels 空提交。返回删除后的最新清单。
	RemoveModels(ctx context.Context, refs []ModelRef, dataDir string) ([]ModelEntry, error)
	// AddModels 逐条添加,已存在跳过;全部跳过时不备份不落盘。返回最新清单
	// 与逐项成败(added/skipped)。
	AddModels(ctx context.Context, req AddModelsRequest, dataDir string) (AddModelsResult, error)
}

// DefaultModelSetter 可选能力接口:支持读写"默认模型"的 Agent 额外实现
// (当前仅 ZCode;WorkBuddy 无此概念,不实现)。handler 按类型断言探测,
// 未实现报 4405。能力位(Snapshot.SupportsDefaultModel)与本接口实现必须
// 一致,由 agents 包一致性测试兜底。
type DefaultModelSetter interface {
	// ApplyDefaultModel 读取-校验-备份-树编辑-原子写,返回写回后的最新
	// Snapshot;dataDir 语义与 ApplyModels 相同。
	ApplyDefaultModel(ctx context.Context, patch DefaultModelPatch, dataDir string) (Snapshot, error)
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Agent{}
)

// Register 注册 Agent 实现;仅限实现包 init 调用,空名或重名视为初始化期编程错误。
func Register(a Agent) {
	if a == nil || a.Name() == "" {
		panic("agentconf: 注册的 Agent 不能为空且必须有名称")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[a.Name()]; dup {
		panic("agentconf: Agent 重复注册: " + a.Name())
	}
	registry[a.Name()] = a
}

// Get 按名称查找已注册 Agent;不存在时 ok=false。
func Get(name string) (Agent, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	a, ok := registry[name]
	return a, ok
}

// List 返回全部已注册 Agent,按名称排序保证输出稳定。
func List() []Agent {
	registryMu.RLock()
	defer registryMu.RUnlock()
	agents := make([]Agent, 0, len(registry))
	for _, a := range registry {
		agents = append(agents, a)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name() < agents[j].Name() })
	return agents
}

// FileExists 判断路径是否存在且是普通文件;供 backup.go 与各实现共用。
func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// WriteJSONFile 以 2 空格缩进、不转义 HTML 的格式写 JSON 文件,保持文件可读。
// 先写同目录临时文件再改名替换:os.WriteFile 会先截断原文件,写回中途失败
// (磁盘满、进程被杀)会把原配置破坏成半截内容;改名在同一目录内完成,
// 原文件要么是旧内容要么是完整新内容,不出现中间态。
func WriteJSONFile(path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
