package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTempDir 创建测试用临时目录，并在测试结束时清理。
func newTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

// touchFile 创建文件并写入内容。
func touchFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("创建文件失败 %s: %v", path, err)
	}
}

// setMTime 显式设置文件修改时间（用于验证排序）。
func setMTime(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("设置修改时间失败 %s: %v", path, err)
	}
}

func TestIsBackupName(t *testing.T) {
	cases := []struct {
		base, name string
		want       bool
	}{
		{"config.yaml", "config.yaml", false},                    // 自身不是备份
		{"config.yaml", "config.yaml.bak.20260816_140000", true}, // 点分备份
		{"config.yaml", "config.yaml_back", true},                // 下划线备份
		{"config.yaml", "config.yaml.bak", true},                 // 无时间戳备份
		{"config.yaml", "config.yaml.1", true},                   // 序号备份
		{"config.yaml", "conf.yaml.bak", false},                  // 不同前缀
		{"Makefile", "Makefile.bak.20260816_140000", true},       // 无扩展名目标
		{"Makefile", "Makefile", false},                          // 自身不是备份
		{"config.yaml", "config.yaml_backup_dir", true},          // 前缀覆盖一切（文档化行为）
	}
	for _, c := range cases {
		if got := IsBackupName(c.base, c.name); got != c.want {
			t.Errorf("IsBackupName(%q, %q) = %v, want %v", c.base, c.name, got, c.want)
		}
	}
}

func TestBackupNameFormat(t *testing.T) {
	ts := time.Date(2026, 8, 16, 14, 0, 0, 0, time.Local)
	got := BackupName("config.yaml", ts)
	want := "config.yaml.bak.20260816_140000"
	if got != want {
		t.Errorf("BackupName() = %q, want %q", got, want)
	}
}

func TestListOrderAndBaseFirst(t *testing.T) {
	dir := newTempDir(t)
	base := filepath.Join(dir, "config.yaml")
	old := filepath.Join(dir, "config.yaml.bak.20260801")
	newer := filepath.Join(dir, "config.yaml.bak.20260816_140000")
	unrelated := filepath.Join(dir, "other.txt")

	touchFile(t, base, "current")
	touchFile(t, old, "old")
	touchFile(t, newer, "newer")
	touchFile(t, unrelated, "unrelated")

	baseT := time.Date(2026, 8, 10, 0, 0, 0, 0, time.Local)
	oldT := time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)
	newT := time.Date(2026, 8, 16, 14, 0, 0, 0, time.Local)
	setMTime(t, base, baseT)
	setMTime(t, old, oldT)
	setMTime(t, newer, newT)

	entries, err := List(base)
	if err != nil {
		t.Fatalf("List() 出错: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("条目数 = %d, want 3（unrelated.txt 不应被列出）", len(entries))
	}
	// 目标文件恒在首位。
	if !entries[0].IsBase || entries[0].Name != "config.yaml" {
		t.Errorf("首位应是目标文件本身, got %+v", entries[0])
	}
	// 备份按修改时间从新到旧。
	if entries[1].Name != "config.yaml.bak.20260816_140000" {
		t.Errorf("第二位应是最新备份, got %q", entries[1].Name)
	}
	if entries[2].Name != "config.yaml.bak.20260801" {
		t.Errorf("第三位应是最旧备份, got %q", entries[2].Name)
	}
}

func TestListMissingDir(t *testing.T) {
	if _, err := List(filepath.Join(t.TempDir(), "no_such_dir", "config.yaml")); err == nil {
		t.Error("目录不存在时应返回错误")
	}
}

func TestUniqueBackupPathNoCollision(t *testing.T) {
	dir := newTempDir(t)
	base := filepath.Join(dir, "config.yaml")
	ts := time.Date(2026, 8, 16, 14, 0, 0, 0, time.Local)

	got, err := UniqueBackupPath(base, ts)
	if err != nil {
		t.Fatalf("UniqueBackupPath() 出错: %v", err)
	}
	want := filepath.Join(dir, "config.yaml.bak.20260816_140000")
	if got != want {
		t.Errorf("UniqueBackupPath() = %q, want %q", got, want)
	}
}

func TestUniqueBackupPathCollisionSuffix(t *testing.T) {
	dir := newTempDir(t)
	base := filepath.Join(dir, "config.yaml")
	ts := time.Date(2026, 8, 16, 14, 0, 0, 0, time.Local)

	// 预先占满 主名 与 _1，应返回 _2。
	touchFile(t, filepath.Join(dir, "config.yaml.bak.20260816_140000"), "a")
	touchFile(t, filepath.Join(dir, "config.yaml.bak.20260816_140000_1"), "b")

	got, err := UniqueBackupPath(base, ts)
	if err != nil {
		t.Fatalf("UniqueBackupPath() 出错: %v", err)
	}
	if !strings.HasSuffix(got, "_2") {
		t.Errorf("应返回 _2 后缀避让路径, got %q", got)
	}
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Errorf("返回的路径不应已存在: %q", got)
	}
}

func TestReadContentMissingReturnsNil(t *testing.T) {
	dir := newTempDir(t)
	data, err := ReadContent(filepath.Join(dir, "not_exist.yaml"))
	if err != nil {
		t.Fatalf("缺失文件应返回 nil, 得到错误: %v", err)
	}
	if data != nil {
		t.Errorf("缺失文件应返回 nil 内容, got %q", data)
	}
}

func TestWriteAndReadContent(t *testing.T) {
	dir := newTempDir(t)
	path := filepath.Join(dir, "config.yaml")
	if err := WriteContent(path, []byte("hello\nworld")); err != nil {
		t.Fatalf("WriteContent() 出错: %v", err)
	}
	data, err := ReadContent(path)
	if err != nil {
		t.Fatalf("ReadContent() 出错: %v", err)
	}
	if string(data) != "hello\nworld" {
		t.Errorf("读回内容 = %q, want %q", data, "hello\nworld")
	}
}

func TestFindIdenticalBackup(t *testing.T) {
	dir := newTempDir(t)
	base := filepath.Join(dir, "config.yaml")
	b1 := filepath.Join(dir, "config.yaml.bak.20260801")
	b2 := filepath.Join(dir, "config.yaml.bak.20260802")
	touchFile(t, base, "current")
	touchFile(t, b1, "current") // 与当前内容相同
	touchFile(t, b2, "old")

	entries, err := List(base)
	if err != nil {
		t.Fatalf("List() 出错: %v", err)
	}

	// 当前内容在备份中已存在 → 应命中 b1。
	found, err := FindIdenticalBackup(entries, []byte("current"))
	if err != nil {
		t.Fatalf("FindIdenticalBackup() 出错: %v", err)
	}
	if found == nil || found.Path != b1 {
		t.Errorf("应命中 %q, got %+v", b1, found)
	}

	// 当前内容在备份中不存在 → 应返回 nil。
	found, err = FindIdenticalBackup(entries, []byte("brand-new"))
	if err != nil {
		t.Fatalf("FindIdenticalBackup() 出错: %v", err)
	}
	if found != nil {
		t.Errorf("应返回 nil, got %+v", found)
	}
}

func TestDelete(t *testing.T) {
	dir := newTempDir(t)
	path := filepath.Join(dir, "config.yaml.bak.20260801")
	touchFile(t, path, "x")
	if err := Delete(path); err != nil {
		t.Fatalf("Delete() 出错: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("删除后文件仍存在: %q", path)
	}
}
