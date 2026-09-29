package agents

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/service/agentconf"
)

// 测试夹具:伪造的 provider_config.json,结构对齐 research.md 的真实勘探结论:
// 含明文 apiKey、templateId、manualProviderModelRules、无 config/api 的残缺
// 供应商与未知键,用于证明读侧脱敏与写侧零丢失。apiKey 为测试虚构值。
const testProviderConfig = `{
  "schemaVersion": 2,
  "config": {
    "providerOrder": ["deepseek", "new-provider-7"],
    "providerConfigRules": {
      "providerRules": [
        {
          "providerId": "deepseek",
          "templateId": "deepseek",
          "providerName": "DeepSeek",
          "enabled": true,
          "config": {
            "access": {"type": "api-key", "apiKey": "sk-fixture-never-leak"},
            "api": {"type": "openai-chat-completions", "baseUrl": "https://api.example.com/v1"},
            "modelOrder": ["deepseek-chat", "deepseek-reasoner"],
            "personalModelIds": ["deepseek-reasoner", "my-custom-model"]
          }
        },
        {"providerId": "new-provider-7", "providerName": "新供应商"}
      ]
    },
    "modelConfigRules": {
      "providerModelRules": [
        {
          "providerId": "deepseek",
          "modelId": "deepseek-chat",
          "config": {
            "enabled": false,
            "properties": {"contextWindow": 128000, "inputFormat": {"supportsImage": true}},
            "optionSpecs": {"maxOutputTokens": {"max": 8192}, "reasoningLevel": {"values": ["low", "high"]}}
          }
        }
      ],
      "manualProviderModelRules": []
    },
    "unknownTopKey": {"keep": true}
  }
}`

// fixtureKey 夹具中的明文 apiKey,所有断言用它验证"不出现/不被改写"。
const fixtureKey = "sk-fixture-never-leak"

// writeZCodeFixture 在临时 home 下伪造 ~/.zcode/v2/provider_config.json。
func writeZCodeFixture(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".zcode", "v2")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("创建伪造配置目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "provider_config.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("写入伪造 provider_config.json 失败: %v", err)
	}
}

// readFixtureTree 重新解析磁盘上的配置文件(UseNumber),供写回断言使用。
func readFixtureTree(t *testing.T, home string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, ".zcode", "v2", "provider_config.json"))
	if err != nil {
		t.Fatalf("读取写回后的文件失败: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("写回后的文件不是合法 JSON: %v", err)
	}
	return doc
}

// findColumn 按列键取列描述,取不到时直接判失败。
func findColumn(t *testing.T, columns []agentconf.FieldSpec, key string) agentconf.FieldSpec {
	t.Helper()
	for _, c := range columns {
		if c.Key == key {
			return c
		}
	}
	t.Fatalf("列 %s 不存在", key)
	return agentconf.FieldSpec{}
}

// entryFieldsOf 按 (providerId, modelId) 取条目字段,取不到时判失败。
func entryFieldsOf(t *testing.T, entries []agentconf.ModelEntry, providerID, modelID string) map[string]any {
	t.Helper()
	for _, e := range entries {
		if e.ProviderID == providerID && e.ModelID == modelID {
			return e.Fields
		}
	}
	t.Fatalf("条目 %s / %s 不存在", providerID, modelID)
	return nil
}

// fixtureProviderRule 取 providerRules 中指定 providerId 的定义节点。
func fixtureProviderRule(t *testing.T, doc map[string]any, providerID string) map[string]any {
	t.Helper()
	for _, e := range anySlice(mapGetObj(doc, "config", "providerConfigRules")["providerRules"]) {
		if m, ok := e.(map[string]any); ok && stringOf(m["providerId"]) == providerID {
			return m
		}
	}
	t.Fatalf("providerRules 中不存在 %s", providerID)
	return nil
}

// fixtureRule 取 providerModelRules 中指定 (providerId, modelId) 的规则节点。
func fixtureRule(t *testing.T, doc map[string]any, providerID, modelID string) map[string]any {
	t.Helper()
	for _, e := range anySlice(mapGetObj(doc, "config", "modelConfigRules")["providerModelRules"]) {
		if m, ok := e.(map[string]any); ok &&
			stringOf(m["providerId"]) == providerID && stringOf(m["modelId"]) == modelID {
			return m
		}
	}
	t.Fatalf("providerModelRules 中不存在 %s / %s", providerID, modelID)
	return nil
}

// isAgentNotFound 判断错误是否为 4404(配置文件不存在)。
func isAgentNotFound(err error) bool {
	var ae *apperr.Error
	return errors.As(err, &ae) && ae.Code == apperr.CodeAgentNotFound
}

// isAgentFileIO 判断错误是否为 4402(文件 IO 或非法 JSON)。
func isAgentFileIO(err error) bool {
	var ae *apperr.Error
	return errors.As(err, &ae) && ae.Code == apperr.CodeAgentFileIO
}

// isAgentInvalid 判断错误是否为 4403(提交值非法,含白名单外键)。
func isAgentInvalid(err error) bool {
	var ae *apperr.Error
	return errors.As(err, &ae) && ae.Code == apperr.CodeAgentInvalid
}

// TestZCode_Models_清单合并与脱敏 验证 modelOrder ∪ personalModelIds 的保序
// 去重合并、规则字段关联与读侧凭据脱敏。
func TestZCode_Models_清单合并与脱敏(t *testing.T) {
	home := t.TempDir()
	writeZCodeFixture(t, home, testProviderConfig)

	entries, err := NewZCodeAgent(home).Models(context.Background())
	if err != nil {
		t.Fatalf("Models 失败: %v", err)
	}
	// 合并顺序:modelOrder 优先,personalModelIds 中新增的排后,重复去重
	if len(entries) != 3 {
		t.Fatalf("应有 3 个模型条目,实际 %d 个:%+v", len(entries), entries)
	}
	wantOrder := []string{"deepseek-chat", "deepseek-reasoner", "my-custom-model"}
	for i, want := range wantOrder {
		if entries[i].ModelID != want {
			t.Errorf("第 %d 个条目应为 %s,实际 %s", i, want, entries[i].ModelID)
		}
		if entries[i].ProviderID != "deepseek" || entries[i].ProviderName != "DeepSeek" {
			t.Errorf("条目 %s 供应商信息错误,实际 %s / %s", want, entries[i].ProviderID, entries[i].ProviderName)
		}
	}
	// 有规则节点的模型:字段来自规则
	chat := entryFieldsOf(t, entries, "deepseek", "deepseek-chat")
	if chat["enabled"] != false {
		t.Errorf("deepseek-chat 的 enabled 应来自规则(false),实际 %v", chat["enabled"])
	}
	if n, ok := chat["context_window"].(json.Number); !ok || n.String() != "128000" {
		t.Errorf("context_window 应为 json.Number(128000),实际 %v(%T)", chat["context_window"], chat["context_window"])
	}
	if n, ok := chat["max_output_tokens"].(json.Number); !ok || n.String() != "8192" {
		t.Errorf("max_output_tokens 应为 json.Number(8192),实际 %v(%T)", chat["max_output_tokens"], chat["max_output_tokens"])
	}
	// 无规则节点的模型:enabled 缺省 true,数字字段不出现
	reasoner := entryFieldsOf(t, entries, "deepseek", "deepseek-reasoner")
	if reasoner["enabled"] != true {
		t.Errorf("无规则节点的模型 enabled 应缺省 true,实际 %v", reasoner["enabled"])
	}
	if _, ok := reasoner["context_window"]; ok {
		t.Errorf("无规则节点的模型不应出现 context_window,实际 %v", reasoner["context_window"])
	}
	// 只读展示字段来自供应商定义
	if chat["api_type"] != "openai-chat-completions" || chat["base_url"] != "https://api.example.com/v1" {
		t.Errorf("api_type/base_url 解析错误,实际 %v / %v", chat["api_type"], chat["base_url"])
	}
	if chat["provider_enabled"] != true {
		t.Errorf("provider_enabled 应为 true,实际 %v", chat["provider_enabled"])
	}
	// 残缺供应商(无 config/api)不产生条目,也不应导致解析失败
	// 读侧脱敏:清单序列化后不得出现夹具中的 apiKey
	entriesJSON, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("清单序列化失败: %v", err)
	}
	if strings.Contains(string(entriesJSON), fixtureKey) {
		t.Error("模型清单不得包含凭据内容")
	}
}

// TestZCode_Snapshot_列描述与脱敏 验证 found 状态、Columns 描述与脱敏。
func TestZCode_Snapshot_列描述与脱敏(t *testing.T) {
	home := t.TempDir()
	writeZCodeFixture(t, home, testProviderConfig)

	snap := NewZCodeAgent(home).Snapshot(context.Background())
	if snap.Status != agentconf.StatusFound || snap.ConfigPath == "" {
		t.Fatalf("应返回 found 且带路径,实际 %+v", snap)
	}
	enabled := findColumn(t, snap.Columns, "enabled")
	if enabled.Type != "bool" || enabled.Readonly {
		t.Errorf("enabled 应为可编辑 bool 列,实际 %+v", enabled)
	}
	if c := findColumn(t, snap.Columns, "context_window"); c.Type != "number" {
		t.Errorf("context_window 应为 number 列,实际 %+v", c)
	}
	if c := findColumn(t, snap.Columns, "provider_name"); !c.Readonly {
		t.Errorf("provider_name 应为只读列,实际 %+v", c)
	}
	snapJSON, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("快照序列化失败: %v", err)
	}
	if strings.Contains(string(snapJSON), fixtureKey) {
		t.Error("快照不得包含凭据内容")
	}
}

// TestZCode_Snapshot_文件缺失_not_found 配置文件缺失时降级为 not_found。
func TestZCode_Snapshot_文件缺失_not_found(t *testing.T) {
	home := t.TempDir()
	snap := NewZCodeAgent(home).Snapshot(context.Background())
	if snap.Status != agentconf.StatusNotFound || snap.Message == "" {
		t.Fatalf("文件缺失应返回 not_found 且带指引,实际 %+v", snap)
	}
}

// TestZCode_ApplyModels_白名单写回零丢失 验证目标键生效,且 apiKey、未知键、
// manualProviderModelRules、规则内兄弟键语义零变化;多 patch 一次备份。
func TestZCode_ApplyModels_白名单写回零丢失(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeZCodeFixture(t, home, testProviderConfig)

	agent := NewZCodeAgent(home)
	patches := []agentconf.ModelPatch{
		{ProviderID: "deepseek", ModelID: "deepseek-chat", Fields: map[string]any{
			"enabled":           true,
			"context_window":    256000,
			"max_output_tokens": 16384,
		}},
		// 无规则节点的模型:应创建最小规则节点
		{ProviderID: "deepseek", ModelID: "my-custom-model", Fields: map[string]any{
			"context_window": 4096,
		}},
	}
	latest, err := agent.ApplyModels(context.Background(), patches, dataDir)
	if err != nil {
		t.Fatalf("ApplyModels 失败: %v", err)
	}
	// 返回的最新清单反映改动
	chat := entryFieldsOf(t, latest, "deepseek", "deepseek-chat")
	if chat["enabled"] != true {
		t.Errorf("写回后 enabled 应为 true,实际 %v", chat["enabled"])
	}
	if n, ok := chat["context_window"].(json.Number); !ok || n.String() != "256000" {
		t.Errorf("写回后 context_window 应为 256000,实际 %v(%T)", chat["context_window"], chat["context_window"])
	}
	if n, ok := chat["max_output_tokens"].(json.Number); !ok || n.String() != "16384" {
		t.Errorf("写回后 max_output_tokens 应为 16384,实际 %v(%T)", chat["max_output_tokens"], chat["max_output_tokens"])
	}
	custom := entryFieldsOf(t, latest, "deepseek", "my-custom-model")
	if n, ok := custom["context_window"].(json.Number); !ok || n.String() != "4096" {
		t.Errorf("新建规则节点的 context_window 应为 4096,实际 %v(%T)", custom["context_window"], custom["context_window"])
	}

	// 重新解析文件,验证白名单之外的节点语义零变化
	doc := readFixtureTree(t, home)
	if doc["schemaVersion"] != json.Number("2") {
		t.Errorf("schemaVersion 应原样保留,实际 %v(%T)", doc["schemaVersion"], doc["schemaVersion"])
	}
	rule0 := fixtureRule(t, doc, "deepseek", "deepseek-chat")
	// 凭据原样:供应商节点 access.apiKey 的值不变
	providerKey := stringOf(mapGetObj(fixtureProviderRule(t, doc, "deepseek"), "config", "access")["apiKey"])
	if providerKey != fixtureKey {
		t.Errorf("apiKey 必须原样保留,实际 %q", providerKey)
	}
	// 规则内兄弟键原样:inputFormat / reasoningLevel 不被触碰
	if b, ok := boolOf(mapGetObj(rule0, "config", "properties", "inputFormat")["supportsImage"]); !ok || !b {
		t.Errorf("properties.inputFormat.supportsImage 应原样保留,实际 %v", mapGetObj(rule0, "config", "properties", "inputFormat")["supportsImage"])
	}
	values := stringSliceOf(mapGetObj(rule0, "config", "optionSpecs", "reasoningLevel")["values"])
	if len(values) != 2 || values[0] != "low" || values[1] != "high" {
		t.Errorf("optionSpecs.reasoningLevel.values 应原样保留,实际 %v", values)
	}
	// 目标键生效
	if b, ok := boolOf(mapGetObj(rule0, "config")["enabled"]); !ok || !b {
		t.Errorf("config.enabled 应已改为 true,实际 %v", mapGetObj(rule0, "config")["enabled"])
	}
	if n := mapGetObj(rule0, "config", "properties")["contextWindow"]; n != json.Number("256000") {
		t.Errorf("properties.contextWindow 应为 256000,实际 %v(%T)", n, n)
	}
	if n := mapGetObj(rule0, "config", "optionSpecs", "maxOutputTokens")["max"]; n != json.Number("16384") {
		t.Errorf("maxOutputTokens.max 应为 16384,实际 %v(%T)", n, n)
	}
	// 新建的最小规则节点:只包含 patch 涉及的键
	newRule := fixtureRule(t, doc, "deepseek", "my-custom-model")
	if newRule == nil {
		t.Fatal("应为 my-custom-model 创建最小规则节点")
	}
	if n := mapGetObj(newRule, "config", "properties")["contextWindow"]; n != json.Number("4096") {
		t.Errorf("新建节点 properties.contextWindow 应为 4096,实际 %v", n)
	}
	if _, exists := mapGetObj(newRule, "config")["enabled"]; exists {
		t.Error("新建节点不应包含 patch 未涉及的 enabled 键")
	}
	// templateId 与 manualProviderModelRules 原样保留
	dsRule := fixtureProviderRule(t, doc, "deepseek")
	if s := stringOf(dsRule["templateId"]); s != "deepseek" {
		t.Errorf("templateId 应原样保留,实际 %v", dsRule["templateId"])
	}
	if manual := anySlice(mapGetObj(doc, "config", "modelConfigRules")["manualProviderModelRules"]); len(manual) != 0 {
		t.Errorf("manualProviderModelRules 应原样保留为空数组,实际 %v", manual)
	}
	// 未知键原样保留(夹具中位于 config 节点内)
	if keep, ok := boolOf(mapGetObj(doc, "config", "unknownTopKey")["keep"]); !ok || !keep {
		t.Errorf("未知键 unknownTopKey 应原样保留,实际 %v", mapGetObj(doc, "config", "unknownTopKey")["keep"])
	}
	// 写回文件保持 2 空格缩进
	raw, err := os.ReadFile(filepath.Join(home, ".zcode", "v2", "provider_config.json"))
	if err != nil {
		t.Fatalf("读取写回后的文件失败: %v", err)
	}
	if !strings.Contains(string(raw), "\n  \"config\"") {
		t.Errorf("写回应为 2 空格缩进,实际:\n%s", raw)
	}
	// 多 patch 一次备份
	entries, err := os.ReadDir(filepath.Join(dataDir, "agent-backups", "zcode"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("多 patch 写回应只生成 1 份备份,实际 err=%v entries=%d", err, len(entries))
	}
}

// TestZCode_ApplyModels_白名单外键_4403 提交白名单外的键报 4403 且不落盘。
func TestZCode_ApplyModels_白名单外键_4403(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeZCodeFixture(t, home, testProviderConfig)

	for name, fields := range map[string]map[string]any{
		"只读展示字段": {"base_url": "https://evil.example.com"},
		"凭据类键":   {"api_key": "sk-attack"},
		"凭据对象":   {"access": map[string]any{"apiKey": "sk-attack"}},
	} {
		patches := []agentconf.ModelPatch{{ProviderID: "deepseek", ModelID: "deepseek-chat", Fields: fields}}
		_, err := NewZCodeAgent(home).ApplyModels(context.Background(), patches, dataDir)
		var ae *apperr.Error
		if !errors.As(err, &ae) || ae.Code != apperr.CodeAgentInvalid {
			t.Errorf("%s: 期望 4403,实际 %v", name, err)
		}
	}
	// 校验失败不得写盘:apiKey 必须与夹具一致
	doc := readFixtureTree(t, home)
	if key := stringOf(mapGetObj(fixtureProviderRule(t, doc, "deepseek"), "config", "access")["apiKey"]); key != fixtureKey {
		t.Errorf("校验失败后文件不得被改写,apiKey 实际 %q", key)
	}
}

// TestZCode_ApplyModels_非法值类型_4403 类型不符的值报 4403。
func TestZCode_ApplyModels_非法值类型_4403(t *testing.T) {
	home := t.TempDir()
	writeZCodeFixture(t, home, testProviderConfig)

	cases := []struct {
		name   string
		fields map[string]any
	}{
		{"enabled 为字符串", map[string]any{"enabled": "yes"}},
		{"context_window 为字符串", map[string]any{"context_window": "128k"}},
		{"max_output_tokens 为布尔", map[string]any{"max_output_tokens": true}},
	}
	for _, tc := range cases {
		patches := []agentconf.ModelPatch{{ProviderID: "deepseek", ModelID: "deepseek-chat", Fields: tc.fields}}
		_, err := NewZCodeAgent(home).ApplyModels(context.Background(), patches, t.TempDir())
		var ae *apperr.Error
		if !errors.As(err, &ae) || ae.Code != apperr.CodeAgentInvalid {
			t.Errorf("%s: 期望 4403,实际 %v", tc.name, err)
		}
	}
}

// TestZCode_ApplyModels_定位不存在_4404 不存在的供应商或模型报 4404。
func TestZCode_ApplyModels_定位不存在_4404(t *testing.T) {
	home := t.TempDir()
	writeZCodeFixture(t, home, testProviderConfig)

	cases := []struct {
		name       string
		providerID string
		modelID    string
	}{
		{"模型不存在", "deepseek", "no-such-model"},
		{"供应商不存在", "no-such-provider", "deepseek-chat"},
		{"空定位", "", ""},
	}
	for _, tc := range cases {
		patches := []agentconf.ModelPatch{{ProviderID: tc.providerID, ModelID: tc.modelID, Fields: map[string]any{"enabled": true}}}
		_, err := NewZCodeAgent(home).ApplyModels(context.Background(), patches, t.TempDir())
		if !isAgentNotFound(err) {
			t.Errorf("%s: 期望 4404,实际 %v", tc.name, err)
		}
	}
}

// TestZCode_Models_文件缺失_4404 配置文件不存在时读与写均报 4404。
func TestZCode_Models_文件缺失_4404(t *testing.T) {
	home := t.TempDir()
	agent := NewZCodeAgent(home)
	if _, err := agent.Models(context.Background()); !isAgentNotFound(err) {
		t.Errorf("Models 期望 4404,实际 %v", err)
	}
	patches := []agentconf.ModelPatch{{ProviderID: "p", ModelID: "m", Fields: map[string]any{"enabled": true}}}
	if _, err := agent.ApplyModels(context.Background(), patches, t.TempDir()); !isAgentNotFound(err) {
		t.Errorf("ApplyModels 期望 4404,实际 %v", err)
	}
}

// TestZCode_ApplyModels_非法JSON_4402 配置文件不是合法 JSON 时报 4402。
func TestZCode_ApplyModels_非法JSON_4402(t *testing.T) {
	home := t.TempDir()
	writeZCodeFixture(t, home, "这不是 JSON")
	patches := []agentconf.ModelPatch{{ProviderID: "p", ModelID: "m", Fields: map[string]any{"enabled": true}}}
	_, err := NewZCodeAgent(home).ApplyModels(context.Background(), patches, t.TempDir())
	if !isAgentFileIO(err) {
		t.Fatalf("期望 4402,实际 %v", err)
	}
	if _, err := NewZCodeAgent(home).Models(context.Background()); !isAgentFileIO(err) {
		t.Errorf("Models 对非法 JSON 期望 4402,实际 %v", err)
	}
}

// TestZCode_ApplyModels_空patch_不落盘 空 patch 列表只返回现状,不备份不写盘。
func TestZCode_ApplyModels_空patch_不落盘(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeZCodeFixture(t, home, testProviderConfig)

	entries, err := NewZCodeAgent(home).ApplyModels(context.Background(), []agentconf.ModelPatch{}, dataDir)
	if err != nil {
		t.Fatalf("空 patch 不应报错: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("空 patch 应返回现有清单,实际 %d 条", len(entries))
	}
	if _, err := os.Stat(filepath.Join(dataDir, "agent-backups", "zcode")); !os.IsNotExist(err) {
		t.Errorf("空 patch 不应产生备份目录,实际 err=%v", err)
	}
}
