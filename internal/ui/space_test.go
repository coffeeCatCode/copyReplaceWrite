package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestSpaceJumpToBaseAndBack 验证空格键：跳到母本 → 再按返回原位置。
func TestSpaceJumpToBaseAndBack(t *testing.T) {
	m, _ := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "old1",
		"config.yaml.bak.20260802": "old2",
	})
	// 光标下移两格到 20260801。
	send(t, m, "j")
	send(t, m, "j")
	origIdx := m.cursor
	if m.entries[origIdx].Name != "config.yaml.bak.20260801" {
		t.Fatalf("初始光标应在 20260801, got %q", m.entries[origIdx].Name)
	}

	// 空格 → 跳到母本（目标文件本身）。
	send(t, m, " ")
	if got := m.entries[m.cursor].Name; got != "config.yaml" {
		t.Fatalf("空格后应在母本, got %q", got)
	}
	if !m.entries[m.cursor].IsBase {
		t.Fatal("空格后光标应指向 IsBase 条目")
	}
	if m.lastPosPath == "" {
		t.Fatal("跳到母本前应记住原位置")
	}

	// 再按空格 → 返回原位置。
	send(t, m, " ")
	if m.cursor != origIdx {
		t.Fatalf("再按空格应返回原索引 %d, got %d", origIdx, m.cursor)
	}
	if m.lastPosPath != "" {
		t.Error("返回后待返回位置应清空")
	}
}

// TestSpaceAfterMoveJumpsBackToBase 验证跳到母本后移动光标，再按空格仍回母本。
func TestSpaceAfterMoveJumpsBackToBase(t *testing.T) {
	m, _ := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "old1",
		"config.yaml.bak.20260802": "old2",
	})
	send(t, m, "j")
	send(t, m, " ") // 跳到母本
	if !m.entries[m.cursor].IsBase {
		t.Fatal("应先跳到母本")
	}

	// 在母本处下移一格（离开母本），再按空格 → 应重新跳回母本。
	send(t, m, "j")
	send(t, m, " ")
	if !m.entries[m.cursor].IsBase {
		t.Fatal("移动后再按空格应跳回母本")
	}
}

// TestSpaceOnBaseWithoutPrev 验证在母本上按空格且无上次位置时给出提示。
func TestSpaceOnBaseWithoutPrev(t *testing.T) {
	m, _ := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "old1",
	})
	// 初始光标就在母本，无待返回位置。
	send(t, m, " ")
	if !m.entries[m.cursor].IsBase {
		t.Fatal("无上次位置时不应移动光标")
	}
	if !strings.Contains(m.status, "没有可返回") {
		t.Errorf("应提示没有可返回的位置, got %q", m.status)
	}
}

// TestSpaceJumpWorksInCompareMode 验证对比模式下空格往返后预览呈现差异。
func TestSpaceJumpWorksInCompareMode(t *testing.T) {
	m, _ := newTestModel(t, map[string]string{
		"config.yaml":              "aaa\nbbb\nccc\n",
		"config.yaml.bak.20260801": "aaa\nCHANGED\nccc\n",
	})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// 在备份上按空格跳到母本，F1 设母本，再按空格回到备份查看差异。
	send(t, m, "j")
	send(t, m, " ")
	send(t, m, "f1")
	if !m.compareMode || !m.compareBase.IsBase {
		t.Fatal("F1 后应处于对比模式且母本为目标文件本身")
	}
	send(t, m, " ")
	if m.entries[m.cursor].Name != "config.yaml.bak.20260801" {
		t.Fatalf("应返回备份条目, got %q", m.entries[m.cursor].Name)
	}
	view := m.preview.View()
	if !strings.Contains(view, "CHANGED") {
		t.Errorf("对比预览应包含差异内容, got:\n%s", view)
	}
}
