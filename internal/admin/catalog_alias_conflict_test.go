package admin

// 别名是「对外 ID 覆盖」：把 A 的别名设成 B 的官方 ID，请求 B 会被还原成 A
// 的上游模型，而 B 在清单里还挂着。这一组用例锁死三道闸：保存时拒绝、应用时
// 兜底、自身重复保存不误伤。

import (
	"encoding/json"
	"net/http"
	"testing"

	"strings"

	"freetier2api-plugin/cpasdk/pluginapi"
)

func seedModels(t *testing.T, c *Catalog, vendorID string, ids ...string) {
	t.Helper()
	models := make([]pluginapi.ModelInfo, 0, len(ids))
	for _, id := range ids {
		models = append(models, pluginapi.ModelInfo{ID: id})
	}
	if !c.CacheProbedModels(vendorID, models) {
		t.Fatalf("seed %s: CacheProbedModels 拒绝了空清单", vendorID)
	}
}

func TestAliasConflictsAgainstOtherModelID(t *testing.T) {
	c := newTestCatalog(t)
	seedModels(t, c, "workbuddycn", "glm-5.2", "glm-5.3")

	if got := c.AliasConflicts("workbuddycn", "glm-5.2", "glm-5.3"); got != "glm-5.3" {
		t.Fatalf("撞其它模型官方 ID 应报 %q，得到 %q", "glm-5.3", got)
	}
	// 自己的官方 ID 不算冲突（等价于清空别名）。
	if got := c.AliasConflicts("workbuddycn", "glm-5.2", "glm-5.2"); got != "" {
		t.Fatalf("别名等于自身 ID 不该报冲突，得到 %q", got)
	}
	// 跨渠道同名不算冲突：别名按供应商作用域生效。
	if got := c.AliasConflicts("codearts", "glm-5.2", "glm-5.3"); got != "" {
		t.Fatalf("跨渠道不该报冲突，得到 %q", got)
	}
	// 撞自动渠道标识后的名字同样要拦（对外 ID 一样会重复）。
	if got := c.AliasConflicts("workbuddycn", "glm-5.2", "workbuddycn-glm-5.3"); got != "glm-5.3" {
		t.Fatalf("撞渠道标识名应报 %q，得到 %q", "glm-5.3", got)
	}
}

func TestAliasConflictAllowsRepeatedSave(t *testing.T) {
	c := newTestCatalog(t)
	seedModels(t, c, "workbuddycn", "glm-5.2", "glm-5.3")
	c.SetAlias("workbuddycn", "glm-5.2", "glm-max")
	// 重复保存同一个别名不能被自己挡住（否则页面每次点保存都报错）。
	if got := c.AliasConflicts("workbuddycn", "glm-5.2", "glm-max"); got != "" {
		t.Fatalf("重复保存同一别名不该报冲突，得到 %q", got)
	}
	// 换绑到另一个模型时，撞的是别人而不是自己，仍应放行。
	if got := c.AliasConflicts("workbuddycn", "glm-5.3", "glm-max"); got != "" {
		t.Fatalf("同一别名换绑不该报冲突，得到 %q", got)
	}
}

func TestApplyAliasesDropsCollidingManualAlias(t *testing.T) {
	c := newTestCatalog(t)
	seedModels(t, c, "workbuddycn", "glm-5.2", "glm-5.3")
	// 绕过 handler 直接写入冲突别名，模拟「别名先存、清单后变」的存量配置。
	c.SetAlias("workbuddycn", "glm-5.2", "glm-5.3")

	out := c.ApplyAliases("workbuddycn", []pluginapi.ModelInfo{{ID: "glm-5.2"}, {ID: "glm-5.3"}})
	if len(out) != 2 {
		t.Fatalf("模型数量变了：%d", len(out))
	}
	if out[0].ID != "glm-5.2" {
		t.Fatalf("撞车的别名必须被放弃，得到 %q", out[0].ID)
	}
	// 被撞的模型保持自己的官方 ID（可能带渠道标识，取决于 tag 模式），
	// 关键是不能被别名的名字顶掉。
	if !strings.HasSuffix(out[1].ID, "glm-5.3") {
		t.Fatalf("被撞的模型 ID 不该变，得到 %q", out[1].ID)
	}
	// 两个对外 ID 必须唯一——这正是冲突要拦的原因。
	if out[0].ID == out[1].ID {
		t.Fatal("应用别名后出现了重复的对外 ID")
	}
}

func TestModelAliasHandlerRejectsConflict(t *testing.T) {
	service := newService(t)
	seedModels(t, service.deps.Model(), "workbuddycn", "glm-5.2", "glm-5.3")

	body, _ := json.Marshal(map[string]any{"model": "workbuddycn:glm-5.2", "alias": "glm-5.3"})
	response := service.modelAliasHandler(pluginapi.ManagementRequest{
		Method: http.MethodPost, Body: body,
	})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d，期望 400（body=%s）", response.StatusCode, response.Body)
	}
	var payload map[string]any
	if errUnmarshal := json.Unmarshal(response.Body, &payload); errUnmarshal != nil {
		t.Fatalf("body 不是 JSON：%v", errUnmarshal)
	}
	if payload["error"] == nil || payload["error"] == "" {
		t.Fatalf("缺少可读的 error：%s", response.Body)
	}
	// 别名没有写进状态：拒绝必须是硬拒绝，不能「先存后报错」。
	if got := service.deps.Model().AliasFor("workbuddycn", "glm-5.2"); got != "" {
		t.Fatalf("被拒绝的别名却写进了状态：%q", got)
	}
}
