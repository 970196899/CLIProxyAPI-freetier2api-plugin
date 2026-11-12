package admin

// 本文件锁定「页面片段 → 整页」的拼装不变量。
//
// 页面拆成 page/*.html 之后，最大的风险不是逻辑，而是拼装：少拼一段、顺序
// 错位、片段被清空，浏览器都不会报错——只会少一块 UI 或整段脚本失效，而
// 单测全绿。这里把拼装结果的结构性约束钉死。

import (
	"io/fs"
	"strings"
	"testing"
)

// TestPageAssemblesAllFragments 验证每个片段都被拼进整页，且顺序按文件名。
func TestPageAssemblesAllFragments(t *testing.T) {
	names, errGlob := fs.Glob(pageFS, pagePattern)
	if errGlob != nil {
		t.Fatalf("glob 页面片段：%v", errGlob)
	}
	// 片段数量变化必须是有意的：新增功能区要同时更新这里。
	if len(names) != 9 {
		t.Fatalf("页面片段数量为 %d，期望 9（新增/删除片段请同步更新本断言）", len(names))
	}
	page := Page("/plugins/freetier")
	previous := -1
	for _, name := range names {
		raw, errRead := fs.ReadFile(pageFS, name)
		if errRead != nil {
			t.Fatalf("读片段 %s：%v", name, errRead)
		}
		// 片段首行的锚点必须出现在整页里，且严格递增——少拼一段或顺序错位
		// 都会在这里断掉。
		anchor := firstNonEmptyLine(string(raw))
		index := strings.Index(page, anchor)
		if index < 0 {
			t.Fatalf("片段 %s 的内容没有出现在整页里（锚点 %q）", name, anchor)
		}
		if index <= previous {
			t.Fatalf("片段 %s 的拼接顺序错位（位置 %d，上一片段 %d）", name, index, previous)
		}
		previous = index
	}
}

// TestPageStructureIsComplete 验证整页骨架完整、base path 已注入。
func TestPageStructureIsComplete(t *testing.T) {
	page := Page("/plugins/freetier")
	if !strings.HasPrefix(page, "<!DOCTYPE html>") {
		t.Fatal("整页没有以 <!DOCTYPE html> 开头：首个片段缺失或顺序错位")
	}
	if !strings.HasSuffix(strings.TrimRight(page, "\n"), "</html>") {
		t.Fatal("整页没有以 </html> 结尾：末个片段缺失")
	}
	if strings.Contains(page, "__BASE__") {
		t.Fatal("整页里仍残留 __BASE__ 占位符：base path 没有替换干净")
	}
	// 页面里 MGMT 是 "/v0/management" + BASE 拼出来的，断言两段都在即可。
	if !strings.Contains(page, `var MGMT = "/v0/management" + BASE`) || !strings.Contains(page, `var BASE = "/plugins/freetier"`) {
		t.Fatal("管理接口前缀没有注入正确的 base path")
	}
	// 脚本只有一段：拆分是「文件级」，不是「多 <script> 标签」，避免重复的
	// 顶层作用域把 IIFE 里的私有状态拆散。
	if got := strings.Count(page, "<script>"); got != 1 {
		t.Fatalf("整页里有 %d 个 <script> 标签，期望 1", got)
	}
	if got := strings.Count(page, "</script>"); got != 1 {
		t.Fatalf("整页里有 %d 个 </script> 标签，期望 1", got)
	}
}

// TestPageKeepsCriticalEntryPoints 验证关键函数没在拆分中丢失。
func TestPageKeepsCriticalEntryPoints(t *testing.T) {
	page := Page("/plugins/freetier")
	for _, literal := range []string{
		"function api(",
		"function loadStatus(",
		"function loadQuotas(",
		"function loadModels(",
		"function renderAccounts(",
		"function deobfuscateAll(",
		"document.getElementById(\"btnSubmitCallback\")",
	} {
		if !strings.Contains(page, literal) {
			t.Fatalf("整页缺少 %q：拆分时丢了关键入口", literal)
		}
	}
}

// TestPageHasRequestTimeout 验证前端请求有超时兜底。
//
// 没有超时的表现不是「慢」而是「假死」：请求挂住时调用方永远停在「查询中…」，
// 页面既不报错也不重试。这条断言把两档超时与中断翻译都钉住。
func TestPageHasRequestTimeout(t *testing.T) {
	page := Page("/plugins/freetier")
	for _, literal := range []string{
		"var API_TIMEOUT_MS = 60000",
		"var API_SLOW_TIMEOUT_MS = 240000",
		"new AbortController()",
		`err.name === "AbortError"`,
		"请求超时（",
	} {
		if !strings.Contains(page, literal) {
			t.Fatalf("页面缺少 %q：请求超时没有兜底", literal)
		}
	}
	// 额度批量必须走慢档，否则正常的三分钟批次会被 60 秒掐断。
	if !strings.Contains(page, `{ timeout: API_SLOW_TIMEOUT_MS }`) {
		t.Fatal("额度批量请求没有使用慢档超时")
	}
}

// TestPageReportsErrorsOnce 验证错误只报一次：loadStatus/loadQuotas 不再 rethrow。
//
// rethrow 会让下游 catch 再弹一次同样的横幅，而顶层没人接这个 rejection，
// 控制台只会多一条 unhandled rejection。
func TestPageReportsErrorsOnce(t *testing.T) {
	page := Page("/plugins/freetier")
	if strings.Contains(page, `banner("err", err.message); throw err;`) {
		t.Fatal("页面里仍有「报错后 rethrow」的写法，会重复报错")
	}
}

// firstNonEmptyLine 返回内容里第一行非空文本（去掉首尾空白）。
func firstNonEmptyLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
