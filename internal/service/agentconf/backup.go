// backup.go Agent 配置文件的备份与还原:写回前先复制一份带时间戳的副本到
// dataDir/agent-backups/<agentName>/,每个工具只保留最近 keepLatestN 份;
// 还原取最近一份整体覆盖回原路径。备份是纯文件复制,不解析内容、不触碰凭据。
package agentconf

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/acooler15/modelmeter/internal/apperr"
)

// keepLatestN 每个工具保留的备份份数上限,超过时从最旧开始清理。
const keepLatestN = 10

// backupTimestamp 备份文件名的时间戳格式:各段定宽,纳秒级精度,
// 保证字典序即时间序且同进程内连续备份不会重名。
const backupTimestamp = "20060102-150405.000000000"

// backupDirFor 备份目录:dataDir/agent-backups/<agentName>/。
func backupDirFor(dataDir, agentName string) string {
	return filepath.Join(dataDir, "agent-backups", agentName)
}

// Backup 写回前备份:把 srcPath 复制为 <agentName>/<时间戳>.bak 并清理超量,
// 返回备份文件路径。同一时间戳已存在时追加序号,避免覆盖既有备份。
func Backup(srcPath, agentName, dataDir string) (string, error) {
	dir := backupDirFor(dataDir, agentName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := time.Now().Format(backupTimestamp)
	dst := filepath.Join(dir, base+".bak")
	for i := 2; fileExists(dst); i++ {
		dst = filepath.Join(dir, base+"-"+strconv.Itoa(i)+".bak")
	}
	if err := copyFile(srcPath, dst); err != nil {
		return "", err
	}
	pruneBackups(dir)
	return dst, nil
}

// RestoreLatest 把 agentName 最近一份备份整体覆盖回 targetPath(配置文件被
// 连同目录误删时也会重建父目录)。返回所用备份路径;没有任何备份时报 4404。
func RestoreLatest(targetPath, agentName, dataDir string) (string, error) {
	dir := backupDirFor(dataDir, agentName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", apperr.New(apperr.CodeAgentNotFound, "暂无可还原的备份")
		}
		return "", apperr.Wrap(apperr.CodeAgentFileIO, "读取备份目录失败", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".bak") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return "", apperr.New(apperr.CodeAgentNotFound, "暂无可还原的备份")
	}
	// 文件名为定宽时间戳,字典序最大即最近一份
	sort.Strings(names)
	latest := filepath.Join(dir, names[len(names)-1])
	// 目标文件可能已被连同目录一起删除,先确保父目录存在
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", apperr.Wrap(apperr.CodeAgentFileIO, "还原备份失败", err)
	}
	if err := copyFile(latest, targetPath); err != nil {
		return "", apperr.Wrap(apperr.CodeAgentFileIO, "还原备份失败", err)
	}
	return latest, nil
}

// pruneBackups 只保留最近 keepLatestN 份备份;清理失败不影响备份主流程。
func pruneBackups(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // 备份目录读不到时无需清理,备份本身已成功
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".bak") {
			names = append(names, e.Name())
		}
	}
	if len(names) <= keepLatestN {
		return
	}
	// 字典序升序即时间升序,从最旧开始删
	sort.Strings(names)
	for _, name := range names[:len(names)-keepLatestN] {
		_ = os.Remove(filepath.Join(dir, name)) // 单个清理失败可容忍,下次写回会再次清理
	}
}

// copyFile 按字节复制文件内容,不解析、不改动。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close() // 只读句柄,关闭失败无实质影响
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close() // 复制已失败,优先返回复制阶段的错误
		return err
	}
	return out.Close()
}
