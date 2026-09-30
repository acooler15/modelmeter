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

// 测试夹具:伪造的 models.json(对象形态),结构对齐任务 research.md 的真实
// 勘探结论:顶层对象含 models 数组与 availableModels(程序写侧会维护的可见性
// 键)及未知顶层键;首条含未知键、完整 reasoning 节点与全部白名单字段
// (disabled/onlyReasoning/数字项/reasoning.canDisableThinking);次条缺
// reasoning 节点;第三条无字符串 id(运行时会过滤,清单应跳过)。
// apiKey 均为测试虚构值。
const testCodeBuddyJSON = `{
  "version": 1,
  "availableModels": ["deepseek-v4-pro"],
  "models": [
    {
      "id": "deepseek-v4-pro",
      "name": "DeepSeek V4 Pro",
      "vendor": "deepseek",
      "url": "https://api.example.com/chat/completions",
      "apiKey": "sk-cb-fixture-never-leak",
      "maxInputTokens": 128000,
      "maxOutputTokens": 8192,
      "temperature": 0.7,
      "supportsToolCall": true,
      "supportsImages": false,
      "supportsReasoning": true,
      "onlyReasoning": false,
      "disabled": true,
      "reasoning": {"defaultEffort": "high", "supportedEfforts": ["high", "max"], "canDisableThinking": false},
      "unknownKey": {"note": "keep"}
    },
    {
      "id": "deepseek-v4-flash",
      "name": "deepseek-v4-flash",
      "vendor": "user",
      "url": "https://x.example.com/v1/chat/completions",
      "apiKey": "sk-cb-second",
      "supportsToolCall": false
    },
    {
      "name": "无 id 条目"
    }
  ]
}`

// cbFixtureKey 首条记录的明文 apiKey,用于验证"不出现/不被改写"。
const cbFixtureKey = "sk-cb-fixture-never-leak"

// writeCodeBuddyFixture 在临时 home 下伪造 ~/.codebuddy/models.json。
func writeCodeBuddyFixture(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".codebuddy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("创建伪造配置目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "models.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("写入伪造 models.json 失败: %v", err)
	}
}

// readCodeBuddyDoc 重新解析磁盘上的 models.json(UseNumber)为顶层对象,
// 供写回断言使用。
func readCodeBuddyDoc(t *testing.T, home string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, ".codebuddy", "models.json"))
	if err != nil {
		t.Fatalf("读取写回后的文件失败: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		t.Fatalf("写回后的文件不是合法 JSON: %v", err)
	}
	obj, ok := root.(map[string]any)
	if !ok {
		t.Fatalf("写回后的文件顶层不是对象")
	}
	return obj
}

// cbFixtureModels 取写回结果中的 models 数组。
func cbFixtureModels(t *testing.T, home string) []any {
	return anySlice(readCodeBuddyDoc(t, home)["models"])
}

// cbFixtureElement 取 models 数组中第 i 个元素(必须是对象),否则判失败。
func cbFixtureElement(t *testing.T, arr []any, i int) map[string]any {
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

// TestCodeBuddy_Models_清单与脱敏 验证字段映射(含全部白名单字段)、缺省
// 语义、无 id 条目跳过与读侧脱敏。
func TestCodeBuddy_Models_清单与脱敏(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

	entries, err := NewCodeBuddyAgent(home).Models(context.Background())
	if err != nil {
		t.Fatalf("Models 失败: %v", err)
	}
	// 无字符串 id 的条目按运行时口径跳过
	if len(entries) != 2 {
		t.Fatalf("应有 2 个条目(无 id 条目跳过),实际 %d 个", len(entries))
	}
	first := entryOf(t, entries, "deepseek-v4-pro")
	if entries[0].DisplayName != "DeepSeek V4 Pro" {
		t.Errorf("display_name 应为显示名,实际 %q", entries[0].DisplayName)
	}
	if first["name"] != "DeepSeek V4 Pro" || first["vendor"] != "deepseek" {
		t.Errorf("name/vendor 解析错误,实际 %v / %v", first["name"], first["vendor"])
	}
	if first["url"] != "https://api.example.com/chat/completions" {
		t.Errorf("url 应为完整 endpoint,实际 %v", first["url"])
	}
	if first["supports_tool_call"] != true || first["supports_images"] != false || first["supports_reasoning"] != true {
		t.Errorf("能力开关解析错误,实际 %v / %v / %v", first["supports_tool_call"], first["supports_images"], first["supports_reasoning"])
	}
	if first["disabled"] != true || first["only_reasoning"] != false {
		t.Errorf("disabled/only_reasoning 解析错误,实际 %v / %v", first["disabled"], first["only_reasoning"])
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
	second := entryOf(t, entries, "deepseek-v4-flash")
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
	if strings.Contains(string(entriesJSON), cbFixtureKey) || strings.Contains(string(entriesJSON), "sk-cb-second") {
		t.Error("模型清单不得包含凭据内容")
	}
}

// TestCodeBuddy_Models_effort兜底 default_effort 读取兜底 legacy 键 effort。
func TestCodeBuddy_Models_effort兜底(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, `{
		"models": [{"id": "legacy", "name": "旧键模型", "reasoning": {"effort": "medium"}}]
	}`)

	entries, err := NewCodeBuddyAgent(home).Models(context.Background())
	if err != nil {
		t.Fatalf("Models 失败: %v", err)
	}
	if got := entryOf(t, entries, "legacy")["default_effort"]; got != "medium" {
		t.Errorf("default_effort 应兜底读取 reasoning.effort=medium,实际 %v", got)
	}
}

// TestCodeBuddy_Snapshot_列描述 验证 found 状态、Columns 描述与下拉选项。
func TestCodeBuddy_Snapshot_列描述(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

	snap := NewCodeBuddyAgent(home).Snapshot(context.Background())
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
	// 全部布尔与数字列均可编辑;CodeBuddy 无 use_custom_protocol 列
	for key, wantType := range map[string]string{
		"disabled":             "bool",
		"only_reasoning":       "bool",
		"can_disable_thinking": "bool",
		"max_input_tokens":     "number",
		"max_output_tokens":    "number",
		"temperature":          "number",
	} {
		if c := findColumn(t, snap.Columns, key); c.Type != wantType || c.Readonly {
			t.Errorf("列 %s 应为可编辑 %s 列,实际 %+v", key, wantType, c)
		}
	}
	// CodeBuddy 无 use_custom_protocol 列(findColumn 缺列即 Fatal,需手动遍历)
	for _, col := range snap.Columns {
		if col.Key == "use_custom_protocol" {
			t.Errorf("CodeBuddy 不应有 use_custom_protocol 列,实际 %+v", col)
		}
	}
	// 快照不得包含凭据内容
	snapJSON, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("快照序列化失败: %v", err)
	}
	if strings.Contains(string(snapJSON), cbFixtureKey) {
		t.Error("快照不得包含凭据内容")
	}
}

// TestCodeBuddy_Snapshot_文件缺失_not_found 配置文件缺失时降级为 not_found。
func TestCodeBuddy_Snapshot_文件缺失_not_found(t *testing.T) {
	home := t.TempDir()
	snap := NewCodeBuddyAgent(home).Snapshot(context.Background())
	if snap.Status != agentconf.StatusNotFound || snap.Message == "" {
		t.Fatalf("文件缺失应返回 not_found 且带指引,实际 %+v", snap)
	}
}

// TestCodeBuddy_ApplyModels_白名单写回零丢失 验证目标键生效,且 apiKey、
// 未知键、id、vendor、availableModels 等顶层键、未提交的字段及未命中的
// 条目逐条原样保留。
func TestCodeBuddy_ApplyModels_白名单写回零丢失(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

	patches := []agentconf.ModelPatch{{
		ModelID: "deepseek-v4-pro",
		Fields: map[string]any{
			"name":              "新名字",
			"url":               "https://new.example.com/chat/completions",
			"supports_images":   true,
			"default_effort":    "max",
			"supported_efforts": []any{"low", "high", "max"},
		},
	}}
	latest, err := NewCodeBuddyAgent(home).ApplyModels(context.Background(), patches, dataDir)
	if err != nil {
		t.Fatalf("ApplyModels 失败: %v", err)
	}
	// 返回的最新清单反映改动
	first := entryOf(t, latest, "deepseek-v4-pro")
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
	doc := readCodeBuddyDoc(t, home)
	if v, ok := doc["availableModels"]; !ok {
		t.Error("availableModels 应原样保留")
	} else if ids := stringSliceOf(v); len(ids) != 1 || ids[0] != "deepseek-v4-pro" {
		t.Errorf("availableModels 内容应不变,实际 %v", ids)
	}
	if v, ok := doc["version"].(json.Number); !ok || v.String() != "1" {
		t.Errorf("未知顶层键 version 应原样保留,实际 %v(%T)", doc["version"], doc["version"])
	}
	arr := cbFixtureModels(t, home)
	if len(arr) != 3 {
		t.Fatalf("数组条目数不得变化,实际 %d 条", len(arr))
	}
	m0 := cbFixtureElement(t, arr, 0)
	if key := stringOf(m0["apiKey"]); key != cbFixtureKey {
		t.Errorf("apiKey 必须原样保留,实际 %q", key)
	}
	if stringOf(m0["id"]) != "deepseek-v4-pro" || stringOf(m0["vendor"]) != "deepseek" {
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
	m1 := cbFixtureElement(t, arr, 1)
	if key := stringOf(m1["apiKey"]); key != "sk-cb-second" {
		t.Errorf("未命中条目的 apiKey 应原样保留,实际 %q", key)
	}
	if stringOf(m1["name"]) != "deepseek-v4-flash" {
		t.Errorf("未命中条目不得被修改,实际 %v", m1["name"])
	}
	// 写回文件保持 2 空格缩进
	raw, err := os.ReadFile(filepath.Join(home, ".codebuddy", "models.json"))
	if err != nil {
		t.Fatalf("读取写回后的文件失败: %v", err)
	}
	if !strings.Contains(string(raw), "\n    {\n      \"apiKey\"") {
		t.Errorf("写回应为 2 空格缩进,实际:\n%s", raw)
	}
	// 写回前自动备份
	entries, err := os.ReadDir(filepath.Join(dataDir, "agent-backups", "codebuddy"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("写回应生成 1 份备份,实际 err=%v entries=%d", err, len(entries))
	}
}

// TestCodeBuddy_ApplyModels_新增字段写回 剩余白名单键全部落盘,显式 false
// 与缺省 true 的 canDisableThinking 均按提交值写入。
func TestCodeBuddy_ApplyModels_新增字段写回(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

	patches := []agentconf.ModelPatch{{
		ModelID: "deepseek-v4-flash",
		Fields: map[string]any{
			"disabled":             true,
			"only_reasoning":       true,
			"can_disable_thinking": true,
			"max_input_tokens":     200000,
			"max_output_tokens":    65536,
			"temperature":          1.5,
		},
	}}
	if _, err := NewCodeBuddyAgent(home).ApplyModels(context.Background(), patches, t.TempDir()); err != nil {
		t.Fatalf("ApplyModels 失败: %v", err)
	}
	m1 := cbFixtureElement(t, cbFixtureModels(t, home), 1)
	if b, ok := boolOf(m1["disabled"]); !ok || !b {
		t.Errorf("disabled 应为 true,实际 %v(%T)", m1["disabled"], m1["disabled"])
	}
	if b, ok := boolOf(m1["onlyReasoning"]); !ok || !b {
		t.Errorf("onlyReasoning 应为 true,实际 %v", m1["onlyReasoning"])
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

// TestCodeBuddy_ApplyModels_reasoning节点创建 reasoning 缺失时写推理强度会
// 创建节点。
func TestCodeBuddy_ApplyModels_reasoning节点创建(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

	patches := []agentconf.ModelPatch{{ModelID: "deepseek-v4-flash", Fields: map[string]any{"default_effort": "high"}}}
	if _, err := NewCodeBuddyAgent(home).ApplyModels(context.Background(), patches, t.TempDir()); err != nil {
		t.Fatalf("ApplyModels 失败: %v", err)
	}
	reasoning := mapGetObj(cbFixtureElement(t, cbFixtureModels(t, home), 1), "reasoning")
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

// TestCodeBuddy_ApplyModels_supportedEfforts写回 []string 提交(前端 list 列
// 的 Go 侧等价形态)落为字符串数组,reasoning 节点缺失时创建,其余键原样。
func TestCodeBuddy_ApplyModels_supportedEfforts写回(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

	patches := []agentconf.ModelPatch{{ModelID: "deepseek-v4-flash", Fields: map[string]any{
		"supported_efforts": []string{"low", "medium"},
	}}}
	latest, err := NewCodeBuddyAgent(home).ApplyModels(context.Background(), patches, t.TempDir())
	if err != nil {
		t.Fatalf("ApplyModels 失败: %v", err)
	}
	if efforts, ok := entryOf(t, latest, "deepseek-v4-flash")["supported_efforts"].([]string); !ok || len(efforts) != 2 {
		t.Errorf("写回后 supported_efforts 应为 2 项,实际 %v(%T)", latest[1].Fields["supported_efforts"], latest[1].Fields["supported_efforts"])
	}
	m1 := cbFixtureElement(t, cbFixtureModels(t, home), 1)
	efforts := stringSliceOf(mapGetObj(m1, "reasoning")["supportedEfforts"])
	if len(efforts) != 2 || efforts[0] != "low" || efforts[1] != "medium" {
		t.Errorf("reasoning.supportedEfforts 应为 [low medium],实际 %v", efforts)
	}
	if key := stringOf(m1["apiKey"]); key != "sk-cb-second" {
		t.Errorf("apiKey 必须原样保留,实际 %q", key)
	}
}

// TestCodeBuddy_ApplyModels_首个匹配定位 存在重复 id 时仅首个元素被修改。
func TestCodeBuddy_ApplyModels_首个匹配定位(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, `{
		"models": [
			{"id": "dup", "name": "第一个", "apiKey": "sk-first"},
			{"id": "dup", "name": "第二个", "apiKey": "sk-second"}
		]
	}`)

	patches := []agentconf.ModelPatch{{ModelID: "dup", Fields: map[string]any{"name": "已修改"}}}
	if _, err := NewCodeBuddyAgent(home).ApplyModels(context.Background(), patches, t.TempDir()); err != nil {
		t.Fatalf("ApplyModels 失败: %v", err)
	}
	arr := cbFixtureModels(t, home)
	if stringOf(cbFixtureElement(t, arr, 0)["name"]) != "已修改" {
		t.Errorf("首个匹配元素应被修改,实际 %v", cbFixtureElement(t, arr, 0)["name"])
	}
	if stringOf(cbFixtureElement(t, arr, 1)["name"]) != "第二个" {
		t.Errorf("第二个同 id 元素不得被修改,实际 %v", cbFixtureElement(t, arr, 1)["name"])
	}
	if key := stringOf(cbFixtureElement(t, arr, 1)["apiKey"]); key != "sk-second" {
		t.Errorf("第二个元素应原样保留,apiKey 实际 %q", key)
	}
}

// TestCodeBuddy_ApplyModels_白名单外键_4403 提交白名单外的键报 4403 且不落盘。
func TestCodeBuddy_ApplyModels_白名单外键_4403(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

	for name, fields := range map[string]map[string]any{
		"凭据类键":  {"api_key": "sk-attack"},
		"只读标识":  {"id": "hijack"},
		"只读供应商": {"vendor": "evil"},
		"标签数组":  {"tags": []any{"custom"}},
		"未知键":   {"anything": true},
	} {
		patches := []agentconf.ModelPatch{{ModelID: "deepseek-v4-pro", Fields: fields}}
		_, err := NewCodeBuddyAgent(home).ApplyModels(context.Background(), patches, t.TempDir())
		if !isAgentInvalid(err) {
			t.Errorf("%s: 期望 4403,实际 %v", name, err)
		}
	}
	// 校验失败不得写盘:apiKey 必须与夹具一致
	m0 := cbFixtureElement(t, cbFixtureModels(t, home), 0)
	if key := stringOf(m0["apiKey"]); key != cbFixtureKey {
		t.Errorf("校验失败后文件不得被改写,apiKey 实际 %q", key)
	}
}

// TestCodeBuddy_ApplyModels_非法值类型_4403 类型不符的值报 4403。
func TestCodeBuddy_ApplyModels_非法值类型_4403(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

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
		{"can_disable_thinking 为字符串", map[string]any{"can_disable_thinking": "yes"}},
		{"max_input_tokens 为字符串", map[string]any{"max_input_tokens": "128000"}},
		{"max_output_tokens 为布尔", map[string]any{"max_output_tokens": true}},
		{"temperature 为字符串", map[string]any{"temperature": "0.7"}},
	}
	for _, tc := range cases {
		patches := []agentconf.ModelPatch{{ModelID: "deepseek-v4-pro", Fields: tc.fields}}
		_, err := NewCodeBuddyAgent(home).ApplyModels(context.Background(), patches, t.TempDir())
		if !isAgentInvalid(err) {
			t.Errorf("%s: 期望 4403,实际 %v", tc.name, err)
		}
	}
}

// TestCodeBuddy_ApplyModels_id不存在_4404 不存在的 id 报 4404。
func TestCodeBuddy_ApplyModels_id不存在_4404(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

	patches := []agentconf.ModelPatch{{ModelID: "no-such-model", Fields: map[string]any{"name": "x"}}}
	_, err := NewCodeBuddyAgent(home).ApplyModels(context.Background(), patches, t.TempDir())
	if !isAgentNotFound(err) {
		t.Errorf("期望 4404,实际 %v", err)
	}
}

// TestCodeBuddy_Models_文件缺失_4404 配置文件不存在时读与写均报 4404。
func TestCodeBuddy_Models_文件缺失_4404(t *testing.T) {
	home := t.TempDir()
	agent := NewCodeBuddyAgent(home)
	if _, err := agent.Models(context.Background()); !isAgentNotFound(err) {
		t.Errorf("Models 期望 4404,实际 %v", err)
	}
	patches := []agentconf.ModelPatch{{ModelID: "m", Fields: map[string]any{"name": "x"}}}
	if _, err := agent.ApplyModels(context.Background(), patches, t.TempDir()); !isAgentNotFound(err) {
		t.Errorf("ApplyModels 期望 4404,实际 %v", err)
	}
}

// TestCodeBuddy_非法JSON_4402 非法 JSON 文本报 4402。
func TestCodeBuddy_非法JSON_4402(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, "这不是 JSON")
	agent := NewCodeBuddyAgent(home)
	if _, err := agent.Models(context.Background()); !isAgentFileIO(err) {
		t.Errorf("Models 期望 4402,实际 %v", err)
	}
	patches := []agentconf.ModelPatch{{ModelID: "m", Fields: map[string]any{"name": "x"}}}
	if _, err := agent.ApplyModels(context.Background(), patches, t.TempDir()); !isAgentFileIO(err) {
		t.Errorf("ApplyModels 期望 4402,实际 %v", err)
	}
}

// TestCodeBuddy_裸数组顶层_4402 CodeBuddy 仅支持对象形态:裸数组读不到模型、
// 写回会丢数据,读与写均报 4402,不得当作 CodeBuddy 配置编辑。
func TestCodeBuddy_裸数组顶层_4402(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, `[{"id": "m", "name": "裸数组"}]`)
	agent := NewCodeBuddyAgent(home)
	if _, err := agent.Models(context.Background()); !isAgentFileIO(err) {
		t.Errorf("Models 期望 4402,实际 %v", err)
	}
	patches := []agentconf.ModelPatch{{ModelID: "m", Fields: map[string]any{"name": "x"}}}
	if _, err := agent.ApplyModels(context.Background(), patches, t.TempDir()); !isAgentFileIO(err) {
		t.Errorf("ApplyModels 期望 4402,实际 %v", err)
	}
}

// TestCodeBuddy_对象缺models数组_按空清单 对齐程序 buildResponse 口径:
// 缺 models 键是"新建后未添加模型"的正常态,Models 返回空清单不报错,
// patch 定位必然 4404,不得误写文件。
func TestCodeBuddy_对象缺models数组_按空清单(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, `{"id": "m"}`)
	agent := NewCodeBuddyAgent(home)

	entries, err := agent.Models(context.Background())
	if err != nil {
		t.Fatalf("缺 models 数组不应报错: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("缺 models 数组应返回空清单,实际 %d 条", len(entries))
	}
	patches := []agentconf.ModelPatch{{ModelID: "m", Fields: map[string]any{"name": "x"}}}
	if _, err := agent.ApplyModels(context.Background(), patches, t.TempDir()); !isAgentNotFound(err) {
		t.Errorf("patch 期望 4404,实际 %v", err)
	}
	// 校验失败不得写盘:models 键不得被凭空创建
	if _, exists := readCodeBuddyDoc(t, home)["models"]; exists {
		t.Error("写失败后不得凭空创建 models 键")
	}
}

// TestCodeBuddy_ApplyModels_空patch_不落盘 空 patch 列表只返回现状,不备份
// 不写盘。
func TestCodeBuddy_ApplyModels_空patch_不落盘(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

	entries, err := NewCodeBuddyAgent(home).ApplyModels(context.Background(), []agentconf.ModelPatch{}, dataDir)
	if err != nil {
		t.Fatalf("空 patch 不应报错: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("空 patch 应返回现有清单,实际 %d 条", len(entries))
	}
	if _, err := os.Stat(filepath.Join(dataDir, "agent-backups", "codebuddy")); !os.IsNotExist(err) {
		t.Errorf("空 patch 不应产生备份目录,实际 err=%v", err)
	}
}

// TestCodeBuddy_能力一致性 CodeBuddy 无"默认模型"概念:能力位恒 false、
// 默认模型恒 nil,且不实现 DefaultModelSetter(handler 将按未实现报 4405)。
func TestCodeBuddy_能力一致性(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

	snap := NewCodeBuddyAgent(home).Snapshot(context.Background())
	if snap.SupportsDefaultModel {
		t.Error("CodeBuddy 能力位应为 false")
	}
	if snap.DefaultModel != nil {
		t.Errorf("CodeBuddy 默认模型应为 nil,实际 %+v", snap.DefaultModel)
	}
	setterType := reflect.TypeOf((*agentconf.DefaultModelSetter)(nil)).Elem()
	if reflect.TypeOf(&CodeBuddyAgent{}).Implements(setterType) {
		t.Error("CodeBuddy 不应实现 DefaultModelSetter 接口")
	}
}

// TestCodeBuddy_RemoveModels_对象形态 对象形态删除:命中条目移除(非对象元素
// 保留)、顶层 availableModels 同步移除命中的 id、既有 apiKey 零接触、备份产生。
func TestCodeBuddy_RemoveModels_对象形态(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeCodeBuddyFixture(t, home, `{
		"version": 1,
		"availableModels": ["cb-a", "cb-b"],
		"models": [
			{"id": "cb-a", "name": "A", "apiKey": "sk-cb-a"},
			"纯字符串元素",
			{"id": "cb-b", "name": "B", "apiKey": "sk-cb-b"}
		]
	}`)

	latest, err := NewCodeBuddyAgent(home).RemoveModels(context.Background(),
		[]agentconf.ModelRef{{ModelID: "cb-a"}}, dataDir)
	if err != nil {
		t.Fatalf("RemoveModels 失败: %v", err)
	}
	if len(latest) != 1 || latest[0].ModelID != "cb-b" {
		t.Errorf("最新清单应只剩 cb-b,实际 %+v", latest)
	}
	doc := readCodeBuddyDoc(t, home)
	if ids := stringSliceOf(doc["availableModels"]); len(ids) != 1 || ids[0] != "cb-b" {
		t.Errorf("availableModels 应同步移除 cb-a,实际 %v", ids)
	}
	if v, ok := doc["version"].(json.Number); !ok || v.String() != "1" {
		t.Errorf("未知顶层键 version 应原样保留,实际 %v(%T)", doc["version"], doc["version"])
	}
	arr := anySlice(doc["models"])
	if len(arr) != 2 {
		t.Fatalf("数组应剩 2 个元素,实际 %d 个:%v", len(arr), arr)
	}
	if s, ok := arr[0].(string); !ok || s != "纯字符串元素" {
		t.Errorf("非对象元素应原样保留,实际 %v(%T)", arr[0], arr[0])
	}
	if key := stringOf(cbFixtureElement(t, arr, 1)["apiKey"]); key != "sk-cb-b" {
		t.Errorf("既有条目 apiKey 必须零接触,实际 %q", key)
	}
	if got := backupCount(t, dataDir, "codebuddy"); got != 1 {
		t.Errorf("删除写回应产生 1 份备份,实际 %d", got)
	}
}

// TestCodeBuddy_RemoveModels_availableModels缺失_不触碰 availableModels 键缺失
// 时删除不创建该键。
func TestCodeBuddy_RemoveModels_availableModels缺失_不触碰(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, `{"models": [{"id": "only", "name": "唯一"}]}`)

	if _, err := NewCodeBuddyAgent(home).RemoveModels(context.Background(),
		[]agentconf.ModelRef{{ModelID: "only"}}, t.TempDir()); err != nil {
		t.Fatalf("RemoveModels 失败: %v", err)
	}
	doc := readCodeBuddyDoc(t, home)
	if _, exists := doc["availableModels"]; exists {
		t.Error("availableModels 缺失时不得凭空创建")
	}
	if arr := anySlice(doc["models"]); len(arr) != 0 {
		t.Errorf("models 应已清空,实际 %v", arr)
	}
}

// TestCodeBuddy_RemoveModels_定位不存在_4404整批拒绝 任一定位不存在时报 4404,
// 整批不生效(不备份、不落盘)。
func TestCodeBuddy_RemoveModels_定位不存在_4404整批拒绝(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

	refs := []agentconf.ModelRef{
		{ModelID: "deepseek-v4-pro"},
		{ModelID: "no-such-model"},
	}
	if _, err := NewCodeBuddyAgent(home).RemoveModels(context.Background(), refs, dataDir); !isAgentNotFound(err) {
		t.Errorf("期望 4404,实际 %v", err)
	}
	if got := backupCount(t, dataDir, "codebuddy"); got != 0 {
		t.Errorf("校验失败不得产生备份,实际 %d 份", got)
	}
	if arr := cbFixtureModels(t, home); len(arr) != 3 {
		t.Errorf("校验失败后文件不得被改写,实际 %d 条", len(arr))
	}
}

// TestCodeBuddy_RemoveModels_空refs_不落盘 空 refs 只读返回现状,不备份不写盘。
func TestCodeBuddy_RemoveModels_空refs_不落盘(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

	entries, err := NewCodeBuddyAgent(home).RemoveModels(context.Background(), []agentconf.ModelRef{}, dataDir)
	if err != nil {
		t.Fatalf("空 refs 不应报错: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("空 refs 应返回现有清单,实际 %d 条", len(entries))
	}
	if got := backupCount(t, dataDir, "codebuddy"); got != 0 {
		t.Errorf("空 refs 不应产生备份,实际 %d 份", got)
	}
}

// TestCodeBuddy_AddModels_对象形态 新条目恰含 5 键(url 为完整 endpoint)、
// availableModels 为数组且不含该 id 时追加、既有条目 apiKey 零接触、备份产生。
func TestCodeBuddy_AddModels_对象形态(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

	req := agentconf.AddModelsRequest{
		ModelIDs: []string{"new-model", "deepseek-v4-pro"},
		Source: agentconf.ModelSource{
			ProviderName: "来源接口",
			EndpointURL:  "https://api.example.com/v1/chat/completions",
			APIKey:       "sk-source-credential",
		},
	}
	result, err := NewCodeBuddyAgent(home).AddModels(context.Background(), req, dataDir)
	if err != nil {
		t.Fatalf("AddModels 失败: %v", err)
	}
	if len(result.Added) != 1 || result.Added[0] != "new-model" {
		t.Errorf("added 应为 [new-model],实际 %v", result.Added)
	}
	if len(result.Skipped) != 1 || result.Skipped[0] != "deepseek-v4-pro" {
		t.Errorf("skipped 应为 [deepseek-v4-pro],实际 %v", result.Skipped)
	}
	if _, ok := entryOfOK(result.Entries, "new-model"); !ok {
		t.Error("最新清单应包含 new-model")
	}
	arr := cbFixtureModels(t, home)
	if len(arr) != 4 {
		t.Fatalf("数组应追加到尾部共 4 条,实际 %d 条", len(arr))
	}
	m := cbFixtureElement(t, arr, 3)
	if len(m) != 5 {
		t.Errorf("新条目应恰含 5 个键,实际 %d 个:%v", len(m), m)
	}
	for _, k := range []string{"id", "name", "vendor", "url", "apiKey"} {
		if _, ok := m[k]; !ok {
			t.Errorf("新条目缺少键 %s,实际 %v", k, m)
		}
	}
	if stringOf(m["url"]) != "https://api.example.com/v1/chat/completions" {
		t.Errorf("url 应为完整 endpoint,实际 %v", m["url"])
	}
	if stringOf(m["apiKey"]) != "sk-source-credential" {
		t.Errorf("apiKey 应写入来源接口凭据,实际 %q", stringOf(m["apiKey"]))
	}
	if ids := stringSliceOf(readCodeBuddyDoc(t, home)["availableModels"]); len(ids) != 2 || ids[1] != "new-model" {
		t.Errorf("availableModels 应追加为 [deepseek-v4-pro new-model],实际 %v", ids)
	}
	if key := stringOf(cbFixtureElement(t, arr, 0)["apiKey"]); key != cbFixtureKey {
		t.Errorf("既有条目 apiKey 必须零接触,实际 %q", key)
	}
	if got := backupCount(t, dataDir, "codebuddy"); got != 1 {
		t.Errorf("添加写回应产生 1 份备份,实际 %d", got)
	}
}

// TestCodeBuddy_AddModels_availableModels缺失_不创建 availableModels 键缺失时
// 只追加条目,不创建该键;缺 models 数组的空清单文件可正常添加(models 键创建)。
func TestCodeBuddy_AddModels_availableModels缺失_不创建(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, `{"version": 1}`)
	req := agentconf.AddModelsRequest{
		ModelIDs: []string{"m-1"},
		Source:   agentconf.ModelSource{ProviderName: "来源", EndpointURL: "https://s/v1/chat/completions", APIKey: "sk-x"},
	}
	if _, err := NewCodeBuddyAgent(home).AddModels(context.Background(), req, t.TempDir()); err != nil {
		t.Fatalf("空清单添加不应报错: %v", err)
	}
	doc := readCodeBuddyDoc(t, home)
	if _, exists := doc["availableModels"]; exists {
		t.Error("availableModels 缺失时不得凭空创建")
	}
	arr := anySlice(doc["models"])
	if len(arr) != 1 || stringOf(cbFixtureElement(t, arr, 0)["id"]) != "m-1" {
		t.Errorf("models 应有新条目 m-1,实际 %v", arr)
	}
	if v, ok := doc["version"].(json.Number); !ok || v.String() != "1" {
		t.Errorf("未知顶层键 version 应原样保留,实际 %v(%T)", doc["version"], doc["version"])
	}
}

// TestCodeBuddy_AddModels_全skip_零落盘 全部已存在时不备份不落盘。
func TestCodeBuddy_AddModels_全skip_零落盘(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

	req := agentconf.AddModelsRequest{
		ModelIDs: []string{"deepseek-v4-pro", "deepseek-v4-flash"},
		Source:   agentconf.ModelSource{EndpointURL: "https://s/v1/chat/completions", APIKey: "sk-x"},
	}
	result, err := NewCodeBuddyAgent(home).AddModels(context.Background(), req, dataDir)
	if err != nil {
		t.Fatalf("全 skip 不应报错: %v", err)
	}
	if len(result.Added) != 0 || len(result.Skipped) != 2 {
		t.Errorf("added/skipped 应为 0/2,实际 %v / %v", result.Added, result.Skipped)
	}
	if got := backupCount(t, dataDir, "codebuddy"); got != 0 {
		t.Errorf("全 skip 不应产生备份,实际 %d 份", got)
	}
	if arr := cbFixtureModels(t, home); len(arr) != 3 {
		t.Errorf("全 skip 时文件不得被改写,实际 %d 条", len(arr))
	}
}

// TestCodeBuddy_AddModels_model_ids空_4403 model_ids 为空报 4403 且零落盘。
func TestCodeBuddy_AddModels_model_ids空_4403(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeCodeBuddyFixture(t, home, testCodeBuddyJSON)

	_, err := NewCodeBuddyAgent(home).AddModels(context.Background(), agentconf.AddModelsRequest{}, dataDir)
	if !isAgentInvalid(err) {
		t.Errorf("期望 4403,实际 %v", err)
	}
	if got := backupCount(t, dataDir, "codebuddy"); got != 0 {
		t.Errorf("校验失败不得产生备份,实际 %d 份", got)
	}
}

// TestCodeBuddy_增删_文件缺失_4404 配置文件缺失时增删均报 4404。
func TestCodeBuddy_增删_文件缺失_4404(t *testing.T) {
	agent := NewCodeBuddyAgent(t.TempDir())
	if _, err := agent.RemoveModels(context.Background(), []agentconf.ModelRef{{ModelID: "m"}}, t.TempDir()); !isAgentNotFound(err) {
		t.Errorf("RemoveModels 期望 4404,实际 %v", err)
	}
	if _, err := agent.AddModels(context.Background(), agentconf.AddModelsRequest{ModelIDs: []string{"m"}}, t.TempDir()); !isAgentNotFound(err) {
		t.Errorf("AddModels 期望 4404,实际 %v", err)
	}
}

// TestCodeBuddy_增删_裸数组顶层_4402 维持既有口径:顶层裸数组不能当作
// CodeBuddy 配置编辑,增删均报 4402。
func TestCodeBuddy_增删_裸数组顶层_4402(t *testing.T) {
	home := t.TempDir()
	writeCodeBuddyFixture(t, home, `[{"id": "m", "name": "裸数组"}]`)
	agent := NewCodeBuddyAgent(home)
	if _, err := agent.RemoveModels(context.Background(), []agentconf.ModelRef{{ModelID: "m"}}, t.TempDir()); !isAgentFileIO(err) {
		t.Errorf("RemoveModels 期望 4402,实际 %v", err)
	}
	if _, err := agent.AddModels(context.Background(), agentconf.AddModelsRequest{ModelIDs: []string{"m"}}, t.TempDir()); !isAgentFileIO(err) {
		t.Errorf("AddModels 期望 4402,实际 %v", err)
	}
}
