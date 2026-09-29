package agentconf

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/acooler15/modelmeter/internal/apperr"
)

// TestBackup_超量清理与还原 连续备份 12 次后只保留最近 10 份,
// 还原最近一份应得到最后一次写入的内容。
func TestBackup_超量清理与还原(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.json")
	dataDir := t.TempDir()
	for i := 0; i < 12; i++ {
		// 每轮改写内容,便于核对保留的是最新副本
		if err := os.WriteFile(src, []byte(strconv.Itoa(i)), 0o644); err != nil {
			t.Fatalf("改写源文件失败: %v", err)
		}
		if _, err := Backup(src, "zcode", dataDir); err != nil {
			t.Fatalf("备份失败: %v", err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(dataDir, "agent-backups", "zcode"))
	if err != nil {
		t.Fatalf("读取备份目录失败: %v", err)
	}
	if len(entries) != keepLatestN {
		t.Fatalf("应只保留 %d 份备份,实际 %d 份", keepLatestN, len(entries))
	}
	// 还原最近一份应得到最后一次写入的内容
	target := filepath.Join(t.TempDir(), "restored.json")
	if _, err := RestoreLatest(target, "zcode", dataDir); err != nil {
		t.Fatalf("还原失败: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("读取还原结果失败: %v", err)
	}
	if string(data) != "11" {
		t.Errorf("还原应得到最后一次备份的内容 11,实际 %q", data)
	}
}

// TestBackup_连续多次备份不覆盖 同一秒内连续备份不得互相覆盖。
func TestBackup_连续多次备份不覆盖(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.json")
	if err := os.WriteFile(src, []byte("v"), 0o644); err != nil {
		t.Fatalf("写入源文件失败: %v", err)
	}
	dataDir := t.TempDir()
	for i := 0; i < 3; i++ {
		if _, err := Backup(src, "zcode", dataDir); err != nil {
			t.Fatalf("第 %d 次备份失败: %v", i+1, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(dataDir, "agent-backups", "zcode"))
	if err != nil {
		t.Fatalf("读取备份目录失败: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("连续 3 次备份应产生 3 份文件,实际 %d 份", len(entries))
	}
}

// TestRestoreLatest_无备份_4404 没有任何备份时还原报 4404。
func TestRestoreLatest_无备份_4404(t *testing.T) {
	_, err := RestoreLatest(filepath.Join(t.TempDir(), "x.json"), "zcode", t.TempDir())
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != apperr.CodeAgentNotFound {
		t.Fatalf("期望 4404,实际 %v", err)
	}
}

// TestRestoreLatest_目标目录缺失可重建 目标文件连同目录被删后仍可还原。
func TestRestoreLatest_目标目录缺失可重建(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".zcode", "settings", "models.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatalf("写入原文件失败: %v", err)
	}
	dataDir := t.TempDir()
	if _, err := Backup(target, "zcode", dataDir); err != nil {
		t.Fatalf("备份失败: %v", err)
	}
	// 模拟误删:连目录一起删掉
	if err := os.RemoveAll(filepath.Join(home, ".zcode")); err != nil {
		t.Fatalf("删除目录失败: %v", err)
	}
	if _, err := RestoreLatest(target, "zcode", dataDir); err != nil {
		t.Fatalf("还原失败: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "original" {
		t.Errorf("还原应重建文件且内容一致,实际 data=%q err=%v", data, err)
	}
}
