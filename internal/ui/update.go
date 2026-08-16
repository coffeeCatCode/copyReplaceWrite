package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Update 处理 Bubble Tea 消息：终端尺寸变化与键盘事件。
// 返回 tea.Quit 命令请求程序退出；其余消息返回 nil 命令。
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()

	case tea.KeyMsg:
		if m.handleKey(msg) {
			// 用户请求退出（q / Ctrl+C / Esc）。
			return m, tea.Quit
		}
	}
	return m, nil
}

// handleKey 分发全部按键事件。返回 true 表示请求退出程序。
// 按键语义：
//
//	列表焦点下：j/k/↑/↓ 移动光标；g/G 跳转首/末项
//	预览焦点下：j/k/↑/↓ 滚动预览；g/G 滚到顶/底
//	任意焦点：  Tab/l/→ 切到预览，h/← 切回列表；
//	            Ctrl+D/Ctrl+U/PgDn/PgUp 滚动预览
//	全局操作：  Enter 用选中备份替换当前文件；c 克隆当前内容为备份；
//	            d 删除（需确认）；D 直接删除；r 重新扫描目录；
//	            q/Ctrl+C/Esc 退出
func (m *Model) handleKey(msg tea.KeyMsg) bool {
	// 删除确认框优先拦截按键。
	if m.mode == modeConfirmDelete {
		return m.handleConfirmKey(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return true

	case "tab":
		// Tab 在列表与预览之间来回切换。
		if m.focus == focusList {
			m.focus = focusPreview
		} else {
			m.focus = focusList
		}
	case "l", "right":
		m.focus = focusPreview
	case "h", "left":
		m.focus = focusList

	case "j", "down":
		if m.focus == focusList {
			m.moveCursor(1)
		} else {
			m.preview.LineDown(1)
		}
	case "k", "up":
		if m.focus == focusList {
			m.moveCursor(-1)
		} else {
			m.preview.LineUp(1)
		}
	case "g":
		if m.focus == focusList {
			m.moveCursorTo(0)
		} else {
			m.preview.GotoTop()
		}
	case "G":
		if m.focus == focusList {
			m.moveCursorTo(len(m.entries) - 1)
		} else {
			m.preview.GotoBottom()
		}

	case "ctrl+d":
		m.preview.HalfPageDown()
	case "ctrl+u":
		m.preview.HalfPageUp()
	case "pgdown":
		m.preview.PageDown()
	case "pgup":
		m.preview.PageUp()

	case "enter":
		m.performReplace()
	case "c":
		m.performCopy()
	case "d":
		m.askDelete(true)
	case "D":
		m.askDelete(false)
	case "r":
		m.refresh()
	}
	return false
}

// handleConfirmKey 处理删除确认框的按键：y/Enter 确认删除，n/Esc/q 取消。
// 确认框不提供退出程序的能力，避免误触。
func (m *Model) handleConfirmKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "y", "Y", "enter":
		target := m.deleteTarget // 先取走目标，performDelete 内会刷新列表
		m.cancelConfirm()
		if target != nil {
			m.performDelete(target.entry)
		}
	case "n", "N", "esc", "q":
		m.cancelConfirm()
	}
	return false
}

// cancelConfirm 关闭删除确认框，回到正常模式。
func (m *Model) cancelConfirm() {
	m.mode = modeNormal
	m.deleteTarget = nil
}

// layout 依据终端尺寸计算两栏布局并调整预览视图尺寸。
// 左侧列表约占 30% 宽度（夹在 [18, 44]），右侧预览占剩余宽度。
func (m *Model) layout() {
	// 左侧列表栏宽度（含边框）。
	m.listW = m.width * 3 / 10
	if m.listW < 18 {
		m.listW = 18
	}
	if m.listW > 44 {
		m.listW = 44
	}

	// 主区高度 = 总高度 - 标题行 - 状态栏 - 快捷键栏。
	mainH := m.height - 3
	if mainH < 4 {
		mainH = 4
	}
	m.listH = mainH - 2 // 列表内容行数 = 主区 - 上下边框

	// 预览栏总宽 = 剩余宽度 - 两栏间 1 格留白。
	// 预览内容宽 = 预览栏总宽 - 2（左右边框） - 2（标题行 + 分隔线）。
	m.preview.Width = m.width - m.listW - 3 - 2 - 2
	if m.preview.Width < 10 {
		m.preview.Width = 10
	}
	m.preview.Height = mainH - 4
}

// moveCursor 将列表光标移动 delta 格（负值向上），并刷新预览到新选中项。
func (m *Model) moveCursor(delta int) {
	m.moveCursorTo(m.cursor + delta)
}

// moveCursorTo 将列表光标定位到 idx（自动夹取到合法范围），并刷新预览。
func (m *Model) moveCursorTo(idx int) {
	if len(m.entries) == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.entries) {
		idx = len(m.entries) - 1
	}
	if m.cursor != idx {
		m.cursor = idx
		m.loadPreview()
	}
}
