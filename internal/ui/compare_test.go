package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestCompareModeToggle 验证 F1 进入/退出对比模式，母本为进入时的选中项。
func TestCompareModeToggle(t *testing.T) {
	m, dir := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "old1",
		"config.yaml.bak.20260802": "old2",
	})

	// 初始为常规模式，预览显示目标文件内容。
	if m.compareMode {
		t.Fatal("初始不应处于对比模式")
	}

	// 光标移到第二个备份后按 F1 → 该备份成为母本。
	send(t, m, "j")
	send(t, m, "j")
	send(t, m, "f1")
	if !m.compareMode {
		t.Fatal("按 F1 后应进入对比模式")
	}
	if m.compareBase.Name != "config.yaml.bak.20260801" {
		t.Errorf("母本应为当前选中项, got %q", m.compareBase.Name)
	}
	_ = dir

	// 再按 F1 → 退出对比模式。
	send(t, m, "f1")
	if m.compareMode {
		t.Error("再次按 F1 应退出对比模式")
	}
}

// TestComparePreviewShowsDiff 验证对比模式下预览区展示与母本的行级差异。
func TestComparePreviewShowsDiff(t *testing.T) {
	m, dir := newTestModel(t, map[string]string{
		"config.yaml":              "aaa\nbbb\nccc\n",
		"config.yaml.bak.20260801": "aaa\nCHANGED\nccc\n",
	})
	// 单测未经过真实终端，需手动下发尺寸消息，预览区才有高度可渲染。
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// 进入对比模式：当前光标在目标文件（母本 = 目标文件）。
	send(t, m, "f1")
	if m.compareBase.Name != "config.yaml" {
		t.Fatalf("母本应为 config.yaml, got %q", m.compareBase.Name)
	}

	// 切到备份：预览应为 diff 视图，包含删除行与新增行。
	send(t, m, "j")
	if got := m.previewPath; got != filepath.Join(dir, "config.yaml.bak.20260801") {
		t.Errorf("预览路径应跟随选中项, got %q", got)
	}
	view := m.preview.View()
	if !strings.Contains(view, "+") || !strings.Contains(view, "-") {
		t.Errorf("对比预览应包含 +/- 差异行, got:\n%s", view)
	}
	if !strings.Contains(view, "CHANGED") {
		t.Errorf("对比预览应包含修改后的内容, got:\n%s", view)
	}
	if !strings.Contains(view, "1 行") {
		t.Errorf("对比预览应包含差异统计, got:\n%s", view)
	}
	// 标题应包含母本文件名（对比关系标识）。
	if !strings.Contains(m.previewView(), "config.yaml") {
		t.Error("对比模式标题应显示母本文件名")
	}
}

// TestCompareIdenticalShowsNoDiff 验证选中项与母本相同时显示无差异提示。
func TestCompareIdenticalShowsNoDiff(t *testing.T) {
	m, _ := newTestModel(t, map[string]string{
		"config.yaml":              "same\n",
		"config.yaml.bak.20260801": "same\n",
	})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	send(t, m, "f1") // 母本 = 目标文件
	send(t, m, "j")  // 切到内容相同的备份
	if !strings.Contains(m.preview.View(), "完全相同") {
		t.Errorf("相同内容应提示无差异, got:\n%s", m.preview.View())
	}
}

// TestCompareModeOperationsUnchanged 验证对比模式下业务操作（Enter 替换）语义不变。
func TestCompareModeOperationsUnchanged(t *testing.T) {
	m, _ := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "old1",
	})
	send(t, m, "f1") // 进入对比模式，母本 = 目标文件
	send(t, m, "j")  // 选中备份
	send(t, m, "enter")

	if got := readFileStr(t, m.basePath); got != "old1" {
		t.Errorf("对比模式下回车替换失败, got %q", got)
	}
	if !m.compareMode {
		t.Error("替换后应仍处于对比模式")
	}
}

// TestCompareBaseDeletedExitsMode 验证删除母本后自动退出对比模式。
func TestCompareBaseDeletedExitsMode(t *testing.T) {
	m, dir := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "old1",
		"config.yaml.bak.20260802": "old2",
	})
	send(t, m, "j")  // 光标 = 20260802
	send(t, m, "j")  // 光标 = 20260801
	send(t, m, "f1") // 母本 = 20260801
	if !m.compareMode {
		t.Fatal("应处于对比模式")
	}

	// 删除母本本身。
	send(t, m, "D")
	if m.compareMode {
		t.Error("删除母本后应自动退出对比模式")
	}
	if fileExists(t, filepath.Join(dir, "config.yaml.bak.20260801")) {
		t.Error("母本文件应已被删除")
	}
	if !strings.Contains(m.status, "已退出对比模式") {
		t.Errorf("应提示已退出对比模式, got %q", m.status)
	}
}

// TestCompareBaseMissingShowsHint 验证母本被外部删除时预览显示占位提示而非崩溃。
func TestCompareBaseMissingShowsHint(t *testing.T) {
	m, dir := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "old1",
	})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	send(t, m, "j")
	send(t, m, "f1") // 母本 = 备份
	// 外部删除母本文件后切换选中项触发重新加载。
	if err := os.Remove(filepath.Join(dir, "config.yaml.bak.20260801")); err != nil {
		t.Fatalf("外部删除失败: %v", err)
	}
	send(t, m, "k") // 移回目标文件，触发 loadDiffPreview

	if !strings.Contains(m.preview.View(), "母本文件已不存在") {
		t.Errorf("母本缺失应显示占位提示, got:\n%s", m.preview.View())
	}
}
