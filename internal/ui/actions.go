package ui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"crw/internal/backup"
)

// performReplace 执行核心替换：把选中备份的内容写回目标文件（Enter 键）。
//
// 写回前，若目标文件存在且其当前内容在备份列表中不存在相同项，则先自动
// 备份当前内容（秒级时间戳命名），保证替换可回退。
// 选中项为目标文件本身时不做任何操作：单文件场景按约定不产生备份。
func (m *Model) performReplace() {
	if len(m.entries) == 0 {
		return
	}
	sel := m.entries[m.cursor]
	if sel.IsBase {
		// 目标文件本身：替换无意义，也绝不为其产生备份。
		if len(m.entries) == 1 {
			m.setStatus("当前只有一个文件；按 c 可把当前内容克隆为备份")
		} else {
			m.setStatus("这是当前文件本身，无需替换")
		}
		return
	}

	// 1) 读取当前内容；目标文件缺失时跳过自动备份（空文件可备份，缺失不可）。
	backupName := ""
	if _, err := os.Stat(m.basePath); err == nil {
		current, err := backup.ReadContent(m.basePath)
		if err != nil {
			m.setError(err.Error())
			return
		}
		dup, err := backup.FindIdenticalBackup(m.entries, current)
		if err != nil {
			m.setError(err.Error())
			return
		}
		if dup == nil {
			// 当前内容在备份列表中无相同项 → 自动备份。
			newPath, err := backup.UniqueBackupPath(m.basePath, time.Now())
			if err != nil {
				m.setError(err.Error())
				return
			}
			if err := backup.WriteContent(newPath, current); err != nil {
				m.setError(err.Error())
				return
			}
			backupName = filepath.Base(newPath)
		}
	}

	// 2) 将选中备份内容写回目标文件。
	data, err := backup.ReadContent(sel.Path)
	if err != nil {
		m.setError(err.Error())
		return
	}
	if err := backup.WriteContent(m.basePath, data); err != nil {
		m.setError(err.Error())
		return
	}

	// 3) 刷新列表，保持光标停在刚应用的备份项上。
	m.refresh()
	if idx := indexOfPath(m.entries, sel.Path); idx >= 0 {
		m.moveCursorTo(idx)
	}

	if backupName != "" {
		m.setStatus(fmt.Sprintf("已用 %s 的内容替换 %s，原内容自动备份为 %s", sel.Name, m.baseName, backupName))
	} else {
		m.setStatus(fmt.Sprintf("已用 %s 的内容替换 %s", sel.Name, m.baseName))
	}
}

// performCopy 把当前文件内容克隆为一份新备份（c 键）。
//
// 若备份列表中已存在内容完全相同的备份，则不重复创建，而是将光标跳转到
// 该备份项；否则按秒级时间戳命名创建新备份并跳转到新项。
// 目标文件缺失时无法克隆，直接报错。
func (m *Model) performCopy() {
	if len(m.entries) == 0 {
		return
	}
	if _, err := os.Stat(m.basePath); err != nil {
		m.setError("当前文件不存在，无法复制: " + err.Error())
		return
	}

	current, err := backup.ReadContent(m.basePath)
	if err != nil {
		m.setError(err.Error())
		return
	}
	dup, err := backup.FindIdenticalBackup(m.entries, current)
	if err != nil {
		m.setError(err.Error())
		return
	}
	if dup != nil {
		// 已存在相同内容的备份 → 跳转到该项，不产生重复文件。
		if idx := indexOfPath(m.entries, dup.Path); idx >= 0 {
			m.moveCursorTo(idx)
		}
		m.setStatus("已存在相同内容的备份: " + dup.Name)
		return
	}

	newPath, err := backup.UniqueBackupPath(m.basePath, time.Now())
	if err != nil {
		m.setError(err.Error())
		return
	}
	if err := backup.WriteContent(newPath, current); err != nil {
		m.setError(err.Error())
		return
	}
	m.refresh()
	if idx := indexOfPath(m.entries, newPath); idx >= 0 {
		m.moveCursorTo(idx)
	}
	m.setStatus("已克隆当前内容为备份: " + filepath.Base(newPath))
}

// askDelete 请求删除当前选中项。
// confirm=true 时（d 键）弹出确认框；false 时（D 键）直接删除。
// 目标文件本身禁止删除。
func (m *Model) askDelete(confirm bool) {
	if len(m.entries) == 0 {
		return
	}
	sel := m.entries[m.cursor]
	if sel.IsBase {
		m.setError("不能删除当前文件本身")
		return
	}
	if confirm {
		m.mode = modeConfirmDelete
		m.deleteTarget = &deleteTarget{entry: sel}
		return
	}
	m.performDelete(sel)
}

// performDelete 删除指定备份文件并刷新列表。
func (m *Model) performDelete(e backup.Entry) {
	if err := backup.Delete(e.Path); err != nil {
		m.setError(err.Error())
		return
	}
	m.refresh()
	m.setStatus("已删除 " + e.Name)
}

// refresh 重新扫描目录重建版本列表，并尽量保持光标指向原路径项；
// 原路径项已不存在（被删除）时，光标保持在原索引并自动夹取到合法范围。
func (m *Model) refresh() {
	prevPath := ""
	if m.cursor >= 0 && m.cursor < len(m.entries) {
		prevPath = m.entries[m.cursor].Path
	}

	entries, err := backup.List(m.basePath)
	if err != nil {
		m.setError("刷新列表失败: " + err.Error())
		return
	}
	m.entries = entries

	if idx := indexOfPath(entries, prevPath); idx >= 0 {
		m.cursor = idx
	} else if m.cursor >= len(entries) {
		m.cursor = len(entries) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.loadPreview()
}

// loadPreview 读取并渲染当前选中条目的预览内容（滚动位置重置到顶部）。
// 二进制文件（前 8KB 内出现 NUL 字节）不渲染原始内容，显示占位提示。
func (m *Model) loadPreview() {
	if len(m.entries) == 0 {
		m.preview.SetContent("")
		return
	}
	e := m.entries[m.cursor]
	data, err := backup.ReadContent(e.Path)
	if err != nil {
		m.setError("读取预览失败: " + err.Error())
		m.preview.SetContent("")
		return
	}
	m.previewPath = e.Path
	m.previewRaw = data
	if isBinary(data) {
		m.preview.SetContent("（二进制文件，无法预览）")
	} else {
		m.preview.SetContent(string(data))
	}
	m.preview.GotoTop()
}

// isBinary 粗略判断内容是否包含二进制数据：仅检查前 8KB 是否出现 NUL 字节。
func isBinary(data []byte) bool {
	n := len(data)
	if n > 8192 {
		n = 8192
	}
	return bytes.IndexByte(data[:n], 0) >= 0
}

// indexOfPath 返回 entries 中 Path 等于 target 的索引；不存在返回 -1。
func indexOfPath(entries []backup.Entry, target string) int {
	for i, e := range entries {
		if e.Path == target {
			return i
		}
	}
	return -1
}

// setStatus 显示普通状态消息（白色）。
func (m *Model) setStatus(s string) {
	m.status = s
	m.statusIsError = false
}

// setError 显示错误状态消息（红色）。
func (m *Model) setError(s string) {
	m.status = s
	m.statusIsError = true
}
