package backup

import (
	"fmt"
	"strings"
	"testing"
)

// buildLines 构造一个"上下文为主、中间改一行"的大 diff 输入：
// 总行数 total，第 mid 行（1 起始）改为 added+removed 对。
func buildLines(total, mid int) []DiffLine {
	lines := make([]DiffLine, 0, total+1)
	for i := 1; i <= total; i++ {
		if i == mid {
			lines = append(lines, DiffLine{Kind: DiffRemoved, Text: "old", NumA: i})
			lines = append(lines, DiffLine{Kind: DiffAdded, Text: "new", NumB: i})
			continue
		}
		lines = append(lines, DiffLine{Kind: DiffContext, Text: fmt.Sprintf("L%d", i), NumA: i, NumB: i})
	}
	return lines
}

// kindSummary 输出紧凑类型序列：= 上下文、+ 新增、- 删除、⋮ gap。
func kindSummary(lines []DiffLine) string {
	var sb strings.Builder
	for _, l := range lines {
		switch l.Kind {
		case DiffContext:
			sb.WriteString("=")
		case DiffAdded:
			sb.WriteString("+")
		case DiffRemoved:
			sb.WriteString("-")
		case DiffHunkGap:
			sb.WriteString("⋮")
		}
	}
	return sb.String()
}

func TestDiffHunksMiddleChange(t *testing.T) {
	// 20 行文件中间改 1 行，ctx=3：头部丢 6 行，尾部丢 10 行，
	// 中间无 gap（变更块只有一处），保留 3 上下文 + 1 删 + 1 增 + 3 上下文。
	lines := DiffHunks(buildLines(20, 10), 3)
	got := kindSummary(lines)
	want := "===-+==="
	if got != want {
		t.Errorf("hunk 类型序列 = %q, want %q", got, want)
	}
	if len(lines) != 8 {
		t.Errorf("hunk 行数 = %d, want 8", len(lines))
	}
	// 保留行应为第 7~13 行。
	if lines[0].Text != "L7" || lines[len(lines)-1].Text != "L13" {
		t.Errorf("上下文范围错误: 首 %q 末 %q, want L7/L13", lines[0].Text, lines[len(lines)-1].Text)
	}
}

func TestDiffHunksTwoBlocksWithGap(t *testing.T) {
	// 两个变更块直接拼接：块间 39 行相同内容（块1尾部 20 行 + 块2头部 19 行），
	// 前向/后向各保留 3 行上下文后折叠为 gap = 39 - 3 - 3 = 33 行。
	lines := buildLines(30, 10)
	lines = append(lines, buildLines(30, 20)...)
	got := DiffHunks(lines, 3)
	seq := kindSummary(got)
	if !strings.Contains(seq, "⋮") {
		t.Errorf("两变更块之间应折叠出 gap, 序列 %q", seq)
	}
	gaps := 0
	for _, l := range got {
		if l.Kind == DiffHunkGap {
			gaps++
			if l.NumA != 33 {
				t.Errorf("gap 折叠行数 = %d, want 33", l.NumA)
			}
		}
	}
	if gaps != 1 {
		t.Errorf("gap 数量 = %d, want 1", gaps)
	}
}

func TestDiffHunksNoChangeReturnsNil(t *testing.T) {
	lines := make([]DiffLine, 5)
	for i := range lines {
		lines[i] = DiffLine{Kind: DiffContext, Text: "x"}
	}
	if got := DiffHunks(lines, 3); got != nil {
		t.Errorf("无变更输入应返回 nil, got %v", got)
	}
}

func TestDiffHunksHeadTailDropped(t *testing.T) {
	// 10 行文件改第 1 行：头部无丢弃、尾部 8 行丢弃，无 gap。
	got := kindSummary(DiffHunks(buildLines(10, 1), 3))
	want := "-+==="
	if got != want {
		t.Errorf("首行修改 hunk = %q, want %q", got, want)
	}
	// 改最后一行：尾部无丢弃，头部 8 行丢弃，无 gap。
	got = kindSummary(DiffHunks(buildLines(10, 10), 3))
	want = "===-+"
	if got != want {
		t.Errorf("末行修改 hunk = %q, want %q", got, want)
	}
}

func TestDiffHunksZeroContext(t *testing.T) {
	// ctx=0：仅保留变更行本身，相邻变更间 gap 折叠全部中间相同行。
	lines := buildLines(30, 10)
	lines = append(lines, buildLines(30, 20)...)
	got := DiffHunks(lines, 0)
	seq := kindSummary(got)
	if seq != "-+⋮-+" {
		t.Errorf("ctx=0 序列 = %q, want %q", seq, "-+⋮-+")
	}
}

func TestDiffHunksNegativeContext(t *testing.T) {
	lines := buildLines(20, 10)
	if got := DiffHunks(lines, -5); kindSummary(got) != "-+" {
		t.Errorf("负 ctx 应按 0 处理, got %q", kindSummary(got))
	}
}
