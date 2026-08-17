package ui

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"crw/internal/backup"
)

// 界面配色与样式集中定义，便于统一调整。
// 蓝色系用于焦点/选中强调，红色用于错误与删除确认，灰色用于非焦点边框。
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("75"))

	listStyle         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	listStyleFocused  = listStyle.BorderForeground(lipgloss.Color("75"))
	previewStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	previewFocused    = previewStyle.BorderForeground(lipgloss.Color("75"))
	selStyle          = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("0")).Bold(true)
	baseStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	statusStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	helpStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	confirmBoxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("9")).Padding(0, 2)
	previewTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75"))
	// 对比模式（F1）渲染样式：标题黄色、新增绿、删除红、上下文灰。
	diffHeadStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	diffAddStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	diffDelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	diffCtxStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	diffGapStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
)

// View 渲染整个界面：标题栏 + 双栏主体 + 状态/快捷键栏。
// 终端尺寸尚未就绪时显示初始化提示；过小时提示放大窗口。
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "正在初始化…"
	}
	if m.width < 60 || m.height < 12 {
		return fmt.Sprintf("终端窗口太小，请放大到至少 60x12（当前 %dx%d）", m.width, m.height)
	}

	header := m.headerView()
	var body string
	if m.mode == modeConfirmDelete && m.deleteTarget != nil {
		body = m.confirmBoxView()
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.listView(), " ", m.previewView())
	}
	footer := m.footerView()
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// headerView 渲染顶部标题栏：程序名与目标文件路径。
func (m *Model) headerView() string {
	return titleStyle.Render(fmt.Sprintf("crw — 版本管理: %s", m.basePath))
}

// listView 渲染左侧版本列表面板。当前文件带 ● 前缀，选中行整行高亮。
func (m *Model) listView() string {
	contentW := m.listW - 2 // 扣除左右边框
	rows := make([]string, 0, m.listH)
	for i, e := range m.entries {
		rows = append(rows, m.listRow(i, e, contentW))
	}
	// 用空行补齐剩余行，保证边框高度稳定。
	for len(rows) < m.listH {
		rows = append(rows, strings.Repeat(" ", contentW))
	}
	content := strings.Join(rows, "\n")

	style := listStyle
	if m.focus == focusList {
		style = listStyleFocused
	}
	return style.Render(content)
}

// listRow 渲染列表中的一行。mark 列：目标文件 ●，备份文件空白。
func (m *Model) listRow(i int, e backup.Entry, contentW int) string {
	mark := "  "
	if e.IsBase {
		mark = "● "
	}
	// 文件名超过可用宽度时截断并补省略号。
	name := truncate(e.Name, contentW-2)

	// 用 lipgloss 的 Width 做右对齐补白，可正确处理 CJK 等宽字符。
	rowStyle := lipgloss.NewStyle().Width(contentW)
	padded := rowStyle.Render(mark + name)
	if i == m.cursor {
		return selStyle.Render(padded)
	}
	if e.IsBase {
		return baseStyle.Render(padded)
	}
	return padded
}

// previewView 渲染右侧预览面板：标题行（文件名 + 滚动百分比）+ 分隔线 + 内容。
// 对比模式下标题追加“ ⟷ 母本文件名”标识，提示当前展示的是差异视图。
func (m *Model) previewView() string {
	title := ""
	if m.cursor >= 0 && m.cursor < len(m.entries) {
		e := m.entries[m.cursor]
		name := truncate(e.Name, m.preview.Width)
		if m.compareMode {
			// 对比模式：黄字标题，说明当前对象与母本的对比关系。
			name = truncate(e.Name+" ⟷ "+filepath.Base(m.compareBase.Path), m.preview.Width)
			// ScrollPercent() 返回 0~1 小数，需乘 100 才是百分比；
			// 内容完全可见时不显示百分比（避免误导为"已滚到底"）。
			pct := ""
			if !m.previewFits() {
				pct = fmt.Sprintf("  [%d%%]", int(m.preview.ScrollPercent()*100))
			}
			title = diffHeadStyle.Render(name + pct)
		} else {
			// ScrollPercent() 返回 0~1 小数，需乘 100 才是百分比；
			// 内容完全可见时不显示百分比（避免误导为"已滚到底"）。
			pct := ""
			if !m.previewFits() {
				pct = fmt.Sprintf("  [%d%%]", int(m.preview.ScrollPercent()*100))
			}
			title = previewTitleStyle.Render(name + pct)
		}
	}
	divider := strings.Repeat("─", m.preview.Width)
	content := title + "\n" + divider + "\n" + m.preview.View()

	style := previewStyle
	if m.focus == focusPreview {
		style = previewFocused
	}
	return style.Render(content)
}

// previewFits 报告预览内容是否全部可见（行数不超过预览区高度）。
func (m *Model) previewFits() bool {
	// 二进制占位提示固定一行，必然全部可见。
	if isBinary(m.previewRaw) {
		return true
	}
	return bytes.Count(m.previewRaw, []byte("\n"))+1 <= m.preview.Height
}

// confirmBoxView 渲染删除确认弹窗，垂直水平居中于终端。
func (m *Model) confirmBoxView() string {
	if m.deleteTarget == nil {
		return ""
	}
	box := confirmBoxStyle.Render("确认删除 " + m.deleteTarget.entry.Name + " ?\n\n" +
		"[y] 确认    [n] 取消")
	bw, bh := lipgloss.Size(box)
	top := 1 + (m.height-1-bh)/2 // 扣除标题行后居中
	if top < 1 {
		top = 1
	}
	left := (m.width - bw) / 2
	if left < 0 {
		left = 0
	}
	return strings.Repeat("\n", top) + strings.Repeat(" ", left) + box
}

// footerView 渲染底部两行：状态消息 + 快捷键提示。
func (m *Model) footerView() string {
	status := m.status
	if status == "" {
		status = "就绪"
	}
	statusStyle := statusStyle
	if m.statusIsError {
		statusStyle = errorStyle
	}
	statusLine := statusStyle.Width(m.width).Render(status)

	help := "j/k 选择/滚动  Tab/←/→ 焦点  Space 母本/返回  F1 对比  s 全文/hunk  Enter 替换  c 复制  d/D 删除  r 刷新  q 退出"
	helpLine := helpStyle.Width(m.width).Render(truncate(help, m.width))
	return statusLine + "\n" + helpLine
}

// truncate 将字符串截断到 maxRunes 个字符宽度（近似按 rune 计），超长补省略号。
// 使用近似字符计数：CJK 等宽字符按 2 计，其余按 1 计。
func truncate(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	width := 0
	for i, r := range s {
		w := 1
		if r > 0x2E7F { // CJK 等宽区粗略判断
			w = 2
		}
		if width+w > maxWidth {
			if maxWidth-width >= 2 {
				return s[:i] + "…"
			}
			return s[:i]
		}
		width += w
	}
	return s
}
