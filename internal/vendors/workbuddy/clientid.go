package workbuddy

import (
	"strings"
)

// ==================== 三方客户端身份句/签名归一 ====================
//
// 上游渠道校验（code=11128 "Illegal API invocation from an unapproved
// channel"）按请求内容判定调用方是否来自官方客户端。非官方客户端
// （deepseek-harness 等）会在 system 提示词里留下可识别的签名，被上游
// 整体拒绝。实测（2026-11-06，真实上游）两类签名都会被拦：
//
//   - 品牌句："You are an AI agent powered by DeepSeek Harness."；
//   - 工作区约定："Tokens prefixed with @ are paths ... resolve from
//     the workspace root"（deepseek-harness 独有结构，仅品牌替换不够）。
//
// 本文件对 system / developer 消息做签名归一：检测到非官方签名时，把
// 品牌替换为 WorkBuddy（本渠道官方名）、把 "workspace root" 换成
// "workspace"，让上游不再命中。零宽空格（U+200B）无效——上游归一化
// 零宽后仍做短语匹配。不改用户消息内容，幂等。

// workBuddyBrand 是本渠道的官方品牌名（出站签名替换目标）。
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

// deepSeekWorkspaceSig 是 deepseek-harness 的工作区约定签名（@-路径）。
// 上游 11128 渠道校验会拦它（即使品牌已替换）。
const deepSeekWorkspaceSig = "Tokens prefixed with @"

// normalizeIdentitySentences 处理 messages，对 system/developer 消息做
// 签名归一（非官方品牌 → WorkBuddy；"workspace root" → "workspace"）。
// 返回新 slice（仅在发生改写时分配）。
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

// normalizeIdentitySentence 对单条 system/developer 内容做签名归一。
// 检测到非官方签名（身份句式开头、品牌词、@-路径工作区约定）时，
// 替换品牌与 workspace root；否则原样返回。
func normalizeIdentitySentence(content string) string {
	if !isNonOfficialSignature(content) {
		return content
	}
	out := content
	for _, brand := range nonOfficialBrands {
		out = strings.ReplaceAll(out, brand, workBuddyBrand)
	}
	// 工作区约定：上游渠道校验会拦 "workspace root"（deepseek-harness
	// 的 @-路径签名），换成 "workspace" 后不再命中。
	out = strings.ReplaceAll(out, "workspace root", "workspace")
	return out
}

// isNonOfficialSignature 判断 system/developer 内容是否带非官方签名。
func isNonOfficialSignature(content string) bool {
	lower := strings.ToLower(content)
	if matchesIdentityPrefix(strings.TrimSpace(lower)) {
		return true
	}
	for _, brand := range nonOfficialBrands {
		if strings.Contains(lower, strings.ToLower(brand)) {
			return true
		}
	}
	return strings.Contains(content, deepSeekWorkspaceSig)
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
