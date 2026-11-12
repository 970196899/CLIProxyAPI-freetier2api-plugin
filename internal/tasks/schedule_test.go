package tasks

import (
	"testing"
	"time"
)

func TestNormalizeHoursAndNextWake(t *testing.T) {
	if got := NormalizeHours([]int{9, 9, 25, -1, 21}); len(got) != 2 || got[0] != 9 || got[1] != 21 {
		t.Fatalf("NormalizeHours = %#v", got)
	}
}

func TestNormalizeHoursKeepsValidValues(t *testing.T) {
	if got := NormalizeHours([]int{0, 23}); len(got) != 2 || got[0] != 0 || got[1] != 23 {
		t.Fatalf("NormalizeHours = %#v", got)
	}
}

func TestNormalizeMinutesBoundsAndOrder(t *testing.T) {
	got := NormalizeMinutes([]int{635, 0, 1439, 1439, 1440, -1})
	want := []int{0, 635, 1439}
	if len(got) != len(want) {
		t.Fatalf("NormalizeMinutes = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("NormalizeMinutes = %#v, want %#v", got, want)
		}
	}
}

func TestHoursToMinutes(t *testing.T) {
	got := HoursToMinutes([]int{9, 21, 9, 24})
	want := []int{540, 1260}
	if len(got) != len(want) {
		t.Fatalf("HoursToMinutes = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("HoursToMinutes = %#v, want %#v", got, want)
		}
	}
}

// TestNextWakeMinutesPicksNearestFuture 验证分钟级唤醒：同一天里最近的
// 未来分钟点被选中，且同时刻的多个任务族一起返回。
func TestNextWakeMinutesPicksNearestFuture(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 20, 0, 0, time.UTC)
	schedule := map[string][]int{
		"checkin":   {635}, // 10:35
		"keepalive": {540}, // 09:00（已过）
		"travel":    {635}, // 与 checkin 同刻
	}
	kinds, next := NextWakeMinutes(now, schedule)
	if next.Hour() != 10 || next.Minute() != 35 {
		t.Fatalf("next = %s，期望 10:35", next.Format("15:04"))
	}
	if len(kinds) != 2 {
		t.Fatalf("同刻任务族应一起返回，得到 %#v", kinds)
	}
	for _, kind := range kinds {
		if kind != "checkin" && kind != "travel" {
			t.Fatalf("返回了非本刻的任务族：%#v", kinds)
		}
	}
}

// TestNextWakeMinutesWrapsToTomorrow 验证当天全部过点时滚到次日首个分钟点。
func TestNextWakeMinutesWrapsToTomorrow(t *testing.T) {
	now := time.Date(2026, 11, 12, 23, 30, 0, 0, time.UTC)
	schedule := map[string][]int{"checkin": {15}} // 00:15
	kinds, next := NextWakeMinutes(now, schedule)
	if len(kinds) != 1 || kinds[0] != "checkin" {
		t.Fatalf("kinds = %#v", kinds)
	}
	want := time.Date(2026, 11, 13, 0, 15, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next = %s，期望 %s", next, want)
	}
}

// TestNextWakeMinutesEmptyFallsBackToHour 验证空排程不会算出过去时刻（否则
// 调度循环会立刻唤醒再算一次，变成忙等）。
func TestNextWakeMinutesEmptyFallsBackToHour(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	kinds, next := NextWakeMinutes(now, nil)
	if len(kinds) != 0 {
		t.Fatalf("空排程不该有任务族：%#v", kinds)
	}
	if !next.Equal(now.Add(time.Hour)) {
		t.Fatalf("next = %s，期望一小时后", next)
	}
}

// TestNextWakeLegacyWrapperMatchesMinutes 验证整点入参的旧入口与分钟实现等价：
// 存量调用点仍传小时，换算不能有偏差。
func TestNextWakeLegacyWrapperMatchesMinutes(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 20, 0, 0, time.UTC)
	hours := map[string][]int{"checkin": {11}}
	legacyKinds, legacyNext := NextWake(now, hours)
	minuteKinds, minuteNext := NextWakeMinutes(now, map[string][]int{"checkin": {660}})
	if !legacyNext.Equal(minuteNext) {
		t.Fatalf("整点入口 %s 与分钟入口 %s 不一致", legacyNext, minuteNext)
	}
	if len(legacyKinds) != len(minuteKinds) || legacyKinds[0] != minuteKinds[0] {
		t.Fatalf("任务族不一致：%#v vs %#v", legacyKinds, minuteKinds)
	}
}
