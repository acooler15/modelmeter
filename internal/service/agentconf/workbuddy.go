// workbuddy.go WorkBuddy 模型配置适配(占位实现)。
// 本机勘探(2026-09-29)在 %APPDATA%、%LOCALAPPDATA%、用户主目录下均未找到
// WorkBuddy 的模型配置文件,按任务 prd 预案以「未找到」占位交付:探测候选
// 路径,全部不存在时给出中文指引,Apply 报 4404。日后确认其配置路径与格式
// 后,在本文件内扩展实现并保持接口不变即可接入。
package agentconf

import (
	"context"
	"os"
	"path/filepath"

	"github.com/acooler15/modelmeter/internal/apperr"
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

// notFoundMessage 未找到配置文件时的中文指引,列表态与 Apply 错误共用。
const notFoundMessage = "未在本机找到 WorkBuddy 配置文件,请确认其已安装并至少运行过一次;找到后可扩展此实现接入"

// candidatePaths 依次探测的候选配置路径,命中第一个存在的即视为配置文件。
// 基于系统约定的用户配置/缓存目录与主目录推导,保持跨平台一致。
func (w *WorkBuddyAgent) candidatePaths() []string {
	var paths []string
	if dir, err := os.UserConfigDir(); err == nil {
		paths = append(paths,
			filepath.Join(dir, "WorkBuddy", "config.json"),
			filepath.Join(dir, "WorkBuddy", "settings.json"),
		)
	}
	if dir, err := os.UserCacheDir(); err == nil {
		paths = append(paths, filepath.Join(dir, "WorkBuddy", "config.json"))
	}
	paths = append(paths, filepath.Join(w.homeDir, "WorkBuddy", "config.json"))
	return paths
}

// locate 返回第一个存在的候选配置路径;均不存在时返回空串。
func (w *WorkBuddyAgent) locate() string {
	for _, path := range w.candidatePaths() {
		if fileExists(path) {
			return path
		}
	}
	return ""
}

// Snapshot 实现 Agent 接口:探测候选路径,未找到时返回占位视图与指引。
func (w *WorkBuddyAgent) Snapshot(_ context.Context) Snapshot {
	path := w.locate()
	if path == "" {
		return Snapshot{
			Name:        w.Name(),
			DisplayName: w.DisplayName(),
			Status:      StatusNotFound,
			Fields:      []FieldSpec{},
			Values:      map[string]string{},
			Message:     notFoundMessage,
		}
	}
	// 占位阶段:找到文件但尚未确认其结构,不做结构化编辑承诺
	return Snapshot{
		Name:        w.Name(),
		DisplayName: w.DisplayName(),
		Status:      StatusFound,
		ConfigPath:  path,
		Fields:      []FieldSpec{},
		Values:      map[string]string{},
		Message:     "已找到 WorkBuddy 配置文件,其格式尚未适配,暂不支持在线编辑",
	}
}

// Apply 实现 Agent 接口:未找到配置文件时报 4404;找到时占位实现也不承诺写回。
func (w *WorkBuddyAgent) Apply(_ context.Context, _ map[string]string, _ string) (Snapshot, error) {
	if w.locate() == "" {
		return Snapshot{}, apperr.New(apperr.CodeAgentNotFound, notFoundMessage)
	}
	return Snapshot{}, apperr.New(apperr.CodeAgentFileIO, "暂不支持在线写回 WorkBuddy 配置,请在其原生工具中修改")
}
