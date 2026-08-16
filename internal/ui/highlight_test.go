package ui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"crw/internal/backup"
)

// ansiRe 匹配 256 色 ANSI 转义序列（\x1b[38;5;Nm 与复位 \x1b[0m）。
var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

// stripANSI 移除字符串中的 ANSI 转义，用于断言“高亮不改变文本内容”。
func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// TestHighlightSourceC 验证 C 源码被高亮：输出含 ANSI 转义，且剥离转义后内容与原文一致。
func TestHighlightSourceC(t *testing.T) {
	src := "int main(void) {\n    return 42;\n}\n"
	got := highlightSource([]byte(src), "main.c")
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("C 文件未产生 ANSI 高亮输出: %q", got)
	}
	if stripANSI(got) != src {
		t.Errorf("高亮改变了文本内容:\ngot  %q\nwant %q", stripANSI(got), src)
	}
}

// TestHighlightBackupName 验证 crw 标准备份名（config.yaml.bak.20260816_140000）
// 在剥离时间戳后缀后仍能按原名识别语言并高亮。
func TestHighlightBackupName(t *testing.T) {
	src := "# 注释\nkey: value\n"
	got := highlightSource([]byte(src), "config.yaml.bak.20260816_140000")
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("备份文件名未识别语言: %q", got)
	}
	if stripANSI(got) != src {
		t.Errorf("高亮改变了文本内容: %q", stripANSI(got))
	}
}

// TestHighlightBackupNameWithSeq 同秒冲突追加 _2 后缀的备份名同样应被识别。
func TestHighlightBackupNameWithSeq(t *testing.T) {
	got := highlightSource([]byte("int x = 1;\n"), "main.c.bak.20260816_140000_2")
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("带序号后缀的备份名未识别语言: %q", got)
	}
}

// TestHighlightBackupNameDateOnly 纯日期备份名（如手动 cp 生成的
// config.yaml.bak.20260816）也应识别语言。
func TestHighlightBackupNameDateOnly(t *testing.T) {
	src := "# 注释\nkey: value\n"
	got := highlightSource([]byte(src), "config.yaml.bak.20260816")
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("纯日期备份名未识别语言: %q", got)
	}
	if stripANSI(got) != src {
		t.Errorf("高亮改变了文本内容: %q", stripANSI(got))
	}
}

// TestHighlightUnknownExt 未知扩展名的文件不产生高亮，原样返回。
func TestHighlightUnknownExt(t *testing.T) {
	src := "some unknown content\n"
	got := highlightSource([]byte(src), "data.xyz_unknown")
	if got != src {
		t.Errorf("未知扩展名应原样返回, got %q", got)
	}
}

// TestHighlightNoExt 无扩展名文件不产生高亮（避免误判成某语言），原样返回。
func TestHighlightNoExt(t *testing.T) {
	src := "plain text\n"
	got := highlightSource([]byte(src), "README")
	if got != src {
		t.Errorf("无扩展名文件应原样返回, got %q", got)
	}
}

// TestHighlightTooLarge 超过 highlightMaxSize 的文件不做高亮，原样返回。
func TestHighlightTooLarge(t *testing.T) {
	src := strings.Repeat("int x = 1;\n", (highlightMaxSize/11)+10)
	if len(src) <= highlightMaxSize {
		t.Fatalf("测试数据不够大: %d", len(src))
	}
	got := highlightSource([]byte(src), "big.c")
	if got != string(src) {
		t.Errorf("超限文件应原样返回，不应产生 ANSI: len=%d", len(got))
	}
}

// TestHighlightEmpty 空文件高亮得到空串，不崩溃。
func TestHighlightEmpty(t *testing.T) {
	if got := highlightSource(nil, "empty.c"); got != "" {
		t.Errorf("空文件应返回空串, got %q", got)
	}
}

// TestHighlightKeywordsColored C 关键字应有专属颜色（非默认前景色），
// 证明紧凑 formatter 确实输出了有效的高亮（而非全部原样）。
func TestHighlightKeywordsColored(t *testing.T) {
	got := highlightSource([]byte("int main(void) { return 42; }\n"), "main.c")
	if !strings.Contains(got, "\x1b[38;5;") {
		t.Fatalf("关键字未着色: %q", got)
	}
}

// TestLexerForName 验证语言识别：普通文件名、备份名、未知名。
func TestLexerForName(t *testing.T) {
	cases := []struct {
		name string
		want bool // 是否应识别出语言
	}{
		{"main.c", true},
		{"main.c.bak.20260816_140000", true},
		{"main.c.bak.20260816_140000_2", true},
		{"config.yaml", true},
		{"config.yaml.bak.20260816_140000", true},
		{"config.yaml.bak.20260816", true},
		{"config.yaml.bak.2", true},
		{"config.yaml.1", true},
		{"HTimer.h", true},
		{"Makefile", true},
		{"README.md", true},
		{"data.xyz_unknown", false},
		{"README", false},
	}
	for _, c := range cases {
		if got := lexerForName(c.name); (got != nil) != c.want {
			t.Errorf("lexerForName(%q) = %v, want 识别=%v", c.name, got != nil, c.want)
		}
	}
}

// TestHighlightPreviewCache 验证 Model 级高亮缓存：同一文件二次获取命中缓存
// （缓存表不增长、内容一致）；修改时间或大小变化后重新渲染并更新缓存。
func TestHighlightPreviewCache(t *testing.T) {
	m := New("/tmp/x/config.yaml", nil)
	data := []byte("# a\nkey: value\n")
	mod := time.Now()
	e := backup.Entry{Path: "/tmp/x/config.yaml.bak.20260816_140000", Name: "config.yaml.bak.20260816_140000", ModTime: mod, Size: int64(len(data))}

	first := m.highlightPreview(e, data)
	if !strings.Contains(first, "\x1b[") {
		t.Fatalf("首次高亮未产生 ANSI: %q", first)
	}
	if len(m.highlightCache) != 1 {
		t.Fatalf("首次高亮后缓存项数 = %d, want 1", len(m.highlightCache))
	}

	// 同一文件（大小/时间未变）→ 命中缓存，结果仍有效。
	second := m.highlightPreview(e, data)
	if second != first {
		t.Errorf("缓存命中应返回相同文本")
	}
	if len(m.highlightCache) != 1 {
		t.Errorf("缓存命中后缓存项数 = %d, want 1", len(m.highlightCache))
	}

	// 文件被修改（内容/大小变化）→ 缓存失效，重新渲染。
	e2 := e
	e2.Size = int64(len(data) + 5)
	third := m.highlightPreview(e2, []byte(string(data)+"xx\n"))
	if third == first {
		t.Errorf("内容变化后缓存未失效")
	}
}

// TestHighlightPreviewCacheContentUnchanged 缓存命中不影响渲染内容正确性：
// 剥离 ANSI 后内容与原文一致（覆盖缓存路径的回归保护）。
func TestHighlightPreviewCacheContentUnchanged(t *testing.T) {
	m := New("/tmp/x/main.c", nil)
	src := "int main(void) { return 0; }\n"
	mod := time.Now()
	e := backup.Entry{Path: "/tmp/x/main.c.bak.20260816_140000", Name: "main.c.bak.20260816_140000", ModTime: mod, Size: int64(len(src))}

	// 首次渲染（进缓存）与命中缓存各验证一次内容完整性。
	for i := 0; i < 2; i++ {
		got := m.highlightPreview(e, []byte(src))
		if stripANSI(got) != src {
			t.Fatalf("第 %d 次渲染改变了文本内容: %q", i+1, stripANSI(got))
		}
	}
}
