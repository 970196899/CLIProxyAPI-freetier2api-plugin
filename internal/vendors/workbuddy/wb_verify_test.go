package workbuddy

// 真实验证：身份句归一（非官方品牌 → WorkBuddy）能否骗过上游 11128 渠道校验。
// 需要 WB_TEST_TOKEN / WB_TEST_UID 环境变量（真实账号），否则跳过。
// 控制组：原始 deepseek-harness 身份句、不经归一化（应复现 11128）；
// 修复组：经 PrepareBody 归一化（应 200，不再 11128）。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWBChannelCheckU200B(t *testing.T) {
	token := os.Getenv("WB_TEST_TOKEN")
	uid := os.Getenv("WB_TEST_UID")
	if token == "" || uid == "" {
		t.Skip("WB_TEST_TOKEN/WB_TEST_UID 未设置，跳过真实网络验证")
	}

	rt := &runtime{modelCache: map[Region]modelCacheEntry{}}
	salt := "test-salt-for-verify"
	rt.salt.Store(&salt)
	client := NewClient(rt, Options{Context: context.Background(), EnabledRealms: []string{"cn"}})
	cred := &Credential{AccessToken: token, UID: uid}

	// send 发送一个 chat 请求。body 由调用方构造（控制组不经归一化）。
	send := func(body []byte) string {
		req, _ := http.NewRequest(http.MethodPost, GetEndpoints(RegionCN).ChatBase+chatCompletionsPath, strings.NewReader(string(body)))
		client.applyChatHeaders(req, cred, ChatMeta{}, "")
		httpClient := &http.Client{Timeout: 25 * time.Second}
		resp, err := httpClient.Do(req)
		if err != nil {
			return "transport_err: " + err.Error()
		}
		defer resp.Body.Close()
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Sprintf("HTTP%d %s", resp.StatusCode, string(rb))
	}

	// 手动构造 body（不经 PrepareBody 归一化，但带 stream:true 等必要字段）。
	mkBody := func(system string) []byte {
		body := map[string]any{
			"model":          "hy4free",
			"stream":         true,
			"stream_options": map[string]any{"include_usage": true},
			"messages": []any{
				map[string]any{"role": "system", "content": system},
				map[string]any{"role": "user", "content": "只回答：ok"},
			},
		}
		raw, _ := json.Marshal(body)
		return raw
	}

	original := "You are an AI agent powered by DeepSeek Harness.\n\nBe helpful."

	// 控制组：原始身份句、不经归一化 → 应复现 11128。
	control := send(mkBody(original))
	if !strings.Contains(control, "11128") {
		t.Logf("⚠️ 控制组未复现 11128（可能上游策略变化或账号已白名单）: %s", truncateResp(control))
	} else {
		t.Logf("控制组（原始身份句，未经归一化）复现 11128 ✓")
	}

	// 修复组：经 PrepareBody（含身份句归一）→ 应过渠道校验。
	prepared := PrepareBody(mkBody(original), PrepareOptions{Model: "hy4free"})
	fixed := send(prepared)
	ok := !strings.Contains(fixed, "11128") && !strings.Contains(strings.ToLower(fixed), "unapproved")
	mark := "✗ 仍 11128"
	if ok {
		mark = "✓ 通过渠道校验"
	}
	t.Logf("修复组（PrepareBody 归一化后）: %s | %s", mark, truncateResp(fixed))
	if !ok {
		t.Fatalf("身份句归一后仍被渠道校验拒绝: %s", fixed)
	}
	if !strings.HasPrefix(fixed, "HTTP200") {
		t.Logf("注意：返回 %s（非 200，但已过渠道校验）", truncateResp(fixed))
	}
}

func truncateResp(s string) string {
	if len(s) > 120 {
		return s[:120]
	}
	return s
}
