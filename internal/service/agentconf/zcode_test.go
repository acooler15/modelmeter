package agentconf

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acooler15/modelmeter/internal/apperr"
)

// 测试夹具:伪造的 model-selection.json,含 options 与未知键用于校验保留语义。
const testSelection = `{"providerId":"p1","modelId":"glm-4","options":{"theme":"dark","nested":{"a":1}},"unknownKey":[1,2,3]}`

// 测试夹具:伪造的 provider_config.cli.json;providerRules 中刻意放置
// apiKey 字段,实现若解析/输出凭据即视为违反安全红线。
const testProviders = `{"schemaVersion":1,"config":{"providerOrder":["p1","p2"],"providerConfigRules":{"providerRules":[{"providerId":"p1","providerName":"供应商一","config":{}},{"providerId":"p2","providerName":"供应商二","access":{"apiKey":"sk-should-never-leak"}}]}}}`

// writeZCodeFixture 在临时 home 下伪造 ZCode 的两个配置文件。
func writeZCodeFixture(t *testing.T, home, selection, providers string) {
	t.Helper()
	dir := filepath.Join(home, ".zcode", "cli-bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("创建伪造配置目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "model-selection.json"), []byte(selection), 0o644); err != nil {
		t.Fatalf("写入伪造 model-selection.json 失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "provider_config.cli.json"), []byte(providers), 0o644); err != nil {
		t.Fatalf("写入伪造 provider_config.cli.json 失败: %v", err)
	}
}

// findField 按字段名取字段描述,取不到时直接判失败。
func findField(t *testing.T, fields []FieldSpec, key string) FieldSpec {
	t.Helper()
	for _, f := range fields {
		if f.Key == key {
			return f
		}
	}
	t.Fatalf("字段 %s 不存在", key)
	return FieldSpec{}
}

// TestZCode_Snapshot_正常解析 验证 found 状态、当前值与供应商选项解析。
func TestZCode_Snapshot_正常解析(t *testing.T) {
	home := t.TempDir()
	writeZCodeFixture(t, home, testSelection, testProviders)

	snap := NewZCodeAgent(home).Snapshot(context.Background())
	if snap.Status != StatusFound {
		t.Fatalf("期望状态 found,实际 %s", snap.Status)
	}
	if snap.Values["providerId"] != "p1" || snap.Values["modelId"] != "glm-4" {
		t.Errorf("当前值解析错误,实际 %v", snap.Values)
	}
	provider := findField(t, snap.Fields, "providerId")
	if provider.Type != "select" || len(provider.Options) != 2 {
		t.Fatalf("providerId 应为含 2 个选项的下拉,实际 type=%s options=%d", provider.Type, len(provider.Options))
	}
	// 选项按 providerOrder 定序,展示名来自 providerRules
	if provider.Options[0].Value != "p1" || provider.Options[0].Label != "供应商一" {
		t.Errorf("第一个选项解析错误,实际 %+v", provider.Options[0])
	}
	if provider.Options[1].Value != "p2" || provider.Options[1].Label != "供应商二" {
		t.Errorf("第二个选项解析错误,实际 %+v", provider.Options[1])
	}
	// 快照里不允许出现夹具中的凭据内容
	if strings.Contains(snap.Message, "sk-should-never-leak") {
		t.Error("快照不得包含凭据内容")
	}
}

// TestZCode_Snapshot_文件缺失_not_found 任一配置文件缺失时降级为 not_found。
func TestZCode_Snapshot_文件缺失_not_found(t *testing.T) {
	home := t.TempDir()
	snap := NewZCodeAgent(home).Snapshot(context.Background())
	if snap.Status != StatusNotFound || snap.Message == "" {
		t.Fatalf("文件缺失应返回 not_found 且带指引,实际 %+v", snap)
	}
}

// TestZCode_Apply_写回只改两个键 验证 providerId/modelId 更新,
// options 与未知键保留,写回为 2 空格缩进,且自动生成备份。
func TestZCode_Apply_写回只改两个键(t *testing.T) {
	home := t.TempDir()
	dataDir := t.TempDir()
	writeZCodeFixture(t, home, testSelection, testProviders)

	snap, err := NewZCodeAgent(home).Apply(context.Background(),
		map[string]string{"providerId": "p2", "modelId": "glm-5"}, dataDir)
	if err != nil {
		t.Fatalf("写回失败: %v", err)
	}
	if snap.Values["providerId"] != "p2" || snap.Values["modelId"] != "glm-5" {
		t.Fatalf("写回后的视图应反映新值,实际 %v", snap.Values)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".zcode", "cli-bin", "model-selection.json"))
	if err != nil {
		t.Fatalf("读取写回后的文件失败: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("写回后的文件不是合法 JSON: %v", err)
	}
	if doc["providerId"] != "p2" || doc["modelId"] != "glm-5" {
		t.Errorf("providerId/modelId 应更新,实际 %v", doc)
	}
	opts, _ := doc["options"].(map[string]any)
	if opts == nil || opts["theme"] != "dark" {
		t.Errorf("options 应原样保留,实际 %v", doc["options"])
	}
	unknown, _ := doc["unknownKey"].([]any)
	if len(unknown) != 3 {
		t.Errorf("未知键应原样保留,实际 %v", doc["unknownKey"])
	}
	if !strings.Contains(string(raw), "\n  \"providerId\"") {
		t.Errorf("写回应为 2 空格缩进,实际:\n%s", raw)
	}
	// 写回前应自动生成一份备份
	entries, err := os.ReadDir(filepath.Join(dataDir, "agent-backups", "zcode"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("写回应生成 1 份备份,实际 err=%v entries=%d", err, len(entries))
	}
}

// TestZCode_Apply_文件缺失_4404 配置文件不存在时报 4404。
func TestZCode_Apply_文件缺失_4404(t *testing.T) {
	_, err := NewZCodeAgent(t.TempDir()).Apply(context.Background(),
		map[string]string{"providerId": "p1", "modelId": "m"}, t.TempDir())
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != apperr.CodeAgentNotFound {
		t.Fatalf("期望 4404,实际 %v", err)
	}
}

// TestZCode_Apply_非法JSON_4402 配置文件不是合法 JSON 时报 4402。
func TestZCode_Apply_非法JSON_4402(t *testing.T) {
	home := t.TempDir()
	writeZCodeFixture(t, home, "这不是 JSON", testProviders)
	_, err := NewZCodeAgent(home).Apply(context.Background(),
		map[string]string{"providerId": "p1", "modelId": "m"}, t.TempDir())
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != apperr.CodeAgentFileIO {
		t.Fatalf("期望 4402,实际 %v", err)
	}
}

// TestZCode_Apply_非法值_4403 必填缺失或不在选项内时报 4403。
func TestZCode_Apply_非法值_4403(t *testing.T) {
	home := t.TempDir()
	writeZCodeFixture(t, home, testSelection, testProviders)
	cases := []struct {
		name   string
		values map[string]string
	}{
		{"供应商为空", map[string]string{"providerId": " ", "modelId": "m"}},
		{"模型为空", map[string]string{"providerId": "p1", "modelId": ""}},
		{"供应商不在选项内", map[string]string{"providerId": "p9", "modelId": "m"}},
	}
	for _, tc := range cases {
		_, err := NewZCodeAgent(home).Apply(context.Background(), tc.values, t.TempDir())
		var ae *apperr.Error
		if !errors.As(err, &ae) || ae.Code != apperr.CodeAgentInvalid {
			t.Errorf("%s: 期望 4403,实际 %v", tc.name, err)
		}
	}
}
