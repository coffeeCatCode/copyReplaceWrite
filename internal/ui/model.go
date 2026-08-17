// Package ui 实现 crw 的 Bubble Tea 界面：yazi 风格的双栏窗口，
// 左侧为版本文件列表，右侧为内容预览。负责所有键盘交互、文件操作触发与渲染。
package ui

import (
	"path/filepath"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"crw/internal/backup"
)

// focus 表示当前键盘焦点所在的面板。
type focus int

const (
	focusList    focus = iota // 版本列表：j/k 上下切换选项，预览跟随
	focusPreview              // 内容预览：j/k 滚动预览内容
)

// mode 表示界面所处的交互模式。
type mode int

const (
	modeNormal        mode = iota // 正常浏览
	modeConfirmDelete             // 等待删除确认（d 键触发）
)

// deleteTarget 记录待确认删除的备份项。
type deleteTarget struct {
	entry backup.Entry
}

// Model 是 crw 的 Bubble Tea 模型，保存界面状态与文件操作所需信息。
// 所有方法均使用指针接收者，避免复制内部包含锁的 viewport.Model
// （复制含锁结构体会触发 go vet copylocks 告警）。
type Model struct {
	basePath string         // 目标文件绝对路径
	baseName string         // 目标文件名（不含目录）
	entries  []backup.Entry // 版本列表，entries[0] 为目标文件（若存在）
	cursor   int            // 列表光标位置（指向 entries 的索引）
	focus    focus          // 当前焦点面板
	mode     mode           // 交互模式

	// 对比模式（F1 切换）：进入时把当前选中项记为母本 compareBase，
	// 此后预览区渲染“选中项 vs 母本”的行级 diff，其余操作保持不变。
	compareMode bool         // 是否处于对比模式
	compareBase backup.Entry // 对比母本（进入对比模式时选中的文件）
	// hunkOnly 对比模式下的显示方式（s 键切换）：true 时仅显示变更块及
	// 上下少量上下文（git diff 风格），false 时全文显示（默认）。
	hunkOnly bool

	// lastPosPath 记录按空格跳到母本前光标所在条目的路径；
	// 再次按空格时若仍在母本位置则跳回该条目。为空表示无待返回位置。
	lastPosPath string

	preview     viewport.Model // 预览区滚动视图
	previewRaw  []byte         // 预览区当前展示的原始内容（判断是否全部可见）
	previewPath string         // 预览区当前展示的文件路径（用于判断是否需重载）

	// highlightCache 缓存语法高亮结果（键为文件路径）。
	// 翻列表来回切换文件时，同一文件不再重复分词渲染，保证零延迟；容量见
	// highlightPreview 的清理策略。
	highlightCache map[string]highlightCacheEntry

	width  int // 终端宽度（WindowSizeMsg 提供）
	height int // 终端高度
	listW  int // 左侧列表栏宽度（含边框）
	listH  int // 左侧列表栏内容行数

	status        string // 底部状态栏消息
	statusIsError bool   // 状态消息是否为错误（决定红色显示）

	deleteTarget *deleteTarget // 删除确认目标；nil 表示无待确认项
}

// New 创建界面模型并载入首个条目的预览。
// entries 由 backup.List 提供；basePath 为目标文件绝对路径。
func New(basePath string, entries []backup.Entry) *Model {
	m := &Model{
		basePath:       basePath,
		baseName:       filepath.Base(basePath),
		entries:        entries,
		preview:        viewport.New(0, 0),
		highlightCache: make(map[string]highlightCacheEntry),
	}
	// 预览区的滚动完全由本模型驱动（见 update.go），不使用 viewport 自带键位。
	m.preview.KeyMap = viewport.KeyMap{}
	m.loadPreview()
	return m
}

// Init 初始化 Bubble Tea 程序。本界面无需启动命令（无定时器、无异步任务）。
func (m *Model) Init() tea.Cmd {
	return nil
}
