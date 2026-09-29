package agents

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/acooler15/modelmeter/internal/service/agentconf"
)

// 测试夹具:伪造的 models.json(裸数组),结构对齐任务 research.md 的真实勘探
// 结论:每条记录携带独立明文 apiKey;首条含未知键、完整 reasoning 节点与全部
// 新增字段(disabled/onlyReasoning/useCustomProtocol/数字项/
// reasoning.canDisableThinking);次条缺 reasoning 节点。apiKey 均为测试虚构值。
const testModelsJSON = `[
  {
    "id": "deepseek-v4-flash",
    "name": "DeepSeek V4 Flash",
    "vendor": "deepseek",
    "url": "https://api.example.com/chat/completions",
    "apiKey": "sk-wb-fixture-never-leak",
    "maxInputTokens": 128000,
    "maxOutputTokens": 8192,
    "temperature": 0.7,
    "supportsToolCall": true,
    "supportsImages": false,
    "supportsReasoning": true,
    "onlyReasoning": false,
    "useCustomProtocol": true,
    "disabled": true,
    "reasoning": {"defaultEffort": "high", "supportedEfforts": ["high", "max"], "canDisableThinking": false},
    "unknownKey": {"note": "keep"}
  },
  {
    "id": "second-model",
    "name": "第二个模型",
    "vendor": "openai",
    "url": "https://x.example.com/v1/chat/completions",
    "apiKey": "sk-wb-second",
    "supportsToolCall": false
  }
]`

// wbFixtureKey 首条记录的明文 apiKey,用于验证"不出现/不被改写"。
const wbFixtureKey = "sk-wb-fixture-never-leak"

// writeWorkBuddyFixture 在临时 home 下伪造 ~/.workbuddy/models.json。
func writeWorkBuddyFixture(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".workbuddy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("创建伪造配置目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "models.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("写入伪造 models.json 失败: %v", err)
	}
}

// readFixtureDoc 重新解析磁盘上的 models.json(UseNumber)为根值,供写回断言使用。
func readFixtureDoc(t *testing.T, home string) any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, ".workbuddy", "models.json"))
	if err != nil {
		t.Fatalf("读取写回后的文件失败: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		t.Fatalf("写回后的文件不是合法 JSON: %v", err)
	}
	return root
}

// readFixtureArray 取裸数组形态的写回结果。
func readFixtureArray(t *testing.T, home string) []any {
	t.Helper()
	arr, ok := readFixtureDoc(t, home).([]any)
	if !ok {
		t.Fatalf("写回后的文件顶层不是数组")
	}
	return arr
}

// fixtureElement 取数组中第 i 个元素(必须是对象),否则判失败。
func fixtureElement(t *testing.T, arr []any, i int) map[string]any {
	t.Helper()
	if i < 0 || i >= len(arr) {
		t.Fatalf("数组长度不足,期望第 %d 个元素,实际 %d 个", i, len(arr))
	}
	m, ok := arr[i].(map[string]any)
	if !ok {
		t.Fatalf("第 %d 个元素不是对象", i)
	}
	return m
}

// entryOf 按 modelId 取条目字段,取不到时判失败。
func entryOf(t *testing.T, entries []agentconf.ModelEntry, modelID string) map[string]any {
	t.Helper()
	for _, e := range entries {
		if e.ModelID == modelID {
			return e.Fields
		}
	}
	t.Fatalf("条目 %s 不存在", modelID)
	return nil
}

// jsonNumberOf 断言字段为 json.Number 并返回其字面量,供数字键断言使用。
func jsonNumberOf(t *testing.T, fields map[string]any, key string) string {
	t.Helper()
	n, ok := fields[key].(json.Number)
	if !ok {
		t.Fatalf("字段 %s 应为 json.Number,实际 %v(%T)", key, fields[key], fields[key])
	}
	return n.String()
}

// TestWorkBuddy_Models_清单与脱敏 验证字段映射(含新增字段)、缺省语义与读侧脱敏。
func TestWorkBuddy_Models_清单与脱敏(t *testing.T) {
	home := t.TempDir()
	writeWorkBuddyFixture(t, home, testModelsJSON)

	entries, err := NewWorkBuddyAgent(home).Models(context.Background())
	if err != nil {
		t.Fatalf("Models 失败: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("应有 2 个条目,实际 %d 个", len(entries))
	}
	first := entryOf(t, entries, "deepseek-v4-flash")
	if entries[0].DisplayName != "DeepSeek V4 Flash" {
		t.Errorf("display_name 应为显示名,实际 %q", entries[0].DisplayName)
	}
	if first["name"] != "DeepSeek V4 Flash" || first["vendor"] != "deepseek" {
		t.Errorf("name/vendor 解析错误,实际 %v / %v", first["name"], first["vendor"])
	}
	if first["url"] != "https://api.example.com/chat/completions" {
		t.Errorf("url 应为完整 endpoint,实际 %v", first["url"])
	}
	if first["supports_tool_call"] != true || first["supports_images"] != false || first["supports_reasoning"] != true {
		t.Errorf("能力开关解析错误,实际 %v / %v / %v", first["supports_tool_call"], first["supports_images"], first["supports_reasoning"])
	}
	if first["disabled"] != true || first["only_reasoning"] != false || first["use_custom_protocol"] != true {
		t.Errorf("新增布尔键解析错误,实际 disabled=%v only_reasoning=%v use_custom_protocol=%v",
			first["disabled"], first["only_reasoning"], first["use_custom_protocol"])
	}
	if n := jsonNumberOf(t, first, "max_input_tokens"); n != "128000" {
		t.Errorf("max_input_tokens 应为 128000,实际 %s", n)
	}
	if n := jsonNumberOf(t, first, "max_output_tokens"); n != "8192" {
		t.Errorf("max_output_tokens 应为 8192,实际 %s", n)
	}
	if n := jsonNumberOf(t, first, "temperature"); n != "0.7" {
		t.Errorf("temperature 应为 0.7,实际 %s", n)
	}
	if first["default_effort"] != "high" {
		t.Errorf("default_effort 应为 high,实际 %v", first["default_effort"])
	}
	if efforts, ok := first["supported_efforts"].([]string); !ok || len(efforts) != 2 || efforts[0] != "high" || efforts[1] != "max" {
		t.Errorf("supported_efforts 解析错误,实际 %v(%T)", first["supported_efforts"], first["supported_efforts"])
	}
	if first["can_disable_thinking"] != false {
		t.Errorf("can_disable_thinking 应为文件值 false,实际 %v", first["can_disable_thinking"])
	}
	// 次条缺 reasoning 节点:相应键取缺省值;canDisableThinking 文件缺省即 true
	second := entryOf(t, entries, "second-model")
	if second["default_effort"] != "" {
		t.Errorf("缺 reasoning 时 default_effort 应为空串,实际 %v", second["default_effort"])
	}
	if efforts, ok := second["supported_efforts"].([]string); !ok || len(efforts) != 0 {
		t.Errorf("缺 reasoning 时 supported_efforts 应为空数组,实际 %v(%T)", second["supported_efforts"], second["supported_efforts"])
	}
	if second["can_disable_thinking"] != true {
		t.Errorf("缺 reasoning 时 can_disable_thinking 应缺省 true,实际 %v", second["can_disable_thinking"])
	}
	if second["max_input_tokens"] != nil || second["temperature"] != nil {
		t.Errorf("文件未写的数字键不应出现,实际 max_input_tokens=%v temperature=%v",
			second["max_input_tokens"], second["temperature"])
	}
	// 读侧脱敏:清单序列化后不得出现任何夹具 apiKey
	entriesJSON, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("清单序列化失败: %v", err)
	}
	if strings.Contains(string(entriesJSON), wbFixtureKey) || strings.Contains(string(entriesJSON), "sk-wb-second") {
		t.Error("模型清单不得包含凭据内容")
	}
}

// TestWorkBuddy_Models_effort兜底 default_effort 读取兜底 legacy 键 effort。
func TestWorkBuddy_Models_effort兜底(t *testing.T) {
	home := t.TempDir()
	writeWorkBuddyFixture(t, home, `[
		{"id": "legacy", "name": "旧键模型", "reasoning": {"effort": "medium"}}
	]`)

	entries, err := NewWorkBuddyAgent(home).Models(context.Background())
	if err != nil {
		t.Fatalf("Models 失败: %v", err)
	}
	if got := entryOf(t, entries, "legacy")["default_effort"]; got != "medium" {
		t.Errorf("default_effort 应兜底读取 reasoning.effort=medium,实际 %v", got)
	}
}

// TestWorkBuddy_Snapshot_列描述 验证 found 状态、Columns 描述与下拉选项。
func TestWorkBuddy_Snapshot_列描述(t *testing.T) {
	home := t.TempDir()
	writeWorkBuddyFixture(t, home, testModelsJSON)

	snap := NewWorkBuddyAgent(home).Snapshot(context.Background())
	if snap.Status != agentconf.StatusFound || snap.ConfigPath == "" {
		t.Fatalf("应返回 found 且带路径,实际 %+v", snap)
	}
	if c := findColumn(t, snap.Columns, "name"); c.Type != "text" || c.Readonly {
		t.Errorf("name 应为可编辑 text 列,实际 %+v", c)
	}
	if c := findColumn(t, snap.Columns, "vendor"); !c.Readonly {
		t.Errorf("vendor 应为只读列,实际 %+v", c)
	}
	effort := findColumn(t, snap.Columns, "default_effort")
	if effort.Type != "select" || len(effort.Options) != 2 || effort.Options[0].Value != "high" || effort.Options[1].Value != "max" {
		t.Errorf("default_effort 应为含 2 个选项的下拉,实际 %+v", effort)
	}
	// supported_efforts 列:白名单本就支持,列描述使其可见可编辑
	if c := findColumn(t, snap.Columns, "supported_efforts"); c.Type != "list" || c.Readonly {
		t.Errorf("supported_efforts 应为可编辑 list 列,实际 %+v", c)
	}
	// 新增列:布尔与数字类型,均可编辑
	for key, wantType := range map[string]string{
		"disabled":             "bool",
		"only_reasoning":       "bool",
		"use_custom_protocol":  "bool",
		"can_disable_thinking": "bool",
		"max_input_tokens":     "number",
		"max_output_tokens":    "number",
		"temperature":          "number",
	} {
		if c := findColumn(t, snap.Columns, key); c.Type != wantType || c.Readonly {
			t.Errorf("列 %s 应为可编辑 %s 列,实际 %+v", key, wantType, c)
		}
	}
	// 快照不得包含凭据内容
	snapJSON, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("快照序列化失败: %v", err)
	}
	if strings.Contains(string(snapJSON), wbFixtureKey) {
		t.Error("快照不得包含凭据内容")
	}
}

// TestWorkBuddy_Snapshot_文件缺失_not_found 配置文件缺失时降级为 not_found。
func TestWorkBuddy_Snapshot_文件缺失_not_found(t *testing.T) {
	home := t.TempDir()
	snap := NewWorkBuddyAgent(home).Snapshot(context.Background())
	if snap.Status != agentconf.StatusNotFound || snap.Message == "" {
		t.Fatalf("文件缺失应返回 not_found 且带指引,实际 %+v", snap)
	}
}

// TestWorkBuddy_ApplyModels_白名单写回零丢失 验证目标键生效,且 apiKey、
// 未知键、id、vendor、未提交的新字段及未命中的条目逐条原样保留。
func TestWorkBuddy_ApplyModels_白名单写回零丢失(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeWorkBuddyFixture(t, home, testModelsJSON)

	patches := []agentconf.ModelPatch{{
		ModelID: "deepseek-v4-flash",
		Fields: map[string]any{
			"name":              "新名字",
			"url":               "https://new.example.com/chat/completions",
			"supports_images":   true,
			"default_effort":    "max",
			"supported_efforts": []any{"low", "high", "max"},
		},
	}}
	latest, err := NewWorkBuddyAgent(home).ApplyModels(context.Background(), patches, dataDir)
	if err != nil {
		t.Fatalf("ApplyModels 失败: %v", err)
	}
	// 返回的最新清单反映改动
	first := entryOf(t, latest, "deepseek-v4-flash")
	if first["name"] != "新名字" || first["url"] != "https://new.example.com/chat/completions" || first["supports_images"] != true {
		t.Errorf("写回后的字段未生效,实际 %v", first)
	}
	if first["default_effort"] != "max" {
		t.Errorf("写回后 default_effort 应为 max,实际 %v", first["default_effort"])
	}
	if efforts, ok := first["supported_efforts"].([]string); !ok || len(efforts) != 3 {
		t.Errorf("写回后 supported_efforts 应为 3 项,实际 %v(%T)", first["supported_efforts"], first["supported_efforts"])
	}

	// 重新解析文件,验证白名单之外的键逐条原样保留
	arr := readFixtureArray(t, home)
	if len(arr) != 2 {
		t.Fatalf("数组条目数不得变化,实际 %d 条", len(arr))
	}
	m0 := fixtureElement(t, arr, 0)
	if key := stringOf(m0["apiKey"]); key != wbFixtureKey {
		t.Errorf("apiKey 必须原样保留,实际 %q", key)
	}
	if stringOf(m0["id"]) != "deepseek-v4-flash" || stringOf(m0["vendor"]) != "deepseek" {
		t.Errorf("id/vendor 不得被修改,实际 %v / %v", m0["id"], m0["vendor"])
	}
	if note := stringOf(mapGetObj(m0, "unknownKey")["note"]); note != "keep" {
		t.Errorf("未知键 unknownKey 应原样保留,实际 %v", mapGetObj(m0, "unknownKey")["note"])
	}
	if b, ok := boolOf(m0["supportsToolCall"]); !ok || !b {
		t.Errorf("未提交的 supportsToolCall 应原样保留,实际 %v", m0["supportsToolCall"])
	}
	if n, ok := m0["maxInputTokens"].(json.Number); !ok || n.String() != "128000" {
		t.Errorf("未提交的 maxInputTokens 应原样保留,实际 %v(%T)", m0["maxInputTokens"], m0["maxInputTokens"])
	}
	if e := stringOf(mapGetObj(m0, "reasoning")["defaultEffort"]); e != "max" {
		t.Errorf("reasoning.defaultEffort 应为 max,实际 %v", e)
	}
	if efforts := stringSliceOf(mapGetObj(m0, "reasoning")["supportedEfforts"]); len(efforts) != 3 || efforts[0] != "low" {
		t.Errorf("reasoning.supportedEfforts 应为 [low high max],实际 %v", efforts)
	}
	if b, ok := boolOf(mapGetObj(m0, "reasoning")["canDisableThinking"]); !ok || b {
		t.Errorf("未提交的 reasoning.canDisableThinking 应原样保留为 false,实际 %v", mapGetObj(m0, "reasoning")["canDisableThinking"])
	}
	// 未命中的条目逐字节语义保留
	m1 := fixtureElement(t, arr, 1)
	if key := stringOf(m1["apiKey"]); key != "sk-wb-second" {
		t.Errorf("未命中条目的 apiKey 应原样保留,实际 %q", key)
	}
	if stringOf(m1["name"]) != "第二个模型" {
		t.Errorf("未命中条目不得被修改,实际 %v", m1["name"])
	}
	// 写回文件保持 2 空格缩进
	raw, err := os.ReadFile(filepath.Join(home, ".workbuddy", "models.json"))
	if err != nil {
		t.Fatalf("读取写回后的文件失败: %v", err)
	}
	if !strings.Contains(string(raw), "\n  {\n    \"apiKey\"") {
		t.Errorf("写回应为 2 空格缩进,实际:\n%s", raw)
	}
	// 写回前自动备份
	entries, err := os.ReadDir(filepath.Join(dataDir, "agent-backups", "workbuddy"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("写回应生成 1 份备份,实际 err=%v entries=%d", err, len(entries))
	}
}

// TestWorkBuddy_ApplyModels_新增字段写回 7 个新键全部落盘,显式 false 与
// 缺省 true 的 canDisableThinking 均按提交值写入。
func TestWorkBuddy_ApplyModels_新增字段写回(t *testing.T) {
	home := t.TempDir()
	writeWorkBuddyFixture(t, home, testModelsJSON)

	patches := []agentconf.ModelPatch{{
		ModelID: "second-model",
		Fields: map[string]any{
			"disabled":             true,
			"only_reasoning":       true,
			"use_custom_protocol":  false,
			"can_disable_thinking": true,
			"max_input_tokens":     200000,
			"max_output_tokens":    65536,
			"temperature":          1.5,
		},
	}}
	if _, err := NewWorkBuddyAgent(home).ApplyModels(context.Background(), patches, t.TempDir()); err != nil {
		t.Fatalf("ApplyModels 失败: %v", err)
	}
	m1 := fixtureElement(t, readFixtureArray(t, home), 1)
	if b, ok := boolOf(m1["disabled"]); !ok || !b {
		t.Errorf("disabled 应为 true,实际 %v(%T)", m1["disabled"], m1["disabled"])
	}
	if b, ok := boolOf(m1["onlyReasoning"]); !ok || !b {
		t.Errorf("onlyReasoning 应为 true,实际 %v", m1["onlyReasoning"])
	}
	if b, ok := boolOf(m1["useCustomProtocol"]); !ok || b {
		t.Errorf("useCustomProtocol 应为显式 false(缺省即 false,但提交值必须落盘),实际 %v", m1["useCustomProtocol"])
	}
	if n, ok := m1["maxInputTokens"].(json.Number); !ok || n.String() != "200000" {
		t.Errorf("maxInputTokens 应为 200000,实际 %v(%T)", m1["maxInputTokens"], m1["maxInputTokens"])
	}
	if n, ok := m1["maxOutputTokens"].(json.Number); !ok || n.String() != "65536" {
		t.Errorf("maxOutputTokens 应为 65536,实际 %v", m1["maxOutputTokens"])
	}
	if n, ok := m1["temperature"].(json.Number); !ok || n.String() != "1.5" {
		t.Errorf("temperature 应为 1.5,实际 %v", m1["temperature"])
	}
	reasoning := mapGetObj(m1, "reasoning")
	if reasoning == nil {
		t.Fatal("reasoning 节点应被创建")
	}
	if b, ok := boolOf(reasoning["canDisableThinking"]); !ok || !b {
		t.Errorf("reasoning.canDisableThinking 应为 true,实际 %v", reasoning["canDisableThinking"])
	}
	if _, exists := reasoning["defaultEffort"]; exists {
		t.Error("创建的 reasoning 节点不应包含未提交的 defaultEffort 键")
	}
}

// TestWorkBuddy_Models_对象形态与零丢失 对象形态 {models, availableModels}
// 与裸数组同等读写,写回后其余顶层键原样保留。
func TestWorkBuddy_Models_对象形态与零丢失(t *testing.T) {
	home := t.TempDir()
	writeWorkBuddyFixture(t, home, `{
	  "version": 3,
	  "availableModels": ["obj-model"],
	  "models": [
	    {"id": "obj-model", "name": "对象形态模型", "apiKey": "sk-wb-obj", "supportsToolCall": true}
	  ]
	}`)
	agent := NewWorkBuddyAgent(home)

	entries, err := agent.Models(context.Background())
	if err != nil {
		t.Fatalf("对象形态 Models 不应报错: %v", err)
	}
	if len(entries) != 1 || entries[0].ModelID != "obj-model" {
		t.Fatalf("对象形态应读出 1 个条目,实际 %+v", entries)
	}

	patches := []agentconf.ModelPatch{{ModelID: "obj-model", Fields: map[string]any{"name": "已修改"}}}
	if _, err := agent.ApplyModels(context.Background(), patches, t.TempDir()); err != nil {
		t.Fatalf("对象形态 ApplyModels 失败: %v", err)
	}
	obj, ok := readFixtureDoc(t, home).(map[string]any)
	if !ok {
		t.Fatal("对象形态写回后顶层必须仍是对象")
	}
	if v, ok := obj["availableModels"]; !ok {
		t.Error("availableModels 应原样保留")
	} else if ids := stringSliceOf(v); len(ids) != 1 || ids[0] != "obj-model" {
		t.Errorf("availableModels 内容应不变,实际 %v", ids)
	}
	if v, ok := obj["version"].(json.Number); !ok || v.String() != "3" {
		t.Errorf("未知顶层键 version 应原样保留,实际 %v(%T)", obj["version"], obj["version"])
	}
	m0, ok := anySlice(obj["models"])[0].(map[string]any)
	if !ok {
		t.Fatal("models[0] 应为对象")
	}
	if stringOf(m0["name"]) != "已修改" {
		t.Errorf("对象形态下模型字段应生效,实际 name=%v", m0["name"])
	}
	if key := stringOf(m0["apiKey"]); key != "sk-wb-obj" {
		t.Errorf("对象形态下 apiKey 必须原样保留,实际 %q", key)
	}
}

// TestWorkBuddy_对象形态无models数组_4402 顶层对象缺少 models 数组(或不是
// 数组)时报 4402,不得当作 WorkBuddy 配置编辑。
func TestWorkBuddy_对象形态无models数组_4402(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"无 models 键", `{"id": "m"}`},
		{"models 不是数组", `{"models": "nope"}`},
		{"顶层是标量", `"just a string"`},
	}
	for _, tc := range cases {
		home := t.TempDir()
		writeWorkBuddyFixture(t, home, tc.content)
		agent := NewWorkBuddyAgent(home)
		if _, err := agent.Models(context.Background()); !isAgentFileIO(err) {
			t.Errorf("%s: Models 期望 4402,实际 %v", tc.name, err)
		}
		patches := []agentconf.ModelPatch{{ModelID: "m", Fields: map[string]any{"name": "x"}}}
		if _, err := agent.ApplyModels(context.Background(), patches, t.TempDir()); !isAgentFileIO(err) {
			t.Errorf("%s: ApplyModels 期望 4402,实际 %v", tc.name, err)
		}
	}
}

// TestWorkBuddy_ApplyModels_首个匹配定位 存在重复 id 时仅首个元素被修改。
func TestWorkBuddy_ApplyModels_首个匹配定位(t *testing.T) {
	home := t.TempDir()
	writeWorkBuddyFixture(t, home, `[
		{"id": "dup", "name": "第一个", "apiKey": "sk-first"},
		{"id": "dup", "name": "第二个", "apiKey": "sk-second"}
	]`)

	patches := []agentconf.ModelPatch{{ModelID: "dup", Fields: map[string]any{"name": "已修改"}}}
	if _, err := NewWorkBuddyAgent(home).ApplyModels(context.Background(), patches, t.TempDir()); err != nil {
		t.Fatalf("ApplyModels 失败: %v", err)
	}
	arr := readFixtureArray(t, home)
	if stringOf(fixtureElement(t, arr, 0)["name"]) != "已修改" {
		t.Errorf("首个匹配元素应被修改,实际 %v", fixtureElement(t, arr, 0)["name"])
	}
	if stringOf(fixtureElement(t, arr, 1)["name"]) != "第二个" {
		t.Errorf("第二个同 id 元素不得被修改,实际 %v", fixtureElement(t, arr, 1)["name"])
	}
	if key := stringOf(fixtureElement(t, arr, 1)["apiKey"]); key != "sk-second" {
		t.Errorf("第二个元素应原样保留,apiKey 实际 %q", key)
	}
}

// TestWorkBuddy_ApplyModels_reasoning节点创建 reasoning 缺失时写推理强度会创建节点。
func TestWorkBuddy_ApplyModels_reasoning节点创建(t *testing.T) {
	home := t.TempDir()
	writeWorkBuddyFixture(t, home, testModelsJSON)

	patches := []agentconf.ModelPatch{{ModelID: "second-model", Fields: map[string]any{"default_effort": "high"}}}
	if _, err := NewWorkBuddyAgent(home).ApplyModels(context.Background(), patches, t.TempDir()); err != nil {
		t.Fatalf("ApplyModels 失败: %v", err)
	}
	reasoning := mapGetObj(fixtureElement(t, readFixtureArray(t, home), 1), "reasoning")
	if reasoning == nil {
		t.Fatal("reasoning 节点应被创建")
	}
	if stringOf(reasoning["defaultEffort"]) != "high" {
		t.Errorf("reasoning.defaultEffort 应为 high,实际 %v", reasoning["defaultEffort"])
	}
	if _, exists := reasoning["supportedEfforts"]; exists {
		t.Error("创建的 reasoning 节点不应包含未提交的 supportedEfforts 键")
	}
}

// TestWorkBuddy_ApplyModels_白名单外键_4403 提交白名单外的键报 4403 且不落盘。
func TestWorkBuddy_ApplyModels_白名单外键_4403(t *testing.T) {
	home := t.TempDir()
	writeWorkBuddyFixture(t, home, testModelsJSON)

	for name, fields := range map[string]map[string]any{
		"凭据类键":  {"api_key": "sk-attack"},
		"只读标识":  {"id": "hijack"},
		"只读供应商": {"vendor": "evil"},
		"标签数组":  {"tags": []any{"custom"}},
		"未知键":   {"anything": true},
	} {
		patches := []agentconf.ModelPatch{{ModelID: "deepseek-v4-flash", Fields: fields}}
		_, err := NewWorkBuddyAgent(home).ApplyModels(context.Background(), patches, t.TempDir())
		if !isAgentInvalid(err) {
			t.Errorf("%s: 期望 4403,实际 %v", name, err)
		}
	}
	// 校验失败不得写盘:apiKey 必须与夹具一致
	m0 := fixtureElement(t, readFixtureArray(t, home), 0)
	if key := stringOf(m0["apiKey"]); key != wbFixtureKey {
		t.Errorf("校验失败后文件不得被改写,apiKey 实际 %q", key)
	}
}

// TestWorkBuddy_ApplyModels_非法值类型_4403 类型不符的值报 4403。
func TestWorkBuddy_ApplyModels_非法值类型_4403(t *testing.T) {
	home := t.TempDir()
	writeWorkBuddyFixture(t, home, testModelsJSON)

	cases := []struct {
		name   string
		fields map[string]any
	}{
		{"name 为数字", map[string]any{"name": 123}},
		{"能力开关为字符串", map[string]any{"supports_tool_call": "yes"}},
		{"推理强度为数字", map[string]any{"default_effort": 3}},
		{"强度数组为字符串", map[string]any{"supported_efforts": "high"}},
		{"强度数组含非字符串", map[string]any{"supported_efforts": []any{"high", 1}}},
		{"disabled 为字符串", map[string]any{"disabled": "true"}},
		{"only_reasoning 为字符串", map[string]any{"only_reasoning": 1}},
		{"use_custom_protocol 为字符串", map[string]any{"use_custom_protocol": "on"}},
		{"can_disable_thinking 为字符串", map[string]any{"can_disable_thinking": "yes"}},
		{"max_input_tokens 为字符串", map[string]any{"max_input_tokens": "128000"}},
		{"max_output_tokens 为布尔", map[string]any{"max_output_tokens": true}},
		{"temperature 为字符串", map[string]any{"temperature": "0.7"}},
	}
	for _, tc := range cases {
		patches := []agentconf.ModelPatch{{ModelID: "deepseek-v4-flash", Fields: tc.fields}}
		_, err := NewWorkBuddyAgent(home).ApplyModels(context.Background(), patches, t.TempDir())
		if !isAgentInvalid(err) {
			t.Errorf("%s: 期望 4403,实际 %v", tc.name, err)
		}
	}
}

// TestWorkBuddy_ApplyModels_id不存在_4404 不存在的 modelId 报 4404。
func TestWorkBuddy_ApplyModels_id不存在_4404(t *testing.T) {
	home := t.TempDir()
	writeWorkBuddyFixture(t, home, testModelsJSON)

	patches := []agentconf.ModelPatch{{ModelID: "no-such-model", Fields: map[string]any{"name": "x"}}}
	_, err := NewWorkBuddyAgent(home).ApplyModels(context.Background(), patches, t.TempDir())
	if !isAgentNotFound(err) {
		t.Errorf("期望 4404,实际 %v", err)
	}
}

// TestWorkBuddy_Models_文件缺失_4404 配置文件不存在时读与写均报 4404。
func TestWorkBuddy_Models_文件缺失_4404(t *testing.T) {
	home := t.TempDir()
	agent := NewWorkBuddyAgent(home)
	if _, err := agent.Models(context.Background()); !isAgentNotFound(err) {
		t.Errorf("Models 期望 4404,实际 %v", err)
	}
	patches := []agentconf.ModelPatch{{ModelID: "m", Fields: map[string]any{"name": "x"}}}
	if _, err := agent.ApplyModels(context.Background(), patches, t.TempDir()); !isAgentNotFound(err) {
		t.Errorf("ApplyModels 期望 4404,实际 %v", err)
	}
}

// TestWorkBuddy_非法JSON_4402 非法 JSON 文本报 4402。
func TestWorkBuddy_非法JSON_4402(t *testing.T) {
	home := t.TempDir()
	writeWorkBuddyFixture(t, home, "这不是 JSON")
	agent := NewWorkBuddyAgent(home)
	if _, err := agent.Models(context.Background()); !isAgentFileIO(err) {
		t.Errorf("Models 期望 4402,实际 %v", err)
	}
	patches := []agentconf.ModelPatch{{ModelID: "m", Fields: map[string]any{"name": "x"}}}
	if _, err := agent.ApplyModels(context.Background(), patches, t.TempDir()); !isAgentFileIO(err) {
		t.Errorf("ApplyModels 期望 4402,实际 %v", err)
	}
}

// TestWorkBuddy_ApplyModels_空patch_不落盘 空 patch 列表只返回现状,不备份不写盘。
func TestWorkBuddy_ApplyModels_空patch_不落盘(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeWorkBuddyFixture(t, home, testModelsJSON)

	entries, err := NewWorkBuddyAgent(home).ApplyModels(context.Background(), []agentconf.ModelPatch{}, dataDir)
	if err != nil {
		t.Fatalf("空 patch 不应报错: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("空 patch 应返回现有清单,实际 %d 条", len(entries))
	}
	if _, err := os.Stat(filepath.Join(dataDir, "agent-backups", "workbuddy")); !os.IsNotExist(err) {
		t.Errorf("空 patch 不应产生备份目录,实际 err=%v", err)
	}
}

// TestWorkBuddy_ApplyModels_supportedEfforts写回 supported_efforts 列可写回:
// []string 提交(前端 list 列的 Go 侧等价形态)落为字符串数组,reasoning
// 节点缺失时创建,其余键原样。
func TestWorkBuddy_ApplyModels_supportedEfforts写回(t *testing.T) {
	home := t.TempDir()
	writeWorkBuddyFixture(t, home, testModelsJSON)

	patches := []agentconf.ModelPatch{{ModelID: "second-model", Fields: map[string]any{
		"supported_efforts": []string{"low", "medium"},
	}}}
	latest, err := NewWorkBuddyAgent(home).ApplyModels(context.Background(), patches, t.TempDir())
	if err != nil {
		t.Fatalf("ApplyModels 失败: %v", err)
	}
	if efforts, ok := entryOf(t, latest, "second-model")["supported_efforts"].([]string); !ok || len(efforts) != 2 {
		t.Errorf("写回后 supported_efforts 应为 2 项,实际 %v(%T)", latest[1].Fields["supported_efforts"], latest[1].Fields["supported_efforts"])
	}
	m1 := fixtureElement(t, readFixtureArray(t, home), 1)
	efforts := stringSliceOf(mapGetObj(m1, "reasoning")["supportedEfforts"])
	if len(efforts) != 2 || efforts[0] != "low" || efforts[1] != "medium" {
		t.Errorf("reasoning.supportedEfforts 应为 [low medium],实际 %v", efforts)
	}
	if key := stringOf(m1["apiKey"]); key != "sk-wb-second" {
		t.Errorf("apiKey 必须原样保留,实际 %q", key)
	}
}

// TestWorkBuddy_能力一致性 WorkBuddy 无"默认模型"概念:能力位恒 false、
// 默认模型恒 nil,且不实现 DefaultModelSetter(handler 将按未实现报 4405)。
func TestWorkBuddy_能力一致性(t *testing.T) {
	home := t.TempDir()
	writeWorkBuddyFixture(t, home, testModelsJSON)

	snap := NewWorkBuddyAgent(home).Snapshot(context.Background())
	if snap.SupportsDefaultModel {
		t.Error("WorkBuddy 能力位应为 false")
	}
	if snap.DefaultModel != nil {
		t.Errorf("WorkBuddy 默认模型应为 nil,实际 %+v", snap.DefaultModel)
	}
	setterType := reflect.TypeOf((*agentconf.DefaultModelSetter)(nil)).Elem()
	if reflect.TypeOf(&WorkBuddyAgent{}).Implements(setterType) {
		t.Error("WorkBuddy 不应实现 DefaultModelSetter 接口")
	}
}
