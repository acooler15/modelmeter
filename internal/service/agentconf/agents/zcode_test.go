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
// schemaVersion 必须为数字 1(z.literal(1));含明文 apiKey、templateId、
// manualProviderModelRules、无 config/api 的残缺供应商与未知键,用于证明读侧
// 脱敏与写侧零丢失。apiKey 为测试虚构值。
const testProviderConfig = `{
  "schemaVersion": 1,
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

// 默认模型测试夹具:结构对齐 research.md 第 4 节,含已设置的
// defaultModelSelection 与定义了 reasoningLevel.values 的规则节点
// (values 供档位校验与列选项用,map 为兄弟键保留断言用)。
const testProviderConfigWithDefault = `{
  "schemaVersion": 1,
  "config": {
    "providerOrder": ["deepseek"],
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
            "modelOrder": ["deepseek-chat", "deepseek-reasoner"]
          }
        }
      ]
    },
    "modelConfigRules": {
      "providerModelRules": [
        {
          "providerId": "deepseek",
          "modelId": "deepseek-reasoner",
          "config": {
            "optionSpecs": {"reasoningLevel": {"values": ["low", "medium", "high"], "map": "builtin"}}
          }
        }
      ],
      "manualProviderModelRules": []
    },
    "defaultModelSelection": {
      "providerId": "deepseek",
      "modelId": "deepseek-reasoner",
      "options": {"reasoningLevel": "high"}
    }
  }
}`

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
	if doc["schemaVersion"] != json.Number("1") {
		t.Errorf("schemaVersion 应原样保留为 1,实际 %v(%T)", doc["schemaVersion"], doc["schemaVersion"])
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

// TestZCode_Models_三新键读取 验证 supports_image / supports_json_schema_output /
// reasoning_levels 三个白名单键的读侧解析:规则内出现才透出(缺失不出现在
// Fields),规则节点缺失的模型三个键均不出现。
func TestZCode_Models_三新键读取(t *testing.T) {
	home := t.TempDir()
	writeZCodeFixture(t, home, testProviderConfig)

	entries, err := NewZCodeAgent(home).Models(context.Background())
	if err != nil {
		t.Fatalf("Models 失败: %v", err)
	}
	chat := entryFieldsOf(t, entries, "deepseek", "deepseek-chat")
	if chat["supports_image"] != true {
		t.Errorf("deepseek-chat 的 supports_image 应来自规则(true),实际 %v", chat["supports_image"])
	}
	if _, ok := chat["supports_json_schema_output"]; ok {
		t.Errorf("规则未定义时 supports_json_schema_output 不应出现,实际 %v", chat["supports_json_schema_output"])
	}
	if levels, ok := chat["reasoning_levels"].([]string); !ok || len(levels) != 2 || levels[0] != "low" || levels[1] != "high" {
		t.Errorf("reasoning_levels 应保序透出 [low high],实际 %v(%T)", chat["reasoning_levels"], chat["reasoning_levels"])
	}
	// 无规则节点的模型:三个键均不出现
	reasoner := entryFieldsOf(t, entries, "deepseek", "deepseek-reasoner")
	for _, key := range []string{"supports_image", "supports_json_schema_output", "reasoning_levels"} {
		if _, ok := reasoner[key]; ok {
			t.Errorf("无规则节点的模型不应出现 %s,实际 %v", key, reasoner[key])
		}
	}
}

// TestZCode_Snapshot_能力位与默认模型 验证:能力位是工具属性恒为 true(含
// 文件缺失);defaultModelSelection 存在时解析出 provider/model/level,
// 不存在或残缺时 DefaultModel 为 nil 且不报错;reasoning_levels 列选项为
// 全部规则 values 的保序并集。
func TestZCode_Snapshot_能力位与默认模型(t *testing.T) {
	// 未设置默认模型的文件:能力位仍为 true,DefaultModel 为 nil
	home := t.TempDir()
	writeZCodeFixture(t, home, testProviderConfig)
	snap := NewZCodeAgent(home).Snapshot(context.Background())
	if !snap.SupportsDefaultModel {
		t.Error("ZCode 能力位应恒为 true")
	}
	if snap.DefaultModel != nil {
		t.Errorf("未设置默认模型时 DefaultModel 应为 nil,实际 %+v", snap.DefaultModel)
	}
	levelsCol := findColumn(t, snap.Columns, "reasoning_levels")
	if levelsCol.Type != "list" {
		t.Errorf("reasoning_levels 应为 list 列,实际 %+v", levelsCol)
	}
	if len(levelsCol.Options) != 2 || levelsCol.Options[0].Value != "low" || levelsCol.Options[1].Value != "high" {
		t.Errorf("reasoning_levels 选项应为规则 values 的保序并集 [low high],实际 %+v", levelsCol.Options)
	}

	// 已设置默认模型:完整解析含 reasoning_level;三能力列齐备
	home2 := t.TempDir()
	writeZCodeFixture(t, home2, testProviderConfigWithDefault)
	snap2 := NewZCodeAgent(home2).Snapshot(context.Background())
	if !snap2.SupportsDefaultModel {
		t.Error("ZCode 能力位应恒为 true")
	}
	dm := snap2.DefaultModel
	if dm == nil {
		t.Fatal("defaultModelSelection 存在时应解析出 DefaultModel")
	}
	if dm.ProviderID != "deepseek" || dm.ModelID != "deepseek-reasoner" || dm.ReasoningLevel != "high" {
		t.Errorf("默认模型解析错误,实际 %+v", dm)
	}
	if c := findColumn(t, snap2.Columns, "supports_image"); c.Type != "bool" || c.Readonly {
		t.Errorf("supports_image 应为可编辑 bool 列,实际 %+v", c)
	}
	if c := findColumn(t, snap2.Columns, "supports_json_schema_output"); c.Type != "bool" || c.Readonly {
		t.Errorf("supports_json_schema_output 应为可编辑 bool 列,实际 %+v", c)
	}
	if findColumn(t, snap2.Columns, "reasoning_levels").Type != "list" {
		t.Error("reasoning_levels 应为 list 列")
	}
}

// TestZCode_Snapshot_默认模型残缺容忍 defaultModelSelection 缺字段/缺 options
// 时不报错且按未设置处理(缺 providerId/modelId → nil;缺 options → 仅取两主键)。
func TestZCode_Snapshot_默认模型残缺容忍(t *testing.T) {
	cases := []struct {
		name      string
		selection string
		wantNil   bool
		want      agentconf.DefaultModel
	}{
		{"缺 modelId", `"defaultModelSelection": {"providerId": "deepseek"}`, true, agentconf.DefaultModel{}},
		{"缺 providerId", `"defaultModelSelection": {"modelId": "m"}`, true, agentconf.DefaultModel{}},
		{"缺 options", `"defaultModelSelection": {"providerId": "deepseek", "modelId": "deepseek-chat"}`, false, agentconf.DefaultModel{ProviderID: "deepseek", ModelID: "deepseek-chat"}},
	}
	for _, tc := range cases {
		home := t.TempDir()
		content := strings.Replace(testProviderConfig, `"unknownTopKey"`, tc.selection+",\n    \"unknownTopKey\"", 1)
		writeZCodeFixture(t, home, content)
		dm := NewZCodeAgent(home).Snapshot(context.Background()).DefaultModel
		if tc.wantNil {
			if dm != nil {
				t.Errorf("%s: DefaultModel 应为 nil,实际 %+v", tc.name, dm)
			}
			continue
		}
		if dm == nil || *dm != tc.want {
			t.Errorf("%s: DefaultModel 解析错误,实际 %+v", tc.name, dm)
		}
	}
}

// TestZCode_readTree_schemaVersion防护 schemaVersion 存在且非数字 1 时读与写
// 均报 4402;缺失时容忍(残缺文件仍可编辑);Snapshot 不报错、默认模型按未
// 设置处理。
func TestZCode_readTree_schemaVersion防护(t *testing.T) {
	cases := []struct {
		name    string
		version string
	}{
		{"数字 2", `"schemaVersion": 2`},
		{"字符串 1", `"schemaVersion": "1"`},
		{"小数 1.0", `"schemaVersion": 1.0`},
	}
	for _, tc := range cases {
		home := t.TempDir()
		writeZCodeFixture(t, home, "{\n  "+tc.version+",\n  \"config\": {}\n}")
		agent := NewZCodeAgent(home)
		if _, err := agent.Models(context.Background()); !isAgentFileIO(err) {
			t.Errorf("%s: Models 期望 4402,实际 %v", tc.name, err)
		}
		patches := []agentconf.ModelPatch{{ProviderID: "p", ModelID: "m", Fields: map[string]any{"enabled": true}}}
		if _, err := agent.ApplyModels(context.Background(), patches, t.TempDir()); !isAgentFileIO(err) {
			t.Errorf("%s: ApplyModels 期望 4402,实际 %v", tc.name, err)
		}
		// Snapshot 不返回错误:状态仍为 found,默认模型按未设置处理
		snap := agent.Snapshot(context.Background())
		if snap.Status != agentconf.StatusFound || snap.DefaultModel != nil {
			t.Errorf("%s: Snapshot 应容忍并降级,实际 %+v", tc.name, snap)
		}
	}

	// 缺失 schemaVersion:容忍读写
	home := t.TempDir()
	writeZCodeFixture(t, home, "{\n  \"config\": {\n    \"providerOrder\": [],\n    \"providerConfigRules\": {\"providerRules\": []},\n    \"modelConfigRules\": {\"providerModelRules\": [], \"manualProviderModelRules\": []}\n  }\n}")
	agent := NewZCodeAgent(home)
	if _, err := agent.Models(context.Background()); err != nil {
		t.Errorf("缺失 schemaVersion 应容忍,实际 %v", err)
	}
	patches := []agentconf.ModelPatch{{ProviderID: "p", ModelID: "m", Fields: map[string]any{"enabled": true}}}
	if _, err := agent.ApplyModels(context.Background(), patches, t.TempDir()); !isAgentNotFound(err) {
		t.Errorf("缺失 schemaVersion 时写回应容忍解析、报模型不存在 4404,实际 %v", err)
	}
}

// manual 冲突守护测试夹具:目标模型只有 manualProviderModelRules 节点,
// providerModelRules 为空数组——写回必须落在 manual 节点上。
const testProviderConfigManual = `{
  "schemaVersion": 1,
  "config": {
    "providerOrder": ["deepseek"],
    "providerConfigRules": {
      "providerRules": [
        {
          "providerId": "deepseek",
          "providerName": "DeepSeek",
          "enabled": true,
          "config": {
            "access": {"type": "api-key", "apiKey": "sk-fixture-never-leak"},
            "modelOrder": ["deepseek-chat"]
          }
        }
      ]
    },
    "modelConfigRules": {
      "providerModelRules": [],
      "manualProviderModelRules": [
        {
          "providerId": "deepseek",
          "modelId": "deepseek-chat",
          "config": {"enabled": true, "properties": {"contextWindow": 1000}}
        }
      ]
    }
  }
}`

// backupCount 统计指定 dataDir 下某工具的备份份数。
func backupCount(t *testing.T, dataDir, agentName string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dataDir, "agent-backups", agentName))
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("读取备份目录失败: %v", err)
	}
	return len(entries)
}

// TestZCode_ApplyDefaultModel_设置 验证:设置默认模型写回规范节点(带档位时
// 含 options)、apiKey 等兄弟内容原样、产生备份;重复设置同值无变化零落盘;
// 节点内未知键随整体替换清除(红线显式例外)。
func TestZCode_ApplyDefaultModel_设置(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	// 在规范夹具的 defaultModelSelection 中注入未知键,验证整体替换例外
	withBogus := strings.Replace(
		testProviderConfigWithDefault,
		`"options": {"reasoningLevel": "high"}`,
		`"options": {"reasoningLevel": "high", "bogus": true}, "extra": 1`,
		1,
	)
	writeZCodeFixture(t, home, withBogus)

	agent := NewZCodeAgent(home)
	patch := agentconf.DefaultModelPatch{ProviderID: "deepseek", ModelID: "deepseek-chat", ReasoningLevel: "low"}
	snap, err := agent.ApplyDefaultModel(context.Background(), patch, dataDir)
	if err != nil {
		t.Fatalf("ApplyDefaultModel 失败: %v", err)
	}
	// 返回的最新 Snapshot 反映设置结果
	if snap.DefaultModel == nil || snap.DefaultModel.ProviderID != "deepseek" ||
		snap.DefaultModel.ModelID != "deepseek-chat" || snap.DefaultModel.ReasoningLevel != "low" {
		t.Fatalf("写回后 Snapshot 默认模型应为 deepseek/deepseek-chat/low,实际 %+v", snap.DefaultModel)
	}

	// 重新解析文件:节点为规范形状,未知键随整体替换清除
	doc := readFixtureTree(t, home)
	sel := mapGetObj(doc, "config", "defaultModelSelection")
	if sel == nil {
		t.Fatal("defaultModelSelection 节点应存在")
	}
	if stringOf(sel["providerId"]) != "deepseek" || stringOf(sel["modelId"]) != "deepseek-chat" {
		t.Errorf("defaultModelSelection 主键错误,实际 %v / %v", sel["providerId"], sel["modelId"])
	}
	if stringOf(mapGetObj(sel, "options")["reasoningLevel"]) != "low" {
		t.Errorf("options.reasoningLevel 应为 low,实际 %v", mapGetObj(sel, "options")["reasoningLevel"])
	}
	if _, exists := sel["extra"]; exists {
		t.Error("defaultModelSelection 节点内未知键应随整体替换清除")
	}
	if _, exists := mapGetObj(sel, "options")["bogus"]; exists {
		t.Error("options 子节点内未知键应随整体替换清除")
	}
	// 兄弟内容原样:apiKey、templateId、模型规则与 manual 数组不受影响
	if key := stringOf(mapGetObj(fixtureProviderRule(t, doc, "deepseek"), "config", "access")["apiKey"]); key != fixtureKey {
		t.Errorf("apiKey 必须原样保留,实际 %q", key)
	}
	if s := stringOf(fixtureProviderRule(t, doc, "deepseek")["templateId"]); s != "deepseek" {
		t.Errorf("templateId 必须原样保留,实际 %q", s)
	}
	if got := len(anySlice(mapGetObj(doc, "config", "modelConfigRules")["providerModelRules"])); got != 1 {
		t.Errorf("providerModelRules 不得被触碰,实际 %d 个", got)
	}
	if got := len(anySlice(mapGetObj(doc, "config", "modelConfigRules")["manualProviderModelRules"])); got != 0 {
		t.Errorf("manualProviderModelRules 不得被触碰,实际 %d 个", got)
	}
	// 写回产生备份
	if got := backupCount(t, dataDir, "zcode"); got != 1 {
		t.Fatalf("首次设置应产生 1 份备份,实际 %d", got)
	}

	// 重复设置同值:无实际变化,零落盘(不产生第二次备份)
	if _, err := agent.ApplyDefaultModel(context.Background(), patch, dataDir); err != nil {
		t.Fatalf("重复设置同值不应报错: %v", err)
	}
	if got := backupCount(t, dataDir, "zcode"); got != 1 {
		t.Errorf("重复设置同值不应产生第二次备份,实际 %d 份", got)
	}
}

// TestZCode_ApplyDefaultModel_清除 验证:清除删除 config.defaultModelSelection
// 节点且其余内容原样;本无节点时不备份不落盘。
func TestZCode_ApplyDefaultModel_清除(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeZCodeFixture(t, home, testProviderConfigWithDefault)

	snap, err := NewZCodeAgent(home).ApplyDefaultModel(context.Background(), agentconf.DefaultModelPatch{}, dataDir)
	if err != nil {
		t.Fatalf("ApplyDefaultModel 清除失败: %v", err)
	}
	if snap.DefaultModel != nil {
		t.Errorf("清除后 Snapshot 默认模型应为 nil,实际 %+v", snap.DefaultModel)
	}
	doc := readFixtureTree(t, home)
	if _, exists := mapGetObj(doc, "config")["defaultModelSelection"]; exists {
		t.Error("defaultModelSelection 节点应被删除")
	}
	// 其余内容原样:apiKey、templateId、schemaVersion
	if key := stringOf(mapGetObj(fixtureProviderRule(t, doc, "deepseek"), "config", "access")["apiKey"]); key != fixtureKey {
		t.Errorf("apiKey 必须原样保留,实际 %q", key)
	}
	if s := stringOf(fixtureProviderRule(t, doc, "deepseek")["templateId"]); s != "deepseek" {
		t.Errorf("templateId 必须原样保留,实际 %q", s)
	}
	if doc["schemaVersion"] != json.Number("1") {
		t.Errorf("schemaVersion 应原样保留,实际 %v", doc["schemaVersion"])
	}
	if got := backupCount(t, dataDir, "zcode"); got != 1 {
		t.Fatalf("清除已有节点应产生 1 份备份,实际 %d", got)
	}

	// 本无节点:不备份不落盘直接返回
	home2 := t.TempDir()
	dataDir2 := t.TempDir()
	writeZCodeFixture(t, home2, testProviderConfig)
	snap2, err := NewZCodeAgent(home2).ApplyDefaultModel(context.Background(), agentconf.DefaultModelPatch{}, dataDir2)
	if err != nil {
		t.Fatalf("清除不存在的节点不应报错: %v", err)
	}
	if snap2.DefaultModel != nil {
		t.Errorf("未设置时默认模型应为 nil,实际 %+v", snap2.DefaultModel)
	}
	if got := backupCount(t, dataDir2, "zcode"); got != 0 {
		t.Errorf("本无节点时不应产生备份,实际 %d 份", got)
	}
}

// TestZCode_ApplyDefaultModel_错误分支 验证:混合参数 4403、模型不存在 4404、
// 档位越界 4403、未定义 values 放行、schemaVersion 不受支持 4402。
func TestZCode_ApplyDefaultModel_错误分支(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeZCodeFixture(t, home, testProviderConfigWithDefault)
	agent := NewZCodeAgent(home)

	// 混合参数:一空一非空
	_, err := agent.ApplyDefaultModel(context.Background(), agentconf.DefaultModelPatch{ProviderID: "deepseek"}, dataDir)
	if !isAgentInvalid(err) {
		t.Errorf("混合参数期望 4403,实际 %v", err)
	}
	_, err = agent.ApplyDefaultModel(context.Background(), agentconf.DefaultModelPatch{ModelID: "deepseek-chat"}, dataDir)
	if !isAgentInvalid(err) {
		t.Errorf("混合参数期望 4403,实际 %v", err)
	}
	// 模型不存在
	_, err = agent.ApplyDefaultModel(context.Background(),
		agentconf.DefaultModelPatch{ProviderID: "deepseek", ModelID: "no-such-model"}, dataDir)
	if !isAgentNotFound(err) {
		t.Errorf("模型不存在期望 4404,实际 %v", err)
	}
	// 档位越界:目标规则定义了 values 且不含该档位
	_, err = agent.ApplyDefaultModel(context.Background(),
		agentconf.DefaultModelPatch{ProviderID: "deepseek", ModelID: "deepseek-reasoner", ReasoningLevel: "ultra"}, dataDir)
	if !isAgentInvalid(err) {
		t.Errorf("档位越界期望 4403,实际 %v", err)
	}
	// 校验失败不得写盘
	if _, exists := mapGetObj(readFixtureTree(t, home), "config")["defaultModelSelection"]; !exists {
		t.Error("校验失败后文件不得被改写")
	}
	// 未定义 values 的模型:任意档位放行
	snap, err := agent.ApplyDefaultModel(context.Background(),
		agentconf.DefaultModelPatch{ProviderID: "deepseek", ModelID: "deepseek-chat", ReasoningLevel: "free-form"}, dataDir)
	if err != nil {
		t.Fatalf("未定义 values 的模型档位应放行,实际 %v", err)
	}
	if snap.DefaultModel == nil || snap.DefaultModel.ReasoningLevel != "free-form" {
		t.Errorf("设置结果错误,实际 %+v", snap.DefaultModel)
	}

	// schemaVersion 不受支持:读与写均 4402
	home2 := t.TempDir()
	writeZCodeFixture(t, home2, strings.Replace(testProviderConfigWithDefault, `"schemaVersion": 1`, `"schemaVersion": 2`, 1))
	_, err = NewZCodeAgent(home2).ApplyDefaultModel(context.Background(), agentconf.DefaultModelPatch{}, t.TempDir())
	if !isAgentFileIO(err) {
		t.Errorf("schemaVersion=2 清除期望 4402,实际 %v", err)
	}
	_, err = NewZCodeAgent(home2).ApplyDefaultModel(context.Background(),
		agentconf.DefaultModelPatch{ProviderID: "deepseek", ModelID: "deepseek-chat"}, t.TempDir())
	if !isAgentFileIO(err) {
		t.Errorf("schemaVersion=2 设置期望 4402,实际 %v", err)
	}
}

// TestZCode_能力一致性 ZCode 编译期实现 DefaultModelSetter 且能力位恒 true。
func TestZCode_能力一致性(t *testing.T) {
	var _ agentconf.DefaultModelSetter = (*ZCodeAgent)(nil)

	home := t.TempDir()
	writeZCodeFixture(t, home, testProviderConfig)
	if !NewZCodeAgent(home).Snapshot(context.Background()).SupportsDefaultModel {
		t.Error("ZCode 能力位应恒为 true")
	}
}

// TestZCode_ApplyModels_三新键写回 验证 supports_image / supports_json_schema_output /
// reasoning_levels 的写回:既有节点只动目标路径(兄弟键保留),缺失节点创建。
func TestZCode_ApplyModels_三新键写回(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeZCodeFixture(t, home, testProviderConfig)

	patches := []agentconf.ModelPatch{
		// 既有节点:改目标键,兄弟键(contextWindow/maxOutputTokens/inputFormat)保留
		{ProviderID: "deepseek", ModelID: "deepseek-chat", Fields: map[string]any{
			"supports_image":              false,
			"supports_json_schema_output": true,
			"reasoning_levels":            []any{"xhigh", "max"},
		}},
		// 无规则节点:创建最小节点
		{ProviderID: "deepseek", ModelID: "deepseek-reasoner", Fields: map[string]any{
			"reasoning_levels": []any{"low"},
		}},
	}
	latest, err := NewZCodeAgent(home).ApplyModels(context.Background(), patches, dataDir)
	if err != nil {
		t.Fatalf("ApplyModels 失败: %v", err)
	}
	chat := entryFieldsOf(t, latest, "deepseek", "deepseek-chat")
	if chat["supports_image"] != false || chat["supports_json_schema_output"] != true {
		t.Errorf("三新键写回未生效,实际 %v / %v", chat["supports_image"], chat["supports_json_schema_output"])
	}
	if levels, ok := chat["reasoning_levels"].([]string); !ok || len(levels) != 2 || levels[0] != "xhigh" || levels[1] != "max" {
		t.Errorf("reasoning_levels 写回错误,实际 %v", chat["reasoning_levels"])
	}

	doc := readFixtureTree(t, home)
	rule := fixtureRule(t, doc, "deepseek", "deepseek-chat")
	// 兄弟键保留:contextWindow / maxOutputTokens / inputFormat 位置不动
	if n := mapGetObj(rule, "config", "properties")["contextWindow"]; n != json.Number("128000") {
		t.Errorf("contextWindow 应原样保留,实际 %v", n)
	}
	if n := mapGetObj(rule, "config", "optionSpecs", "maxOutputTokens")["max"]; n != json.Number("8192") {
		t.Errorf("maxOutputTokens.max 应原样保留,实际 %v", n)
	}
	// 新建最小节点:只包含 reasoningLevel.values
	newRule := fixtureRule(t, doc, "deepseek", "deepseek-reasoner")
	values := stringSliceOf(mapGetObj(newRule, "config", "optionSpecs", "reasoningLevel")["values"])
	if len(values) != 1 || values[0] != "low" {
		t.Errorf("新建节点 values 应为 [low],实际 %v", values)
	}
	if _, exists := mapGetObj(newRule, "config", "optionSpecs", "reasoningLevel")["map"]; exists {
		t.Error("新建节点不应包含未提交的 reasoningLevel.map 键")
	}
}

// TestZCode_ApplyModels_reasoningLevel兄弟键保留 在带 map 的规则上替换 values
// 时兄弟键 map 原样保留。
func TestZCode_ApplyModels_reasoningLevel兄弟键保留(t *testing.T) {
	home := t.TempDir()
	writeZCodeFixture(t, home, testProviderConfigWithDefault)

	patches := []agentconf.ModelPatch{{ProviderID: "deepseek", ModelID: "deepseek-reasoner", Fields: map[string]any{
		"reasoning_levels": []any{"low", "high"},
	}}}
	if _, err := NewZCodeAgent(home).ApplyModels(context.Background(), patches, t.TempDir()); err != nil {
		t.Fatalf("ApplyModels 失败: %v", err)
	}
	rule := fixtureRule(t, readFixtureTree(t, home), "deepseek", "deepseek-reasoner")
	levelSpec := mapGetObj(rule, "config", "optionSpecs", "reasoningLevel")
	if values := stringSliceOf(levelSpec["values"]); len(values) != 2 || values[0] != "low" || values[1] != "high" {
		t.Errorf("values 应替换为 [low high],实际 %v", values)
	}
	if stringOf(levelSpec["map"]) != "builtin" {
		t.Errorf("兄弟键 map 应原样保留,实际 %v", levelSpec["map"])
	}
}

// TestZCode_ApplyModels_manual冲突守护 同 (providerId, modelId) 已存在于
// manualProviderModelRules 时,patch 落在 manual 节点而非重复追加。
func TestZCode_ApplyModels_manual冲突守护(t *testing.T) {
	home := t.TempDir()
	writeZCodeFixture(t, home, testProviderConfigManual)

	patches := []agentconf.ModelPatch{{ProviderID: "deepseek", ModelID: "deepseek-chat", Fields: map[string]any{
		"enabled": false,
	}}}
	if _, err := NewZCodeAgent(home).ApplyModels(context.Background(), patches, t.TempDir()); err != nil {
		t.Fatalf("ApplyModels 失败: %v", err)
	}
	doc := readFixtureTree(t, home)
	mcr := mapGetObj(doc, "config", "modelConfigRules")
	if got := len(anySlice(mcr["providerModelRules"])); got != 0 {
		t.Errorf("providerModelRules 不得追加新节点,实际 %d 个", got)
	}
	manual := anySlice(mcr["manualProviderModelRules"])
	if len(manual) != 1 {
		t.Fatalf("manualProviderModelRules 不得增删条目,实际 %d 个", len(manual))
	}
	node, _ := manual[0].(map[string]any)
	if b, ok := boolOf(mapGetObj(node, "config")["enabled"]); !ok || b {
		t.Errorf("manual 节点 enabled 应改为 false,实际 %v", mapGetObj(node, "config")["enabled"])
	}
	if n := mapGetObj(node, "config", "properties")["contextWindow"]; n != json.Number("1000") {
		t.Errorf("manual 节点兄弟键 contextWindow 应原样保留,实际 %v", n)
	}
}
