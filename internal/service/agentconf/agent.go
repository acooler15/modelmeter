// Package agentconf 提供本机 Agent 工具(ZCode、WorkBuddy 等)模型配置的
// 读取、写回与备份还原能力。扩展点为 Agent 接口 + 注册表:新工具实现接口
// 并在包内 init 注册即可接入,handler 与前端无需为单个工具改动。
//
// 安全红线(所有实现共同遵守):绝不读取、回传或写回凭据字段(如 API Key);
// 日志不得出现任何凭据内容;只对明确的配置文件做最小化修改。
package agentconf

import (
	"context"
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

// FieldSpec 字段描述,驱动前端动态表单。
type FieldSpec struct {
	Key      string        `json:"key"`
	Label    string        `json:"label"`
	Type     string        `json:"type"`              // "text" | "select"
	Options  []FieldOption `json:"options,omitempty"` // 仅 select 使用
	Required bool          `json:"required"`
	Help     string        `json:"help,omitempty"` // 字段说明,前端展示
}

// Snapshot Agent 配置现状视图(不回传任何凭据)。
type Snapshot struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	Status      string            `json:"status"` // StatusFound | StatusNotFound
	ConfigPath  string            `json:"config_path"`
	Fields      []FieldSpec       `json:"fields"`
	Values      map[string]string `json:"values"`
	Message     string            `json:"message,omitempty"` // 指引或管理边界说明
}

// Agent 扩展点:本机 Agent 工具的模型配置适配器。
// 新工具实现本接口并在包内 Register 即可接入,调用方无需修改。
type Agent interface {
	// Name 稳定标识:用于 URL 路径与备份目录名,小写英文。
	Name() string
	// DisplayName 界面展示名。
	DisplayName() string
	// Snapshot 返回配置现状;实现需自行降级(如文件缺失),不返回错误。
	Snapshot(ctx context.Context) Snapshot
	// Apply 校验并写回修改,内部先备份原文件;dataDir 为运行数据目录,
	// 备份落在 dataDir/agent-backups/<Name>/ 下。返回写回后的最新视图。
	Apply(ctx context.Context, values map[string]string, dataDir string) (Snapshot, error)
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Agent{}
)

// Register 注册 Agent 实现;仅限包内 init 调用,空名或重名视为初始化期编程错误。
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

// fileExists 判断路径是否存在且是普通文件。
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// 默认注册本机支持的工具。homeDir 由构造函数注入到各实现,拿不到时留空,
// 由实现内部按 not_found 降级,不影响服务启动。
func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	Register(NewZCodeAgent(home))
	Register(NewWorkBuddyAgent(home))
}
