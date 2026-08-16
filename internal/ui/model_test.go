package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"crw/internal/backup"
)

// newTestModel 在临时目录中构造 base + 若干备份，返回模型与目录路径。
// 内容约定：base 内容 "current"，备份 _1 内容 "old1"，备份 _2 内容 "old2"。
func newTestModel(t *testing.T, files map[string]string) (*Model, string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("创建测试文件失败 %s: %v", name, err)
		}
	}
	base := filepath.Join(dir, "config.yaml")
	entries, err := backup.List(base)
	if err != nil {
		t.Fatalf("backup.List() 出错: %v", err)
	}
	return New(base, entries), dir
}

// key 构造指定按键消息。
func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "pgdn":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case "f1":
		return tea.KeyMsg{Type: tea.KeyF1}
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// send 向模型发送按键并断言程序未退出。
func send(t *testing.T, m *Model, s string) {
	t.Helper()
	_, cmd := m.Update(key(s))
	if cmd != nil {
		t.Fatalf("按键 %q 意外返回退出命令", s)
	}
}

// fileExists 检查文件是否存在。
func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

// readFileStr 读取文件内容为字符串。
func readFileStr(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	return string(data)
}

// listBackups 返回目录内所有 config.yaml.bak* 文件路径。
func listBackups(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "config.yaml.bak*"))
	if err != nil {
		t.Fatalf("Glob 失败: %v", err)
	}
	return matches
}

func TestReplaceWritesBackupAndAutoSavesCurrent(t *testing.T) {
	m, dir := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "old1",
		"config.yaml.bak.20260802": "old2",
	})

	// 光标默认在首位（目标文件），下移到第一个备份。
	send(t, m, "j")
	if m.entries[m.cursor].Name != "config.yaml.bak.20260802" {
		t.Fatalf("光标应落在最新备份, got %q", m.entries[m.cursor].Name)
	}

	// 回车替换：目标内容变为 old2，且当前内容 "current" 被自动备份。
	send(t, m, "enter")

	if got := readFileStr(t, m.basePath); got != "old2" {
		t.Errorf("替换后目标内容 = %q, want %q", got, "old2")
	}
	backups := listBackups(t, dir)
	if len(backups) != 3 {
		t.Fatalf("自动备份后备份数 = %d, want 3", len(backups))
	}
	// 自动备份的文件内容必须是替换前的当前内容。
	for _, b := range backups {
		if readFileStr(t, b) == "current" {
			return // 找到自动备份，验证通过
		}
	}
	t.Error("未找到内容为 current 的自动备份文件")
}

func TestReplaceSkipsBackupWhenContentAlreadyInList(t *testing.T) {
	m, dir := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "current", // 与当前内容相同
		"config.yaml.bak.20260802": "old2",
	})

	send(t, m, "j") // 选中 20260802
	send(t, m, "enter")

	if got := readFileStr(t, m.basePath); got != "old2" {
		t.Errorf("替换后目标内容 = %q, want %q", got, "old2")
	}
	// 当前内容已存在于备份列表，不应新增备份。
	if n := len(listBackups(t, dir)); n != 2 {
		t.Errorf("备份数 = %d, want 2（不应新增）", n)
	}
}

func TestEnterOnSingleFileDoesNothing(t *testing.T) {
	m, dir := newTestModel(t, map[string]string{"config.yaml": "current"})

	send(t, m, "enter")

	if got := readFileStr(t, m.basePath); got != "current" {
		t.Errorf("单文件回车不应改变内容, got %q", got)
	}
	if n := len(listBackups(t, dir)); n != 0 {
		t.Errorf("单文件回车不应产生备份, 备份数 = %d", n)
	}
	if !strings.Contains(m.status, "只有一个文件") {
		t.Errorf("应提示单文件状态, got %q", m.status)
	}
}

func TestEnterOnBaseWithBackupsDoesNothing(t *testing.T) {
	m, dir := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "old1",
	})
	// 光标停在目标文件本身。
	send(t, m, "enter")

	if got := readFileStr(t, m.basePath); got != "current" {
		t.Errorf("选中当前文件回车不应改变内容, got %q", got)
	}
	if n := len(listBackups(t, dir)); n != 1 {
		t.Errorf("不应产生新备份, 备份数 = %d", n)
	}
}

func TestCopyClonesWhenNoDuplicate(t *testing.T) {
	m, dir := newTestModel(t, map[string]string{"config.yaml": "current"})

	send(t, m, "c")

	backups := listBackups(t, dir)
	if len(backups) != 1 {
		t.Fatalf("c 应克隆出 1 个备份, got %d", len(backups))
	}
	if got := readFileStr(t, backups[0]); got != "current" {
		t.Errorf("克隆备份内容 = %q, want %q", got, "current")
	}
	// 光标应跳到新备份项。
	if m.entries[m.cursor].IsBase {
		t.Error("克隆后光标应跳到新备份项，而不是停在当前文件")
	}
}

func TestCopyJumpsToDuplicateInsteadOfCloning(t *testing.T) {
	m, dir := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "old1",
		"config.yaml.bak.20260802": "current", // 与当前内容相同
	})

	send(t, m, "c")

	if n := len(listBackups(t, dir)); n != 2 {
		t.Errorf("存在相同备份时不应克隆新文件, 备份数 = %d", n)
	}
	if got := m.entries[m.cursor].Name; got != "config.yaml.bak.20260802" {
		t.Errorf("光标应跳到相同内容备份, got %q", got)
	}
}

func TestDeleteWithConfirm(t *testing.T) {
	m, dir := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "old1",
	})
	send(t, m, "j") // 选中备份

	// d → 弹出确认框，文件暂不删除。
	send(t, m, "d")
	if m.mode != modeConfirmDelete {
		t.Fatalf("按 d 后应进入确认模式, mode = %d", m.mode)
	}
	target := filepath.Join(dir, "config.yaml.bak.20260801")
	if !fileExists(t, target) {
		t.Fatal("确认前文件不应被删除")
	}

	// 确认 y → 删除。
	send(t, m, "y")
	if fileExists(t, target) {
		t.Error("确认后文件应被删除")
	}
	if m.mode != modeNormal {
		t.Error("删除后应回到正常模式")
	}
	if n := len(m.entries); n != 1 {
		t.Errorf("删除后列表条目数 = %d, want 1", n)
	}
}

func TestDeleteConfirmCancelKeepsFile(t *testing.T) {
	m, dir := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "old1",
	})
	send(t, m, "j")
	send(t, m, "d")
	send(t, m, "n") // 取消

	target := filepath.Join(dir, "config.yaml.bak.20260801")
	if !fileExists(t, target) {
		t.Error("取消确认后文件应保留")
	}
	if m.mode != modeNormal {
		t.Error("取消后应回到正常模式")
	}
}

func TestDeleteWithoutConfirm(t *testing.T) {
	m, dir := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "old1",
	})
	send(t, m, "j")
	send(t, m, "D") // 大写 D：直接删除

	target := filepath.Join(dir, "config.yaml.bak.20260801")
	if fileExists(t, target) {
		t.Error("D 键应直接删除文件")
	}
	if m.mode != modeNormal {
		t.Error("D 键不应进入确认模式")
	}
}

func TestDeleteBaseBlocked(t *testing.T) {
	m, _ := newTestModel(t, map[string]string{"config.yaml": "current"})

	send(t, m, "d") // 光标在目标文件上

	if m.mode == modeConfirmDelete {
		t.Fatal("目标文件不应进入删除确认")
	}
	if !fileExists(t, m.basePath) {
		t.Fatal("目标文件不应被删除")
	}
	if !strings.Contains(m.status, "不能删除") {
		t.Errorf("应提示不能删除当前文件, got %q", m.status)
	}

	// D 同样被拦截。
	send(t, m, "D")
	if !fileExists(t, m.basePath) {
		t.Fatal("目标文件不应被 D 删除")
	}
}

func TestRefreshRebuildsList(t *testing.T) {
	m, dir := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "old1",
	})
	// 外部新增一个备份文件。
	extra := filepath.Join(dir, "config.yaml.bak.20260803")
	if err := os.WriteFile(extra, []byte("extra"), 0o644); err != nil {
		t.Fatalf("写外部文件失败: %v", err)
	}

	send(t, m, "r")

	if n := len(m.entries); n != 3 {
		t.Errorf("刷新后条目数 = %d, want 3", n)
	}
}

func TestPreviewFollowsCursor(t *testing.T) {
	m, dir := newTestModel(t, map[string]string{
		"config.yaml":              "current",
		"config.yaml.bak.20260801": "old1",
	})
	if m.previewPath != m.basePath {
		t.Errorf("初始预览应为目标文件, got %q", m.previewPath)
	}
	send(t, m, "j")
	if got := m.previewPath; got != filepath.Join(dir, "config.yaml.bak.20260801") {
		t.Errorf("下移后预览应跟随备份文件, got %q", got)
	}
}

func TestFocusToggleTab(t *testing.T) {
	m, _ := newTestModel(t, map[string]string{"config.yaml": "current"})
	if m.focus != focusList {
		t.Fatalf("初始焦点应在列表, got %v", m.focus)
	}
	send(t, m, "tab")
	if m.focus != focusPreview {
		t.Error("Tab 应切换到预览焦点")
	}
	send(t, m, "tab")
	if m.focus != focusList {
		t.Error("再次 Tab 应切回列表焦点")
	}
}
