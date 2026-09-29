// Package agents 承载 agentconf.Agent 的各具体实现(ZCode、WorkBuddy 等)。
// 每个实现通过本包 init 自注册到 agentconf 注册表,由 cmd/server/main.go 对
// 本包的空导入触发;agentconf 根包不反向依赖本包,避免循环导入。
//
// 各实现共同遵守安全红线:目标配置文件含明文 API Key,绝不读取、回传、
// 写日志或写回凭据字段;白名单之外的键逐字节原样保留。
package agents

import (
	"os"

	"github.com/acooler15/modelmeter/internal/service/agentconf"
)

// init 注册本机支持的工具实现。homeDir 获取失败置空,实现内部按 not_found
// 降级,不影响服务启动。
func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	agentconf.Register(NewZCodeAgent(home))
	agentconf.Register(NewWorkBuddyAgent(home))
}
