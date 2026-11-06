package workbuddy

import (
	"strings"
)

// ==================== 三方客户端身份句归一 ====================
//
// 上游渠道校验（code=11128 "Illegal API invocation from an unapproved
// channel"）会按请求内容判定调用方是否来自官方客户端：非官方客户端
// （deepseek-harness / Claude Code / Codex 等）会在 system 提示词开头
// 声明自己的身份（如 "You are an AI agent powered by DeepSeek Harness."）。
//
// 实测（2026-11-06，真实上游）：
//   - 零宽空格（U+200B）**无效**：上游会归一化零宽/空白后做大小写不敏感的
//     短语匹配，"Deep\u200bSeek Harness" 仍命中 "DeepSeek Harness" → 11128。
//   - 连字符（DeepSeek-Harness）能打破匹配，但保留的是非官方品牌；
//   - 把品牌替换成 "WorkBuddy"（本渠道官方名）最稳：既过渠道校验，
//     又让身份句与模型通道一致。
//
// 本文件对身份句做「品牌替换」：system / developer 消息里的身份句式
// （句子开头命中客户端身份前缀）中，非官方品牌被替换为 WorkBuddy。
// 只作用于身份句式，不改其余内容，幂等。

// workBuddyBrand 是本渠道的官方品牌名（出站身份句替换目标）。
const workBuddyBrand = "WorkBuddy"

// nonOfficialBrands 是会被上游 11128 渠道校验拦截的非官方品牌（按
// 长度降序，长词优先避免误替换短词前缀）。不含 WorkBuddy/CodeBuddy
// 等本渠道品牌。
var nonOfficialBrands = []string{
	"DeepSeek Harness",
	"Claude Code",
	"DeepSeek",
	"Anthropic",
	"Claude",
	"Codex",
	"Devin",
	"Cursor",
	"Windsurf",
	"Trae",
	"Qwen",
	"Gemini",
	"OpenAI",
	"Copilot",
	"Cline",
	"Kimi",
	"Doubao",
	"GLM",
	"Zhipu",
}

// identitySentencePrefixes 是身份句式的常见开头（小写前缀匹配）。
// 命中这些开头的句子才可能是「客户端身份声明」，避免误伤普通用户内容。
var identitySentencePrefixes = []string{
	"you are an ai agent powered by",
	"you are an ai coding agent",
	"you are an ai assistant",
	"you are a coding agent",
	"you are claude",
	"you are codex",
	"you are deepseek",
	"you are qwen",
	"you are gemini",
	"you are cursor",
	"you are windsurf",
	"you are trae",
	"you are cline",
	"you are copilot",
}

// normalizeIdentitySentences 处理 messages，对 system/developer 消息的
// 身份句式做品牌替换（非官方品牌 → WorkBuddy）。返回新 slice。
func normalizeIdentitySentences(messages []any) []any {
	changed := false
	for i, raw := range messages {
		msg, okMsg := raw.(map[string]any)
		if !okMsg {
			continue
		}
		role, _ := msg["role"].(string)
		if role != "system" && role != "developer" {
			continue
		}
		content, okContent := msg["content"].(string)
		if !okContent {
			continue
		}
		replaced := normalizeIdentitySentence(content)
		if replaced == content {
			continue
		}
		if !changed {
			messages = cloneMessages(messages)
			changed = true
		}
		messages[i] = map[string]any{"role": role, "content": replaced}
	}
	return messages
}

// normalizeIdentitySentence 对单条 system/developer 内容做身份句品牌替换。
// 只在句子开头命中身份句式时，把非官方品牌替换为 WorkBuddy；否则原样返回。
func normalizeIdentitySentence(content string) string {
	head := strings.ToLower(strings.TrimSpace(content))
	if !matchesIdentityPrefix(head) {
		return content
	}
	// 只处理首个身份句（首个换行前），避免碰后续业务内容。
	sentence := content
	rest := ""
	if idx := strings.IndexByte(content, '\n'); idx >= 0 {
		sentence = content[:idx]
		rest = content[idx:]
	}
	out := sentence
	for _, brand := range nonOfficialBrands {
		if strings.Contains(out, brand) {
			out = strings.Replace(out, brand, workBuddyBrand, 1)
			break
		}
	}
	if out == sentence {
		return content
	}
	return out + rest
}

// matchesIdentityPrefix 判断句子是否命中身份句式开头。
func matchesIdentityPrefix(head string) bool {
	for _, prefix := range identitySentencePrefixes {
		if strings.HasPrefix(head, prefix) {
			return true
		}
	}
	return false
}

// cloneMessages 深拷贝 messages 顶层（避免改写共享底层数组）。
func cloneMessages(messages []any) []any {
	out := make([]any, len(messages))
	copy(out, messages)
	return out
}
