package admin

// 额度批次的两条预算规则：单账号有独立上限，整批按波次扩容并封顶。
//
// 这组断言的价值在于防止「顺手调参」把两者调反——比如把单账号预算调到
// 比整批还大，尾部账号又会重新吃到耗尽的 ctx。

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestQuotaBatchBudgetGrowsWithWaves(t *testing.T) {
	// 少量账号：保持原有 3 分钟，不给小批量平白加预算。
	if got := quotaBatchBudget(1); got != requestTimeout {
		t.Fatalf("1 个账号预算 = %s，期望 %s", got, requestTimeout)
	}
	if got := quotaBatchBudget(batchConcurrency); got != requestTimeout {
		t.Fatalf("%d 个账号（一波）预算 = %s，期望 %s", batchConcurrency, got, requestTimeout)
	}
	// 多波次：预算必须随波次增长，否则尾部账号一定超时。
	small := quotaBatchBudget(batchConcurrency * 4)
	large := quotaBatchBudget(batchConcurrency * 10)
	if !(small < large) {
		t.Fatalf("预算没有随账号数增长：4 波 %s，10 波 %s", small, large)
	}
	if small <= requestTimeout {
		t.Fatalf("4 波预算 %s 不应等于基础值 %s", small, requestTimeout)
	}
	// 硬上限：账号再多也不能让一轮跑太久。
	if got := quotaBatchBudget(10000); got != quotaBatchMaxBudget {
		t.Fatalf("超大账号数预算 = %s，期望封顶 %s", got, quotaBatchMaxBudget)
	}
}

func TestQuotaAccountTimeoutFitsInsideBatch(t *testing.T) {
	// 单账号预算必须小于整批基础预算，否则「逐账号超时」永远轮不到触发。
	if quotaAccountTimeout >= requestTimeout {
		t.Fatalf("单账号预算 %s 不应大于等于整批基础预算 %s", quotaAccountTimeout, requestTimeout)
	}
	if quotaAccountTimeout <= 0 {
		t.Fatal("单账号预算必须为正")
	}
}

func TestQuotaErrorTextExplainsTimeout(t *testing.T) {
	// 超时与取消都要有可执行的中文说明，不能把裸错误直接怼给用户。
	for _, err := range []error{context.DeadlineExceeded, context.Canceled} {
		text := quotaErrorText(err)
		if text == err.Error() {
			t.Fatalf("%v 没有翻译成可读文案", err)
		}
		if text == "" {
			t.Fatal("文案为空")
		}
	}
	// 包了一层也要认出来（上游常用 fmt.Errorf("...: %w", ctx.Err())）。
	wrapped := errors.Join(errors.New("查询失败"), context.DeadlineExceeded)
	if got := quotaErrorText(wrapped); got == wrapped.Error() {
		t.Fatalf("包装后的超时没有识别出来：%q", got)
	}
	// 其它错误原样透出，不要吞掉上游正文。
	plain := errors.New("upstream 403 forbidden")
	if got := quotaErrorText(plain); got != plain.Error() {
		t.Fatalf("普通错误被改写：%q", got)
	}
}

func TestQuotaAccountTimeoutIsUsable(t *testing.T) {
	// 45 秒是给「单账号一次额度查询」的预算：太短会把慢供应商（Trae/CodeArts）
	// 正常请求掐掉，太长则整批退化成串行等待。
	if quotaAccountTimeout < 20*time.Second || quotaAccountTimeout > 90*time.Second {
		t.Fatalf("单账号预算 %s 超出合理区间 20s~90s", quotaAccountTimeout)
	}
}
