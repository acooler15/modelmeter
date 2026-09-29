// zcode.go ZCode CLI 模型配置适配:读 ~/.zcode/cli-bin/ 下的当前模型选择,
// 写回仅限 model-selection.json 的 providerId/modelId 两个键,options 与其他
// 未知键原样保留。供应商清单只解析 providerId/providerName 两个非敏感字段
// 用于展示;provider_config.cli.json 含明文 API Key,本实现绝不读取其凭据
// 字段、绝不写回该文件,供应商与密钥一律引导用户到 ZCode 原生工具管理。
package agentconf

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/acooler15/modelmeter/internal/apperr"
)

// ZCodeAgent ZCode 的 Agent 接口实现;homeDir 构造注入,便于测试。
type ZCodeAgent struct {
	homeDir string
}

// NewZCodeAgent 构造 ZCode 适配器;homeDir 为用户主目录(对应 ~/.zcode)。
func NewZCodeAgent(homeDir string) *ZCodeAgent {
	return &ZCodeAgent{homeDir: homeDir}
}

// Name 实现 Agent 接口;同时用作 URL 标识与备份目录名。
func (z *ZCodeAgent) Name() string { return "zcode" }

// DisplayName 实现 Agent 接口;界面展示名。
func (z *ZCodeAgent) DisplayName() string { return "ZCode" }

// selectionPath 当前模型选择文件路径,是本实现唯一的写回目标。
func (z *ZCodeAgent) selectionPath() string {
	return filepath.Join(z.homeDir, ".zcode", "cli-bin", "model-selection.json")
}

// providerConfigPath 供应商定义文件路径;只读且只取非敏感字段。
func (z *ZCodeAgent) providerConfigPath() string {
	return filepath.Join(z.homeDir, ".zcode", "cli-bin", "provider_config.cli.json")
}

// manageHint 供应商与密钥的管理边界提示,卡片说明与字段 Help 共用。
const manageHint = "供应商与 API Key 请在 ZCode 原生工具中管理,本页仅切换当前模型"

// selectionFile model-selection.json 中需要读取的字段;写回时其余键以
// map[string]json.RawMessage 形式整体保留。
type selectionFile struct {
	ProviderID string `json:"providerId"`
	ModelID    string `json:"modelId"`
}

// providerConfigFile provider_config.cli.json 中参与解析的字段。刻意不声明
// access/apiKey 等凭据字段:json.Unmarshal 忽略未声明的键,解析结果因此
// 永不可能包含凭据;该文件也绝不用于写回。
type providerConfigFile struct {
	Config struct {
		ProviderOrder       []string `json:"providerOrder"`
		ProviderConfigRules struct {
			ProviderRules []struct {
				ProviderID   string `json:"providerId"`
				ProviderName string `json:"providerName"`
			} `json:"providerRules"`
		} `json:"providerConfigRules"`
	} `json:"config"`
}

// Snapshot 实现 Agent 接口:返回当前模型选择与可选供应商清单。
// 两个配置文件任一缺失返回 not_found;模型选择文件内容损坏时降级为空值
// 并给出指引,不返回错误。
func (z *ZCodeAgent) Snapshot(_ context.Context) Snapshot {
	selPath := z.selectionPath()
	if !fileExists(selPath) || !fileExists(z.providerConfigPath()) {
		return Snapshot{
			Name:        z.Name(),
			DisplayName: z.DisplayName(),
			Status:      StatusNotFound,
			ConfigPath:  selPath,
			Fields:      []FieldSpec{},
			Values:      map[string]string{},
			Message: "未找到 ZCode 配置文件(~/.zcode/cli-bin/ 下需要同时存在 model-selection.json 与 " +
				"provider_config.cli.json),请确认 ZCode 已安装并至少运行过一次",
		}
	}
	values := map[string]string{}
	message := manageHint
	var sel selectionFile
	raw, err := os.ReadFile(selPath)
	if err == nil {
		err = json.Unmarshal(raw, &sel)
	}
	if err != nil {
		// 内容损坏时按空值降级;真正写回时会给出的 4402 与此处提示口径一致
		slog.Warn("ZCode 模型选择文件解析失败", "path", selPath, "err", err)
		message = "模型选择配置文件存在但不是合法 JSON,请先在 ZCode 原生工具中修复后再试"
	} else {
		values["providerId"] = sel.ProviderID
		values["modelId"] = sel.ModelID
	}
	return Snapshot{
		Name:        z.Name(),
		DisplayName: z.DisplayName(),
		Status:      StatusFound,
		ConfigPath:  selPath,
		Fields:      z.fields(),
		Values:      values,
		Message:     message,
	}
}

// fields 构造可编辑字段清单:providerId(供应商下拉,清单解析失败时降级为
// 手输文本)+ modelId(模型 ID 文本)。
func (z *ZCodeAgent) fields() []FieldSpec {
	options := z.providerOptions()
	providerField := FieldSpec{
		Key:      "providerId",
		Label:    "供应商",
		Required: true,
		Help:     manageHint,
	}
	if len(options) == 0 {
		providerField.Type = "text"
		providerField.Help = "未能解析出供应商清单,请参照 ZCode 原生配置手动输入 providerId。" + manageHint
	} else {
		providerField.Type = "select"
		providerField.Options = options
	}
	return []FieldSpec{providerField, {
		Key:      "modelId",
		Label:    "模型 ID",
		Type:     "text",
		Required: true,
		Help:     "当前使用的模型 ID,需与所选供应商提供的模型名称一致",
	}}
}

// providerOptions 解析可选供应商清单:providerOrder 定序,providerRules 补充
// 展示名;只取 providerId/providerName,输出顺序稳定。文件缺失或解析失败
// 返回 nil,由调用方降级处理。
func (z *ZCodeAgent) providerOptions() []FieldOption {
	raw, err := os.ReadFile(z.providerConfigPath())
	if err != nil {
		return nil
	}
	var cfg providerConfigFile
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil
	}
	// 展示名映射:规则里没有名称的供应商回退为 providerId 本身
	names := make(map[string]string)
	for _, rule := range cfg.Config.ProviderConfigRules.ProviderRules {
		if rule.ProviderID == "" {
			continue
		}
		if rule.ProviderName != "" {
			names[rule.ProviderID] = rule.ProviderName
		} else if _, ok := names[rule.ProviderID]; !ok {
			names[rule.ProviderID] = rule.ProviderID
		}
	}
	ordered := make([]FieldOption, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, id := range cfg.Config.ProviderOrder {
		if id == "" || seen[id] {
			continue
		}
		label, ok := names[id]
		if !ok {
			label = id
		}
		ordered = append(ordered, FieldOption{Value: id, Label: label})
		seen[id] = true
	}
	// 规则里有但 providerOrder 未列出的供应商补充在尾部,按 id 排序保证稳定
	extra := make([]string, 0, len(names))
	for id := range names {
		if !seen[id] {
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	for _, id := range extra {
		ordered = append(ordered, FieldOption{Value: id, Label: names[id]})
	}
	return ordered
}

// Apply 实现 Agent 接口:校验提交值 → 备份原文件 → 只改 providerId/modelId
// 两个键 → 以 2 空格缩进写回(options 与未知键的值语义保留,顶层键名按字母
// 序重排,JSON 语义不变)。
func (z *ZCodeAgent) Apply(ctx context.Context, values map[string]string, dataDir string) (Snapshot, error) {
	providerID := strings.TrimSpace(values["providerId"])
	modelID := strings.TrimSpace(values["modelId"])
	if providerID == "" {
		return Snapshot{}, apperr.New(apperr.CodeAgentInvalid, "供应商 providerId 不能为空")
	}
	if modelID == "" {
		return Snapshot{}, apperr.New(apperr.CodeAgentInvalid, "模型 ID modelId 不能为空")
	}
	// 能解析出供应商清单时,提交值必须在清单内;解析失败则不设限,
	// 与 Snapshot 的降级行为保持一致
	if options := z.providerOptions(); len(options) > 0 && !containsOption(options, providerID) {
		return Snapshot{}, apperr.New(apperr.CodeAgentInvalid, "providerId 不在可选供应商列表中,请刷新后重新选择")
	}
	selPath := z.selectionPath()
	if !fileExists(selPath) {
		return Snapshot{}, apperr.New(apperr.CodeAgentNotFound, "未找到 ZCode 配置文件,无法写回")
	}
	// 备份先于写回:任何写回动作前必须先留一份可还原的副本
	if _, err := Backup(selPath, z.Name(), dataDir); err != nil {
		return Snapshot{}, apperr.Wrap(apperr.CodeAgentFileIO, "备份 ZCode 配置失败", err)
	}
	raw, err := os.ReadFile(selPath)
	if err != nil {
		return Snapshot{}, apperr.Wrap(apperr.CodeAgentFileIO, "读取 ZCode 配置失败", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Snapshot{}, apperr.Wrap(apperr.CodeAgentFileIO, "ZCode 配置文件不是合法 JSON,请先在 ZCode 原生工具中修复", err)
	}
	if doc == nil {
		doc = make(map[string]json.RawMessage)
	}
	pid, err := marshalPlainString(providerID)
	if err != nil {
		return Snapshot{}, apperr.Wrap(apperr.CodeAgentFileIO, "编码 ZCode 配置失败", err)
	}
	mid, err := marshalPlainString(modelID)
	if err != nil {
		return Snapshot{}, apperr.Wrap(apperr.CodeAgentFileIO, "编码 ZCode 配置失败", err)
	}
	doc["providerId"] = pid
	doc["modelId"] = mid
	if err := writeJSONFile(selPath, doc); err != nil {
		return Snapshot{}, apperr.Wrap(apperr.CodeAgentFileIO, "写回 ZCode 配置失败", err)
	}
	// 日志只记模型选择结果,不含任何凭据字段
	slog.Info("ZCode 模型选择已更新", "provider_id", providerID, "model_id", modelID)
	return z.Snapshot(ctx), nil
}

// containsOption 判断提交值是否在选项清单内。
func containsOption(options []FieldOption, value string) bool {
	for _, opt := range options {
		if opt.Value == value {
			return true
		}
	}
	return false
}

// marshalPlainString 把字符串序列化为不带 HTML 转义的 JSON 字符串值,
// 避免配置文件里的普通字符被写成 \u003c 这类转义形式。
func marshalPlainString(s string) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n")), nil
}

// writeJSONFile 以 2 空格缩进、不转义 HTML 的格式写 JSON 文件,保持文件可读。
// 先写同目录临时文件再改名替换:os.WriteFile 会先截断原文件,写回中途失败
// (磁盘满、进程被杀)会把原配置破坏成半截内容;改名在同一目录内完成,
// 原文件要么是旧内容要么是完整新内容,不出现中间态。
func writeJSONFile(path string, v any) error {
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
