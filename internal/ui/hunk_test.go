package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// testFileLines 生成 30 行内容，中间改 2 行（第 10、20 行），供 hunk 显示测试。
func testFileLines() (base, changed string) {
	var b, c strings.Builder
	for i := 1; i <= 30; i++ {
		line := "line"
		b.WriteString(line)
		c.WriteString(line)
		if i == 10 {
			c.WriteString(" CHANGED-A")
		}
		if i == 20 {
			c.WriteString(" CHANGED-B")
		}
		b.WriteString("\n")
		c.WriteString("\n")
	}
	return b.String(), c.String()
}

// TestSKeyToggleHunkView 验证对比模式下 s 键切换全文 ↔ hunk 显示。
func TestSKeyToggleHunkView(t *testing.T) {
	base, changed := testFileLines()
	m, _ := newTestModel(t, map[string]string{
		"config.yaml":              base,
		"config.yaml.bak.20260801": changed,
	})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	send(t, m, "f1") // 母本 = 目标文件
	send(t, m, "j")  // 切到改过的备份

	// 默认全文显示：30 行内容全部渲染，无省略标记。
	if m.hunkOnly {
		t.Fatal("默认应为全文显示")
	}
	view := m.preview.View()
	if strings.Contains(view, "省略") {
		t.Fatalf("全文显示不应出现省略标记, got:\n%s", view)
	}

	// 按 s → hunk 显示：出现省略标记，且不再显示被折叠的行。
	send(t, m, "s")
	if !m.hunkOnly {
		t.Fatal("按 s 后应切换为 hunk 显示")
	}
	view = m.preview.View()
	if !strings.Contains(view, "省略") {
		t.Fatalf("hunk 显示应出现省略标记, got:\n%s", view)
	}
	if !strings.Contains(view, "[hunk]") {
		t.Errorf("hunk 显示标题应带 [hunk] 标识, got:\n%s", view)
	}
	// 两处变更块折叠处省略 3 行（10→20 行间 9 行相同，前后各留 3 行）。
	if !strings.Contains(view, "省略 3 行") {
		t.Errorf("应折叠 3 行, got:\n%s", view)
	}

	// 再按 s → 恢复全文。
	send(t, m, "s")
	if m.hunkOnly {
		t.Fatal("再按 s 应恢复全文显示")
	}
	if strings.Contains(m.preview.View(), "省略") {
		t.Error("恢复全文后不应有省略标记")
	}
}

// TestSKeyIgnoredOutsideCompare 验证非对比模式下按 s 无副作用。
func TestSKeyIgnoredOutsideCompare(t *testing.T) {
	base, changed := testFileLines()
	m, _ := newTestModel(t, map[string]string{
		"config.yaml":              base,
		"config.yaml.bak.20260801": changed,
	})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	send(t, m, "j") // 常规模式下切到备份

	send(t, m, "s")
	if m.hunkOnly {
		t.Error("非对比模式下按 s 不应改变显示状态")
	}
	// 状态栏不应被 s 键改写（无提示动作）。
	if strings.Contains(m.status, "hunk") || strings.Contains(m.status, "全文") {
		t.Errorf("非对比模式按 s 不应产生状态提示, got %q", m.status)
	}
}

// TestHunkViewAfterEnteringCompareKeepsState 验证 hunk 状态在切换选中项后保持。
// 两个备份内容均与母本不同，且列表排序依赖文件写入顺序（map 迭代序），
// 故不假设具体排序，只验证：按 s 后切换任意条目，hunk 状态与渲染均保持。
func TestHunkViewAfterEnteringCompareKeepsState(t *testing.T) {
	base, changed := testFileLines()
	changed2 := strings.Replace(changed, "CHANGED-A", "CHANGED-C", 1)
	m, _ := newTestModel(t, map[string]string{
		"config.yaml":              base,
		"config.yaml.bak.20260801": changed,
		"config.yaml.bak.20260802": changed2,
	})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	send(t, m, "f1") // 母本 = 目标文件
	send(t, m, "s")  // 切 hunk（此时预览为无差异提示，状态切换仍生效）

	// 依次访问两个备份，hunk 状态都应保持，且差异渲染带 [hunk] 标识。
	send(t, m, "j")
	if !m.hunkOnly {
		t.Fatal("切 hunk 后切换选中项不应丢失状态")
	}
	if !strings.Contains(m.preview.View(), "[hunk]") {
		t.Errorf("备份 1 差异视图应带 [hunk] 标识, got:\n%s", m.preview.View())
	}
	send(t, m, "j")
	if !m.hunkOnly {
		t.Fatal("连续切换选中项后 hunk 状态应保持")
	}
	if !strings.Contains(m.preview.View(), "[hunk]") {
		t.Errorf("备份 2 差异视图应带 [hunk] 标识, got:\n%s", m.preview.View())
	}

	// 切回母本本身：无差异提示，但 hunk 状态依然保持。
	send(t, m, "k")
	send(t, m, "k")
	if !m.hunkOnly {
		t.Error("切回无差异条目后 hunk 状态应保持")
	}
}
