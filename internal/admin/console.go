// 本文件提供内嵌的控制台页面。
//
// 页面是纯静态 HTML（不含账号、额度或凭证），通过管理接口按需拉取数据；
// 渲染动态数据统一走 textContent，避免把上游内容当成 HTML 注入。
//
// 页面按「结构段」拆在 page/ 目录下的多个片段里（00-head / 10-body /
// 20-core / 30-accounts / 40-quota / 50-models / 60-views / 70-login /
// 80-columns），启动时按文件名升序拼成一份再注入 base path。
//
// 拆片的理由是维护成本：单个三千多行的 HTML 文件里改一处 UI，diff 与
// 定位都靠行号；按「头部样式 / 骨架 / 各功能区脚本」切开后，改动落在
// 对应文件里，评审时能一眼看出影响面。
//
// 为什么不用 html/template：页面里 95% 是 JS，html/template 的上下文
// 自动转义会改写脚本内容（例如把 JS 里的比较运算当模板语法），既拿不到
// 收益，还会引入难以排查的转义差异。这里只做纯字符串拼接 + 一次
// __BASE__ 替换，输出与拆分前逐字节一致（见 console_test.go）。

package admin

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed page/*.html
var pageFS embed.FS

// pagePattern 是页面片段的匹配模式；文件名前缀决定拼接顺序。
const pagePattern = "page/*.html"

// page 是拼接好的完整页面（包级初始化时组装一次）。
var page = mustAssemblePage(pageFS)

// mustAssemblePage 组装页面片段；组装失败时 panic。
//
// 片段缺失属于打包错误（embed 已保证文件存在，只有改名/清空会触发），
// 与其让控制台静默返回半截页面，不如在插件加载时就崩掉。
func mustAssemblePage(fsys fs.FS) string {
	assembled, err := assemblePage(fsys)
	if err != nil {
		panic(fmt.Sprintf("控制台页面组装失败：%v", err))
	}
	return assembled
}

// assemblePage 按文件名升序拼接全部片段。
func assemblePage(fsys fs.FS) (string, error) {
	names, errGlob := fs.Glob(fsys, pagePattern)
	if errGlob != nil {
		return "", errGlob
	}
	if len(names) == 0 {
		return "", fmt.Errorf("没有匹配到任何页面片段（%s）", pagePattern)
	}
	sort.Strings(names)
	var builder strings.Builder
	for _, name := range names {
		raw, errRead := fs.ReadFile(fsys, name)
		if errRead != nil {
			return "", errRead
		}
		if len(raw) == 0 {
			return "", fmt.Errorf("页面片段 %s 是空的", name)
		}
		builder.Write(raw)
	}
	return builder.String(), nil
}

// Page 返回控制台页面（注入管理接口的 base path）。
func Page(basePath string) string {
	return strings.NewReplacer("__BASE__", basePath).Replace(page)
}
