package auth

// 本文件缓存「按 auth_index 取凭证原文」这一步。
//
// 为什么需要：Load 对每个账号都要发一次宿主 RPC（hostbridge.AuthJSONAndPath），
// 30 个账号就是 30 次往返；页面每次刷新（/status + /quotas + 任务中心）都会
// 触发整轮枚举，串行的 RPC 把「加载中」拖成秒级。凭证文件本身以分钟为尺度
// 变化（重新登录、换号），10 秒缓存不会读到过期内容。
//
// 一致性由三件事兜底，任一不符即重新读：
//   - TTL（默认 10s）；
//   - 文件路径变化（同一 auth_index 换了文件）；
//   - 文件修改时间变化（宿主重写了凭证，本机 stat 即可发现）。
//
// 只缓存**成功读取**的结果：失败多半是宿主 RPC 的瞬时问题，缓存失败会让
// 用户点「刷新」也救不回来。

import (
	"sync"
	"time"
)

// authJSONTTL 是凭证原文的缓存时长。
const authJSONTTL = 10 * time.Second

// authJSONEntry 是一条缓存记录。
type authJSONEntry struct {
	raw      []byte
	filePath string
	path     string    // 来源文件路径（用于失效判定）
	modTime  time.Time // 来源文件修改时间（用于失效判定）
	at       time.Time // 写入时刻
}

// authJSONCache 是带 TTL 的凭证原文缓存（并发安全）。
type authJSONCache struct {
	mu      sync.Mutex
	entries map[string]authJSONEntry
	ttl     time.Duration
	now     func() time.Time
}

// newAuthJSONCache 构造缓存；ttl <= 0 时使用默认值。
func newAuthJSONCache(ttl time.Duration) *authJSONCache {
	if ttl <= 0 {
		ttl = authJSONTTL
	}
	return &authJSONCache{
		entries: map[string]authJSONEntry{},
		ttl:     ttl,
		now:     time.Now,
	}
}

// get 返回缓存副本；路径或修改时间不符时视为未命中。
//
// 返回的是**副本**：RawJSON 会被下游持有（账号结构体里），共享同一底层数组
// 时任何一个调用方就地改写都会污染其他调用方。
func (c *authJSONCache) get(key, path string, modTime time.Time) (authJSONEntry, bool) {
	if c == nil {
		return authJSONEntry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, okEntry := c.entries[key]
	if !okEntry {
		return authJSONEntry{}, false
	}
	if c.now().Sub(entry.at) > c.ttl {
		delete(c.entries, key)
		return authJSONEntry{}, false
	}
	if entry.path != path {
		delete(c.entries, key)
		return authJSONEntry{}, false
	}
	if !entry.modTime.IsZero() && !entry.modTime.Equal(modTime) {
		delete(c.entries, key)
		return authJSONEntry{}, false
	}
	copied := entry
	copied.raw = append([]byte(nil), entry.raw...)
	return copied, true
}

// put 写入一条缓存记录。
func (c *authJSONCache) put(key, path string, modTime time.Time, raw []byte, filePath string) {
	if c == nil || key == "" || len(raw) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// 账号会被删除/改名，对应的键再也取不到，只能靠这里顺手清理，避免
	// 长期运行后 map 只增不减（条目本身很小，但没上限就是泄漏）。
	if len(c.entries) > 256 {
		now := c.now()
		for existing, entry := range c.entries {
			if now.Sub(entry.at) > c.ttl {
				delete(c.entries, existing)
			}
		}
	}
	c.entries[key] = authJSONEntry{
		raw:      append([]byte(nil), raw...),
		filePath: filePath,
		path:     path,
		modTime:  modTime,
		at:       c.now(),
	}
}

// reset 清空缓存（凭证被本插件改写后调用，避免读到旧原文）。
func (c *authJSONCache) reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]authJSONEntry{}
}

// size 返回当前缓存条目数（诊断用）。
func (c *authJSONCache) size() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
