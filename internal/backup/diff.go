// Package backup 的 diff 能力：计算两份文件内容之间的行级差异（diff）。
//
// 算法采用 Myers 贪心算法（与 git 同源），输出按原来顺序排列的行序列，
// 每行标记为 Context（两边相同）/ Added（仅当前文件有）/ Removed（仅母本有）。
// 本文件无 UI 依赖，可独立单元测试。
package backup

import "strings"

// DiffKind 表示 diff 结果中一行的类型。
type DiffKind int

const (
	// DiffContext 表示两边文件都包含的相同行（差异上下文）。
	DiffContext DiffKind = iota
	// DiffAdded 表示仅存在于 b（当前文件）的行，即新增/修改后的行。
	DiffAdded
	// DiffRemoved 表示仅存在于 a（母本）的行，即被删除/修改前的行。
	DiffRemoved
)

// DiffLine 表示 diff 结果中的一行。
type DiffLine struct {
	Kind DiffKind // 行类型：Context / Added / Removed
	Text string   // 行文本（不含行尾换行符；行尾 \r 已剥离，使 CRLF 与 LF 文件可比）
	NumA int      // 该行在 a（母本）中的 1 起始行号；Added 行为 0
	NumB int      // 该行在 b（当前文件）中的 1 起始行号；Removed 行为 0
}

// SplitLines 将文件内容拆成行列表。
//
// 规则：按 \n 切分；内容为空返回 nil（0 行）；末尾换行不产生多余空行；
// 每行剥离行尾 \r，使 CRLF 与 LF 两种换行风格的文件逐行内容可比。
// 如 "a\nb\n" → ["a", "b"]；"" → nil；"\n" → [""]。
func SplitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	s := strings.TrimSuffix(string(data), "\n")
	parts := strings.Split(s, "\n")
	for i := range parts {
		parts[i] = strings.TrimSuffix(parts[i], "\r")
	}
	return parts
}

// Diff 计算 a → b 的行级差异，结果按行顺序排列。
//
// 返回 nil 表示两文件内容为空且无差异；两文件行内容完全相同但空行结构
// 不同（如 "" 与 "\n"）仍会如实反映差异。对比结果适合直接逐行渲染：
// 调用方可按 Kind 着色（绿色 Added / 红色 Removed / 灰色 Context），
// 并利用 NumA / NumB 显示行号帮助定位修改位置。
func Diff(a, b []byte) []DiffLine {
	la, lb := SplitLines(a), SplitLines(b)

	// Myers 贪心算法：在 (0,0)→(n,m) 的编辑图上找最短编辑路径。
	// k = x - y 为对角线编号，v[k] 记录到达该对角线的最远 x 坐标。
	n, m := len(la), len(lb)
	max := n + m
	if max == 0 {
		return nil
	}
	offset := max // k 平移量，使索引 offset+k 恒为非负
	v := make([]int, 2*max+1)

	// trace[d] 保存第 d 步迭代开始前的 v 快照，用于回溯还原编辑路径。
	// 最坏情况下 D = n+m，步数与索引空间均为 O(n+m)。
	trace := make([][]int, 0, max+1)
	dFound := -1
	for d := 0; d <= max && dFound < 0; d++ {
		prev := make([]int, 2*max+1)
		copy(prev, v)
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && prev[offset+k-1] < prev[offset+k+1]) {
				// 从上方下来（插入操作，消耗 b 的一行）。
				x = prev[offset+k+1]
			} else {
				// 从左侧过来（删除操作，消耗 a 的一行）。
				x = prev[offset+k-1] + 1
			}
			y := x - k
			// 沿相同行的对角线延伸（snake）。
			for x < n && y < m && la[x] == lb[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				dFound = d
				break
			}
		}
		trace = append(trace, prev)
	}

	// 从终点 (n, m) 沿 trace 回溯，还原每一步操作（逆序生成后整体反转）。
	lines := make([]DiffLine, 0, n+m)
	x, y := n, m
	for d := dFound; d > 0; d-- {
		prev := trace[d]
		k := x - y
		var prevK int
		if k == -d || (k != d && prev[offset+k-1] < prev[offset+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := prev[offset+prevK]
		prevY := prevX - prevK
		// 公共前缀（snake）段：两边各消耗一行，记为上下文。
		for x > prevX && y > prevY {
			lines = append(lines, DiffLine{Kind: DiffContext, Text: la[x-1], NumA: x, NumB: y})
			x--
			y--
		}
		if x == prevX {
			// 插入行：仅 b 有（当前文件新增）。
			lines = append(lines, DiffLine{Kind: DiffAdded, Text: lb[y-1], NumB: y})
			y--
		} else {
			// 删除行：仅 a 有（母本独有）。
			lines = append(lines, DiffLine{Kind: DiffRemoved, Text: la[x-1], NumA: x})
			x--
		}
	}
	// d=0 的残余段：起点 (0,0) 到第一条编辑之间的公共前缀。
	for x > 0 && y > 0 {
		lines = append(lines, DiffLine{Kind: DiffContext, Text: la[x-1], NumA: x, NumB: y})
		x--
		y--
	}
	for x > 0 {
		lines = append(lines, DiffLine{Kind: DiffRemoved, Text: la[x-1], NumA: x})
		x--
	}
	for y > 0 {
		lines = append(lines, DiffLine{Kind: DiffAdded, Text: lb[y-1], NumB: y})
		y--
	}
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines
}
