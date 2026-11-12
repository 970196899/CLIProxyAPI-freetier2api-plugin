package auth

// 凭证原文缓存的失效规则。
//
// 缓存一旦「该失效却没失效」，用户看到的就是改完凭证还显示旧账号；反过来
// 「不该失效却失效」只是多一次 RPC。所以这里把三条失效条件逐条钉死。

import (
	"testing"
	"time"
)

func newTestCache() (*authJSONCache, *time.Time) {
	cache := newAuthJSONCache(time.Second)
	clock := time.Unix(1700000000, 0)
	cache.now = func() time.Time { return clock }
	return cache, &clock
}

func TestAuthJSONCacheHitWithinTTL(t *testing.T) {
	cache, clock := newTestCache()
	cache.put("idx-1", "/data/a.json", time.Time{}, []byte(`{"k":1}`), "/data/a.json")
	*clock = clock.Add(500 * time.Millisecond)

	entry, okEntry := cache.get("idx-1", "/data/a.json", time.Time{})
	if !okEntry {
		t.Fatal("TTL 内应命中")
	}
	if string(entry.raw) != `{"k":1}` || entry.filePath != "/data/a.json" {
		t.Fatalf("缓存内容不对：raw=%s path=%s", entry.raw, entry.filePath)
	}
}

func TestAuthJSONCacheExpiresAfterTTL(t *testing.T) {
	cache, clock := newTestCache()
	cache.put("idx-1", "", time.Time{}, []byte(`{}`), "")
	*clock = clock.Add(2 * time.Second)

	if _, okEntry := cache.get("idx-1", "", time.Time{}); okEntry {
		t.Fatal("超过 TTL 必须未命中")
	}
	// 过期条目应被顺手删除，不能留在 map 里。
	if got := cache.size(); got != 0 {
		t.Fatalf("过期条目没有清掉：size=%d", got)
	}
}

func TestAuthJSONCacheInvalidatesOnPathChange(t *testing.T) {
	cache, _ := newTestCache()
	cache.put("idx-1", "/data/a.json", time.Time{}, []byte(`{"old":true}`), "/data/a.json")

	if _, okEntry := cache.get("idx-1", "/data/b.json", time.Time{}); okEntry {
		t.Fatal("auth_index 换了文件路径必须未命中")
	}
}

func TestAuthJSONCacheInvalidatesOnModTimeChange(t *testing.T) {
	cache, _ := newTestCache()
	original := time.Unix(1700000000, 0)
	cache.put("idx-1", "/data/a.json", original, []byte(`{"old":true}`), "/data/a.json")

	if _, okEntry := cache.get("idx-1", "/data/a.json", original); !okEntry {
		t.Fatal("同一 mtime 应命中")
	}
	// 宿主重写了凭证：mtime 变了，必须重新读。
	if _, okEntry := cache.get("idx-1", "/data/a.json", original.Add(time.Second)); okEntry {
		t.Fatal("mtime 变化必须未命中")
	}
}

func TestAuthJSONCacheReturnsCopy(t *testing.T) {
	cache, _ := newTestCache()
	cache.put("idx-1", "", time.Time{}, []byte(`{"k":1}`), "")

	first, _ := cache.get("idx-1", "", time.Time{})
	first.raw[0] = 'X'
	second, _ := cache.get("idx-1", "", time.Time{})
	if string(second.raw) != `{"k":1}` {
		t.Fatalf("调用方改写污染了缓存：%s", second.raw)
	}
}

func TestAuthJSONCacheSkipsEmptyPut(t *testing.T) {
	cache, _ := newTestCache()
	cache.put("", "", time.Time{}, []byte(`{}`), "")
	cache.put("idx-1", "", time.Time{}, nil, "")
	if got := cache.size(); got != 0 {
		t.Fatalf("空键/空内容不该入缓存：size=%d", got)
	}
}

func TestAuthJSONCachePrunesStaleEntries(t *testing.T) {
	cache, clock := newTestCache()
	for i := 0; i < 300; i++ {
		cache.put(string(rune('a'+i%26))+string(rune('a'+i/26)), "", time.Time{}, []byte(`{}`), "")
	}
	// 全部过期后再写一条：清理应把过期的都丢掉，只剩新写的那条。
	*clock = clock.Add(5 * time.Second)
	cache.put("fresh", "", time.Time{}, []byte(`{}`), "")
	if got := cache.size(); got != 1 {
		t.Fatalf("过期条目没有被清理：size=%d", got)
	}
}
