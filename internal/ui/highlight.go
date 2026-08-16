// 语法高亮：预览区文本渲染前，按文件名识别语言并用 chroma（纯 Go，约 250 种
// 语言，覆盖 C/C++/Go/Python/Shell/Makefile/JSON/YAML/INI/Markdown 等常见格式）
// 输出 256 色 ANSI 转义，效果与 bat 命令的语法高亮一致，但无需外部依赖。
//
// 两个性能保障：
//  1. 超过 highlightMaxSize 的文件不做高亮（chroma 高亮耗时随文件大小线性增长，
//     实测 64KB ≈ 150ms、1MB ≈ 2.4s，交互式 TUI 必须限制）；
//  2. 自定义紧凑 formatter 只给“非默认前景色”的 token 输出转义，
//     空格、标点、普通文本（默认前景色）原样写出，输出量仅为标准 TTY256 的一半，
//     内存占用更省。
package ui

import (
	"io"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"crw/internal/backup"
)

// highlightMaxSize 语法高亮的文件大小上限（字节）。
// 超过该大小的文件以纯文本预览，避免切换文件时明显卡顿。
const highlightMaxSize = 64 * 1024

// monokaiTheme 由 styles.Monokai 预构建的主题表（token 类型 → 样式条目）。
// chroma 的 Style.Get 会把未定义类型的颜色继承为默认前景色，
// 这里改用精确类型 → 子类 → 大类三级查找，未命中即视为无色。
var (
	monokaiOnce  sync.Once
	monokaiTheme map[chroma.TokenType]chroma.StyleEntry
	monokaiText  chroma.Colour // 默认前景色；与该色相同的 token 不输出转义
)

// escapeStart 预构建 256 色索引 → ANSI 前景转义前缀，避免逐 token 格式化。
var escapeStart [256][]byte
var escapeEnd = []byte("\x1b[0m")

func init() {
	for i := range escapeStart {
		escapeStart[i] = []byte("\x1b[38;5;" + itoa(i) + "m")
	}
}

// itoa 十进制整数转字符串（仅用于 0~255 的转义前缀构建）。
func itoa(v int) string {
	return string(rune('0'+v/100)) + string(rune('0'+v%100/10)) + string(rune('0'+v%10))
}

// rgb256 将 RGB 颜色映射为 xterm 256 色索引：
// 彩色通道按 6×6×6 立方体取近似（与 xterm 标准一致），纯灰按 24 级灰度斜坡。
func rgb256(r, g, b uint8) int {
	if r == g && g == b {
		if r < 8 {
			return 16
		}
		gray := (int(r) - 8) * 24 / 230
		if gray > 23 {
			gray = 23
		}
		return 232 + gray
	}
	cube := func(v uint8) int {
		switch {
		case v < 48:
			return 0
		case v < 115:
			return 1
		case v < 155:
			return 2
		case v < 195:
			return 3
		case v < 235:
			return 4
		default:
			return 5
		}
	}
	return 16 + 36*cube(r) + 6*cube(g) + cube(b)
}

// buildMonokaiTheme 初始化主题表：遍历风格中定义的全部 token 类型，
// 记录其精确样式（含类型继承），并取出 Text 类型的默认前景色。
func buildMonokaiTheme() {
	style := styles.Monokai
	monokaiTheme = make(map[chroma.TokenType]chroma.StyleEntry, len(style.Types()))
	for _, t := range style.Types() {
		monokaiTheme[t] = style.Get(t)
	}
	if e, ok := monokaiTheme[chroma.Text]; ok {
		monokaiText = e.Colour
	}
}

// lookupTheme 按 token 类型精确查找样式：先精确类型，再子类，再大类。
// 与 chroma 默认的 Style.Get 不同，不向上继承默认前景色，
// 使“该 token 没有专属颜色”这一信息得以保留。
func lookupTheme(t chroma.TokenType) (chroma.StyleEntry, bool) {
	monokaiOnce.Do(buildMonokaiTheme)
	if e, ok := monokaiTheme[t]; ok {
		return e, true
	}
	if e, ok := monokaiTheme[t.SubCategory()]; ok {
		return e, true
	}
	e, ok := monokaiTheme[t.Category()]
	return e, ok
}

// compactFormat 将分词结果渲染为 256 色 ANSI 文本。
// 仅当 token 命中主题且颜色不是默认前景色时才输出转义序列，
// 普通的空白、标点、未着色文本保持原样，输出量与渲染耗时均远小于标准 TTY256。
func compactFormat(w io.Writer, it chroma.Iterator) error {
	for token := it(); token.Type != chroma.EOFType; token = it() {
		entry, ok := lookupTheme(token.Type)
		if !ok || !entry.Colour.IsSet() || entry.Colour == monokaiText {
			io.WriteString(w, token.Value)
			continue
		}
		c := entry.Colour
		w.Write(escapeStart[rgb256(c.Red(), c.Green(), c.Blue())])
		io.WriteString(w, token.Value)
		w.Write(escapeEnd)
	}
	return nil
}

// lexerForName 按文件名识别语言。先直接匹配（目标文件本身、常规文件名）；
// 匹配不到时逐段剥离最后一段扩展名再试——备份名以各种后缀结尾
// （config.yaml.bak.20260816_140000、config.yaml.bak.20260816、config.yaml.bak.2、
// config.yaml.1），扩展名是备份后缀而非语言扩展名，必须逐段还原：
// config.yaml.bak.20260816_140000 → config.yaml.bak → config.yaml。
// 识别不到返回 nil，调用方以纯文本渲染。
func lexerForName(name string) chroma.Lexer {
	if lx := lexers.Match(name); lx != nil {
		return lx
	}
	rest := name
	for {
		dot := strings.LastIndex(rest, ".")
		if dot <= 0 {
			return nil
		}
		rest = rest[:dot]
		if lx := lexers.Match(rest); lx != nil {
			return lx
		}
	}
}

// highlightCacheEntry 一次语法高亮结果的缓存项。
// 键为文件路径；size/mod 用于在文件被外部修改后自动失效。
type highlightCacheEntry struct {
	size int64
	mod  time.Time
	text string
}

// highlightPreview 返回文件内容的高亮渲染文本，带结果缓存。
// 翻列表来回切换文件时，同一文件（路径、大小、修改时间均未变）直接返回缓存，
// 避免重复分词渲染造成的延迟。
func (m *Model) highlightPreview(e backup.Entry, data []byte) string {
	key := e.Path
	if c, ok := m.highlightCache[key]; ok && c.size == e.Size && c.mod.Equal(e.ModTime) {
		return c.text
	}
	text := highlightSource(data, e.Name)
	// 容量控制：超过 8 项或总缓存文本超过 2MB 时整表清空。
	// 单文件高亮输出最多约 300KB（64KB 上限对应的最大膨胀），
	// 该策略把缓存内存限制在 2MB 量级。
	if len(m.highlightCache) >= 8 || m.highlightCacheBytes() > 2*1024*1024 {
		m.highlightCache = make(map[string]highlightCacheEntry)
	}
	m.highlightCache[key] = highlightCacheEntry{size: e.Size, mod: e.ModTime, text: text}
	return text
}

// highlightCacheBytes 当前缓存文本总字节数。
func (m *Model) highlightCacheBytes() int {
	total := 0
	for _, c := range m.highlightCache {
		total += len(c.text)
	}
	return total
}

// highlightSource 返回 data 的语法高亮文本。无法识别语言、文件过大、
// 分词或渲染出错时原样返回文本（高亮是增强功能，任何异常都不得丢内容）。
func highlightSource(data []byte, name string) string {
	if len(data) > highlightMaxSize {
		return string(data)
	}
	lexer := lexerForName(name)
	if lexer == nil {
		return string(data)
	}
	it, err := lexer.Tokenise(nil, string(data))
	if err != nil {
		return string(data)
	}
	var sb strings.Builder
	sb.Grow(len(data) + len(data)/4)
	if err := compactFormat(&sb, it); err != nil {
		return string(data)
	}
	return sb.String()
}
