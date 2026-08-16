package backup

import (
	"strings"
	"testing"
)

// summary 将 diff 结果压缩成便于断言的紧凑描述，如 `=a +b -c`：
// = 上下文、+ 新增（内容）、- 删除（内容），按顺序拼接。
func summary(lines []DiffLine) string {
	var sb strings.Builder
	for _, l := range lines {
		switch l.Kind {
		case DiffContext:
			sb.WriteString("=" + l.Text + " ")
		case DiffAdded:
			sb.WriteString("+" + l.Text + " ")
		case DiffRemoved:
			sb.WriteString("-" + l.Text + " ")
		}
	}
	return strings.TrimSpace(sb.String())
}

func TestSplitLines(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "a", []string{"a"}},
		{"trailing newline", "a\n", []string{"a"}},
		{"multi line", "a\nb\n", []string{"a", "b"}},
		{"no trailing newline", "a\nb", []string{"a", "b"}},
		{"blank line", "a\n\nb\n", []string{"a", "", "b"}},
		{"only newline", "\n", []string{""}},
		{"crlf normalized", "a\r\nb\r\n", []string{"a", "b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SplitLines([]byte(c.in))
			if len(got) != len(c.want) {
				t.Fatalf("SplitLines(%q) 长度 = %v, want %v", c.in, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("SplitLines(%q) = %v, want %v", c.in, got, c.want)
				}
			}
		})
	}
}

func TestDiffIdentical(t *testing.T) {
	lines := Diff([]byte("a\nb\nc\n"), []byte("a\nb\nc\n"))
	if len(lines) != 3 {
		t.Fatalf("相同内容 diff 行数 = %d, want 3", len(lines))
	}
	for i, l := range lines {
		if l.Kind != DiffContext {
			t.Errorf("第 %d 行应为上下文, got %v", i, l.Kind)
		}
		if l.NumA != i+1 || l.NumB != i+1 {
			t.Errorf("第 %d 行行号 = (%d, %d), want (%d, %d)", i, l.NumA, l.NumB, i+1, i+1)
		}
	}
}

func TestDiffBothEmpty(t *testing.T) {
	if lines := Diff(nil, nil); lines != nil {
		t.Fatalf("两空文件 diff 应为 nil, got %v", lines)
	}
}

func TestDiffAppendOnly(t *testing.T) {
	lines := Diff([]byte("a\nb\n"), []byte("a\nb\nc\nd\n"))
	got := summary(lines)
	want := "=a =b +c +d"
	if got != want {
		t.Errorf("追加场景 diff = %q, want %q", got, want)
	}
	// 行号校验：新增行 NumB 应为 3、4，NumA 为 0。
	adds := 0
	for _, l := range lines {
		if l.Kind == DiffAdded {
			adds++
			if l.NumB == 0 {
				t.Errorf("新增行行号 NumB 不应为 0")
			}
			if l.NumA != 0 {
				t.Errorf("新增行 NumA 应为 0, got %d", l.NumA)
			}
		}
	}
	if adds != 2 {
		t.Errorf("新增行数 = %d, want 2", adds)
	}
}

func TestDiffRemoveOnly(t *testing.T) {
	lines := Diff([]byte("a\nb\nc\n"), []byte("a\n"))
	got := summary(lines)
	want := "=a -b -c"
	if got != want {
		t.Errorf("删除场景 diff = %q, want %q", got, want)
	}
}

func TestDiffModifyLine(t *testing.T) {
	// 中间一行被修改：表现为“删旧行 + 增新行”相邻对。
	lines := Diff([]byte("a\nold\nc\n"), []byte("a\nnew\nc\n"))
	got := summary(lines)
	want := "=a -old +new =c"
	if got != want {
		t.Errorf("修改场景 diff = %q, want %q", got, want)
	}
	// 修改行应的行号：被删行指向母本第 2 行，新增行指向当前文件第 2 行。
	for _, l := range lines {
		if l.Kind == DiffRemoved && l.NumA != 2 {
			t.Errorf("被删行 NumA = %d, want 2", l.NumA)
		}
		if l.Kind == DiffAdded && l.NumB != 2 {
			t.Errorf("新增行 NumB = %d, want 2", l.NumB)
		}
	}
}

func TestDiffFirstLineChanged(t *testing.T) {
	lines := Diff([]byte("old\nb\n"), []byte("new\nb\n"))
	got := summary(lines)
	want := "-old +new =b"
	if got != want {
		t.Errorf("首行修改 diff = %q, want %q", got, want)
	}
}

func TestDiffCRLFAndLFEquivalent(t *testing.T) {
	// CRLF 与 LF 混排的相同内容应全部判为上下文（行尾 \r 已剥离）。
	lines := Diff([]byte("a\r\nb\r\n"), []byte("a\nb\n"))
	for i, l := range lines {
		if l.Kind != DiffContext {
			t.Errorf("第 %d 行 CRLF/LF 混排应判上下文, got %v", i, l.Kind)
		}
	}
}

func TestDiffEmptyVsContent(t *testing.T) {
	// 空母本 vs 有内容：全部新增。
	got := summary(Diff(nil, []byte("x\ny\n")))
	if got != "+x +y" {
		t.Errorf("空→有 diff = %q, want %q", got, "+x +y")
	}
	// 有内容 vs 空：全部删除。
	got = summary(Diff([]byte("x\ny\n"), nil))
	if got != "-x -y" {
		t.Errorf("有→空 diff = %q, want %q", got, "-x -y")
	}
	// 空母本 vs 仅空行："\n" 表示一个空行。
	got = summary(Diff(nil, []byte("\n")))
	if got != "+" {
		t.Errorf("空→空行 diff = %q, want %q", got, "+")
	}
}

func TestDiffLargeFile(t *testing.T) {
	// 2000 行文件里改中间 1 行，验证 Myers 在大输入下正确且无 panic。
	var a, b strings.Builder
	for i := 1; i <= 2000; i++ {
		a.WriteString("line ")
		a.WriteString(strings.Repeat("x", i%10))
		a.WriteString("\n")
	}
	b.WriteString(a.String())
	// 改动第 1000 行。
	linesB := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	linesB[999] = "line CHANGED"
	b.Reset()
	b.WriteString(strings.Join(linesB, "\n"))
	b.WriteString("\n")

	lines := Diff([]byte(a.String()), []byte(b.String()))
	var adds, dels, ctx int
	for _, l := range lines {
		switch l.Kind {
		case DiffContext:
			ctx++
		case DiffAdded:
			adds++
		case DiffRemoved:
			dels++
		}
	}
	// 1 改动 = 1 删 + 1 增，其余 1999 行为上下文。
	if adds != 1 || dels != 1 || ctx != 1999 {
		t.Errorf("大文件 diff 统计 = +%d/-%d/=%d, want +1/-1/=1999", adds, dels, ctx)
	}
}
