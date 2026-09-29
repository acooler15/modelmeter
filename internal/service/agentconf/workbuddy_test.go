package agentconf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/acooler15/modelmeter/internal/apperr"
)

// TestWorkBuddy_未找到 候选路径全部不存在时返回 not_found 与中文指引,
// Apply 报 4404。
func TestWorkBuddy_未找到(t *testing.T) {
	agent := NewWorkBuddyAgent(t.TempDir())
	snap := agent.Snapshot(context.Background())
	if snap.Status != StatusNotFound || snap.Message == "" {
		t.Fatalf("未找到时应返回 not_found 且带指引,实际 %+v", snap)
	}
	if snap.ConfigPath != "" {
		t.Errorf("未找到时 config_path 应为空,实际 %q", snap.ConfigPath)
	}
	_, err := agent.Apply(context.Background(), map[string]string{}, t.TempDir())
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != apperr.CodeAgentNotFound {
		t.Fatalf("期望 4404,实际 %v", err)
	}
}

// TestWorkBuddy_候选路径命中 把系统配置/缓存目录环境变量指向临时目录,
// 验证候选路径探测逻辑(AppData/XDG 双设置保证跨平台可跑)。
func TestWorkBuddy_候选路径命中(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, "AppData", "Roaming")
	cacheDir := filepath.Join(home, "AppData", "Local")
	t.Setenv("AppData", configDir)
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("LocalAppData", cacheDir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	target := filepath.Join(configDir, "WorkBuddy", "config.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("创建伪造配置目录失败: %v", err)
	}
	if err := os.WriteFile(target, []byte("{}"), 0o644); err != nil {
		t.Fatalf("写入伪造配置失败: %v", err)
	}
	snap := NewWorkBuddyAgent(home).Snapshot(context.Background())
	if snap.Status != StatusFound || snap.ConfigPath != target {
		t.Fatalf("应命中候选路径 %s,实际 %+v", target, snap)
	}
}
