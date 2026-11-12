package tasks

import (
	"reflect"
	"testing"
)

func TestMergeMinuteUsesNormalizeRules(t *testing.T) {
	tests := []struct {
		name    string
		minutes []int
		minute  int
		want    []int
	}{
		{name: "空列表", minutes: nil, minute: 540, want: []int{540}},
		{name: "去重", minutes: []int{540}, minute: 540, want: []int{540}},
		{name: "排序", minutes: []int{1260, 540}, minute: 635, want: []int{540, 635, 1260}},
		// 自己写一份去重排序时不会校验边界，1440 会静默留在排程里，永远不触发。
		{name: "越界被丢弃", minutes: []int{540}, minute: 1440, want: []int{540}},
		{name: "负值被丢弃", minutes: []int{540}, minute: -1, want: []int{540}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := mergeMinute(test.minutes, test.minute)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("mergeMinute(%v, %d) = %v, want %v", test.minutes, test.minute, got, test.want)
			}
		})
	}
}

// mergeMinute 不能改动入参切片背后的数组：effectiveScheduleMinutes 会把状态里的
// 切片直接传进来，原地排序会污染后续调用。
func TestMergeMinuteDoesNotMutateInput(t *testing.T) {
	minutes := make([]int, 2, 4)
	minutes[0], minutes[1] = 1260, 540
	backing := minutes[:cap(minutes)]
	_ = mergeMinute(minutes, 635)
	if backing[0] != 1260 || backing[1] != 540 {
		t.Fatalf("入参被就地修改: %v", backing[:2])
	}
}

// parseClockMinute 不再向上取整：10:35 就是 635 分钟。
func TestParseClockMinuteIsExact(t *testing.T) {
	tests := []struct {
		value string
		want  int
		ok    bool
	}{
		{value: "00:00", want: 0, ok: true},
		{value: "10:35", want: 635, ok: true},
		{value: "23:59", want: 1439, ok: true},
		{value: "9:05", want: 545, ok: true},
		{value: "24:00", ok: false},
		{value: "10:60", ok: false},
		{value: "", ok: false},
	}
	for _, test := range tests {
		got, ok := parseClockMinute(test.value)
		if ok != test.ok || (ok && got != test.want) {
			t.Fatalf("parseClockMinute(%q) = (%d,%v), want (%d,%v)", test.value, got, ok, test.want, test.ok)
		}
	}
}

func TestFormatMinuteRoundTrip(t *testing.T) {
	for _, test := range []struct {
		minute int
		want   string
	}{
		{0, "00:00"}, {635, "10:35"}, {1439, "23:59"}, {1440, "00:00"}, {-5, "00:00"},
	} {
		if got := formatMinute(test.minute); got != test.want {
			t.Fatalf("formatMinute(%d) = %q, want %q", test.minute, got, test.want)
		}
	}
}
