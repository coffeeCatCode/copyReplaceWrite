// Package backup 提供 crw 的版本文件底层能力：版本文件的发现、备份命名、
// 内容读写与去重比对。所谓"版本文件"指目标文件本身及其同目录下所有以
// 目标文件名作为前缀的文件（如 config.yaml、config.yaml.bak.20260816、
// config.yaml_back 等）。
//
// 本包不依赖任何 UI 组件，可独立单元测试。
package backup

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Entry 描述版本列表中的一项：目标文件本身，或某一个备份文件。
type Entry struct {
	Path    string    // 文件绝对路径
	Name    string    // 文件名（不含目录部分）
	IsBase  bool      // 是否为目标文件本身
	ModTime time.Time // 文件修改时间（用于排序）
	Size    int64     // 文件大小（字节）
}

// timeLayout 备份时间戳格式：yyyyMMdd_HHmmss，精确到秒。
// 秒级时间戳可保证日常使用中备份名不冲突（见 UniqueBackupPath 的兜底策略）。
const timeLayout = "20060102_150405"

// IsBackupName 判断 name 是否为 baseName 的备份文件名。
// 规则：以 baseName 为前缀且不等于 baseName 本身。
// 因此 config.yaml.bak.20260816、config.yaml_back、config.yaml.1 均被视为
// config.yaml 的备份；该规则同时覆盖 baseName 本身不存在的场景。
func IsBackupName(baseName, name string) bool {
	return name != baseName && strings.HasPrefix(name, baseName)
}

// List 扫描 basePath 所在目录，返回目标文件（若存在）与全部备份文件的 Entry 列表。
// 排序规则：目标文件恒在首位；备份文件按修改时间从新到旧排列，
// 同一秒内再按文件名倒序排列，保证排序结果稳定。
// 目录不存在、路径无法访问等错误会原样返回。
func List(basePath string) ([]Entry, error) {
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("解析路径失败: %w", err)
	}
	dir := filepath.Dir(abs)
	baseName := filepath.Base(abs)

	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("访问目录失败 %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("路径不是目录: %s", dir)
	}

	// 通配符 baseName* 同时匹配目标文件本身与全部前缀备份文件。
	matches, err := filepath.Glob(filepath.Join(dir, baseName+"*"))
	if err != nil {
		return nil, fmt.Errorf("扫描目录失败: %w", err)
	}

	entries := make([]Entry, 0, len(matches))
	for _, p := range matches {
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			// 忽略目录与无法访问的条目（如竞态下刚被删除的文件）。
			continue
		}
		name := filepath.Base(p)
		entries = append(entries, Entry{
			Path:    p,
			Name:    name,
			IsBase:  name == baseName,
			ModTime: fi.ModTime(),
			Size:    fi.Size(),
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsBase != entries[j].IsBase {
			return entries[i].IsBase // 目标文件恒在最前
		}
		if !entries[i].ModTime.Equal(entries[j].ModTime) {
			return entries[i].ModTime.After(entries[j].ModTime) // 新备份在前
		}
		return entries[i].Name > entries[j].Name
	})
	return entries, nil
}

// BackupName 生成备份文件名，如 config.yaml.bak.20260816_140000。
// 命名规则：<目标文件名>.bak.<秒级时间戳>。
func BackupName(baseName string, t time.Time) string {
	return fmt.Sprintf("%s.bak.%s", baseName, t.Format(timeLayout))
}

// UniqueBackupPath 返回一个当前不存在的备份文件绝对路径。
// 正常情况下秒级时间戳即可保证唯一；若极端情况下同一秒内已存在同名文件
// （并发创建或测试场景），则在时间戳后追加 _1、_2… 后缀避让，
// 保证绝不覆盖已有备份。最多尝试 10000 次，仍冲突则返回错误。
func UniqueBackupPath(basePath string, t time.Time) (string, error) {
	dir := filepath.Dir(basePath)
	baseName := filepath.Base(basePath)
	ts := t.Format(timeLayout)

	for i := range 10000 {
		var candidate string
		if i == 0 {
			candidate = filepath.Join(dir, BackupName(baseName, t))
		} else {
			candidate = filepath.Join(dir, fmt.Sprintf("%s.bak.%s_%d", baseName, ts, i))
		}
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("检查备份路径失败: %w", err)
		}
	}
	return "", fmt.Errorf("无法生成不冲突的备份文件名（尝试 %s* 均已被占用）", BackupName(baseName, t))
}

// ReadContent 读取文件全部内容。文件不存在时返回 (nil, nil)，调用方据此
// 区分"目标文件缺失"与"读取失败"，便于实现缺失文件跳过备份的语义。
func ReadContent(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取文件失败 %s: %w", path, err)
	}
	return data, nil
}

// WriteContent 将 data 写入 path（覆盖既有内容）。文件权限固定 0644。
func WriteContent(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入文件失败 %s: %w", path, err)
	}
	return nil
}

// Delete 删除指定备份文件。
func Delete(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("删除文件失败 %s: %w", path, err)
	}
	return nil
}

// FindIdenticalBackup 在 entries 中查找内容与 data 逐字节一致的备份项。
// 目标文件本身（IsBase）不参与比对，因为它就是当前内容本身。
// 找到返回该项指针；未找到返回 (nil, nil)；比对过程中任一文件读取失败
// 都会作为 error 返回，由调用方决定如何处理。
func FindIdenticalBackup(entries []Entry, data []byte) (*Entry, error) {
	for i := range entries {
		if entries[i].IsBase {
			continue
		}
		eq, err := contentEquals(entries[i].Path, data)
		if err != nil {
			return nil, err
		}
		if eq {
			return &entries[i], nil
		}
	}
	return nil, nil
}

// contentEquals 比较文件内容与 data 是否逐字节一致。
func contentEquals(path string, data []byte) (bool, error) {
	fileData, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("读取备份失败 %s: %w", path, err)
	}
	return bytes.Equal(fileData, data), nil
}
