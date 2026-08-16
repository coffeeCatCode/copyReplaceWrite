// Command crw（copyReplaceWrite）是一个基于 Bubble Tea 的 TUI 版本管理工具。
//
// 用法：crw <文件名>
//
// 它以目标文件及其同目录下所有以目标文件名为前缀的备份文件（如
// config.yaml、config.yaml.bak.20260816、config.yaml_back）构成版本列表，
// 在 yazi 风格的双栏窗口中供用户浏览、预览、替换、克隆与删除。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"crw/internal/backup"
	"crw/internal/ui"
)

// usage 打印命令行用法说明。
func usage() {
	fmt.Fprintf(os.Stderr, "用法: crw <文件名>\n\n")
	fmt.Fprintf(os.Stderr, "示例: crw config.yaml\n\n")
	fmt.Fprintf(os.Stderr, "说明:\n")
	fmt.Fprintf(os.Stderr, "  打开同目录下 <文件名> 及其全部 <文件名>.* 备份文件的版本列表。\n")
	fmt.Fprintf(os.Stderr, "  回车: 用选中备份的内容替换当前文件（原内容自动备份，除非已存在相同备份）\n")
	fmt.Fprintf(os.Stderr, "  c:    把当前文件内容克隆为一份新备份\n")
	fmt.Fprintf(os.Stderr, "  d/D:  删除选中的备份（d 需确认，D 直接删）\n")
}

func main() {
	if len(os.Args) != 2 {
		usage()
		os.Exit(2)
	}

	target := os.Args[1]
	abs, err := filepath.Abs(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法解析路径 %s: %v\n", target, err)
		os.Exit(1)
	}

	entries, err := backup.List(abs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "扫描备份文件失败: %v\n", err)
		os.Exit(1)
	}
	if len(entries) == 0 {
		fmt.Fprintf(os.Stderr, "未找到 %s，同目录下也没有 %s.* 备份文件\n",
			filepath.Base(abs), filepath.Base(abs))
		os.Exit(1)
	}

	m := ui.New(abs, entries)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "程序运行失败: %v\n", err)
		os.Exit(1)
	}
}
