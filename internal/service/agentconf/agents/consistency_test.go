package agents

import (
	"context"
	"testing"

	"github.com/acooler15/modelmeter/internal/service/agentconf"
)

// consistencyAgent 供一致性测试用的工具定义:构造器 + 名称。
type consistencyAgent struct {
	name   string
	build  func(home string) agentconf.Agent
	setter bool // 是否实现 DefaultModelSetter
}

// TestAgents_增删能力一致性 增删模型能力是 Agent 接口的必选方法(三个实现
// 均编译期满足),Snapshot 能力位必须与实现一致:SupportsAddModels /
// SupportsRemoveModels 对三个工具恒置 true,配置文件缺失(not_found)时同样
// 成立;AddTargets 仅 ZCode 且 found 时携带(参照 SupportsDefaultModel 的
// 一致性测试模式)。
func TestAgents_增删能力一致性(t *testing.T) {
	tools := []consistencyAgent{
		{name: "zcode", build: func(home string) agentconf.Agent { return NewZCodeAgent(home) }, setter: true},
		{name: "workbuddy", build: func(home string) agentconf.Agent { return NewWorkBuddyAgent(home) }},
		{name: "codebuddy", build: func(home string) agentconf.Agent { return NewCodeBuddyAgent(home) }},
	}
	for _, tool := range tools {
		// 编译期断言:三个实现都必须满足 Agent 接口(含 RemoveModels/AddModels)
		var _ agentconf.Agent = tool.build(t.TempDir())

		// 文件缺失(not_found):能力位仍为 true,AddTargets 不携带
		snap := tool.build(t.TempDir()).Snapshot(context.Background())
		if snap.Status != agentconf.StatusNotFound {
			t.Fatalf("%s: 临时目录应无配置文件,实际状态 %s", tool.name, snap.Status)
		}
		if !snap.SupportsAddModels || !snap.SupportsRemoveModels {
			t.Errorf("%s: not_found 时增删能力位同样应为 true,实际 %+v", tool.name, snap)
		}
		if snap.AddTargets != nil {
			t.Errorf("%s: not_found 时不应携带 AddTargets,实际 %+v", tool.name, snap.AddTargets)
		}
		// 能力位与 DefaultModelSetter 实现的既有一致性不回归
		if snap.SupportsDefaultModel != tool.setter {
			t.Errorf("%s: SupportsDefaultModel 应为 %v,实际 %v", tool.name, tool.setter, snap.SupportsDefaultModel)
		}
	}
}

// TestAgents_仅ZCode携带AddTargets 挂靠候选清单是 ZCode 专属:found 时仅
// ZCode 的 Snapshot 携带 AddTargets,WorkBuddy/CodeBuddy 恒为空。
func TestAgents_仅ZCode携带AddTargets(t *testing.T) {
	zHome := t.TempDir()
	writeZCodeFixture(t, zHome, testProviderConfig)
	wbHome := t.TempDir()
	writeWorkBuddyFixture(t, wbHome, testModelsJSON)
	cbHome := t.TempDir()
	writeCodeBuddyFixture(t, cbHome, testCodeBuddyJSON)

	zSnap := NewZCodeAgent(zHome).Snapshot(context.Background())
	if len(zSnap.AddTargets) == 0 {
		t.Error("found 时 ZCode 应携带 AddTargets")
	}
	for _, a := range []agentconf.Agent{
		NewWorkBuddyAgent(wbHome),
		NewCodeBuddyAgent(cbHome),
	} {
		if snap := a.Snapshot(context.Background()); snap.AddTargets != nil {
			t.Errorf("%s 不应携带 AddTargets,实际 %+v", a.Name(), snap.AddTargets)
		}
	}
}
