// Package tasks 提供供应商无关的任务目录、排程与调度基础设施。
package tasks

import (
	"sort"
	"strings"
	"time"

	"freetier2api-plugin/internal/core"
)

// Catalog 汇总已注册供应商的任务族。
type Catalog struct{ registry *core.Registry }

// NewCatalog 构造任务目录。
func NewCatalog(registry *core.Registry) *Catalog { return &Catalog{registry: registry} }

// Specs 汇总全部启用任务族的规格（按 kind 去重，先注册者优先）。
func (c *Catalog) Specs() []core.TaskSpec {
	out := make([]core.TaskSpec, 0, 8)
	seen := map[string]bool{}
	if c.anyCheckin() {
		seen[core.TaskKindCheckin] = true
		out = append(out, core.TaskSpec{
			Kind:         core.TaskKindCheckin,
			Label:        "每日签到",
			Description:  "对所有支持签到的账号执行签到",
			DefaultHours: []int{9, 21},
			Manual:       true,
		})
	}
	for _, vendor := range c.registry.Vendors() {
		catalog, okCatalog := core.As[core.TaskCatalogProvider](vendor)
		if !okCatalog {
			continue
		}
		for _, spec := range catalog.TaskSpecs() {
			kind := strings.TrimSpace(spec.Kind)
			if kind == "" || seen[kind] {
				continue
			}
			spec.Kind = kind
			spec.DefaultHours = NormalizeHours(spec.DefaultHours)
			seen[kind] = true
			out = append(out, spec)
		}
	}
	return out
}

// SpecByKind 返回某个任务族的规格。
func (c *Catalog) SpecByKind(kind string) (core.TaskSpec, bool) {
	kind = strings.TrimSpace(kind)
	for _, spec := range c.Specs() {
		if spec.Kind == kind {
			return spec, true
		}
	}
	return core.TaskSpec{}, false
}

// ManualSpecs 返回需要管理端手动入口的任务族。
func (c *Catalog) ManualSpecs() []core.TaskSpec {
	all := c.Specs()
	out := make([]core.TaskSpec, 0, len(all))
	for _, spec := range all {
		if spec.Manual {
			out = append(out, spec)
		}
	}
	return out
}

// DefaultSchedule 从任务规格生成默认整点排程。
func (c *Catalog) DefaultSchedule() map[string][]int {
	out := map[string][]int{}
	for _, spec := range c.Specs() {
		if len(spec.DefaultHours) > 0 {
			out[spec.Kind] = append([]int(nil), spec.DefaultHours...)
		}
	}
	return out
}

// EffectiveSchedule 合并默认值、状态文件中的用户覆盖值与任务规格。
func (c *Catalog) EffectiveSchedule(overrides map[string][]int) map[string][]int {
	out := c.DefaultSchedule()
	for kind, hours := range overrides {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		if _, known := c.SpecByKind(kind); !known {
			continue
		}
		if len(hours) > 0 {
			out[kind] = NormalizeHours(hours)
		}
	}
	return out
}

// NormalizeHours 去重、排序并过滤非法小时。
func NormalizeHours(hours []int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(hours))
	for _, hour := range hours {
		if hour < 0 || hour > 23 || seen[hour] {
			continue
		}
		seen[hour] = true
		out = append(out, hour)
	}
	sort.Ints(out)
	return out
}

// HoursToMinutes 把整点列表换算成当天的分钟数（0-1439）。
func HoursToMinutes(hours []int) []int {
	out := make([]int, 0, len(hours))
	for _, hour := range NormalizeHours(hours) {
		out = append(out, hour*60)
	}
	return out
}

// NormalizeMinutes 去重、排序并过滤非法分钟（0-1439）。
func NormalizeMinutes(minutes []int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(minutes))
	for _, minute := range minutes {
		if minute < 0 || minute > 1439 || seen[minute] {
			continue
		}
		seen[minute] = true
		out = append(out, minute)
	}
	sort.Ints(out)
	return out
}

// DefaultScheduleMinutes 返回默认排程（分钟表示）。
func (c *Catalog) DefaultScheduleMinutes() map[string][]int {
	out := map[string][]int{}
	for kind, hours := range c.DefaultSchedule() {
		out[kind] = HoursToMinutes(hours)
	}
	return out
}

// EffectiveScheduleMinutes 合并默认值、整点覆盖与分钟覆盖。
//
// 两级覆盖而不是直接改字段：task_schedules（整点）是存量配置，任务排程
// 写入后不会自动迁移；分钟级是新能力，只在用户显式设置时生效。两者同时
// 存在时分钟优先——否则「设了 10:35」会被同族的整点覆盖打回 10:00。
func (c *Catalog) EffectiveScheduleMinutes(hoursOverrides, minuteOverrides map[string][]int) map[string][]int {
	out := c.DefaultScheduleMinutes()
	for kind, hours := range hoursOverrides {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		if _, known := c.SpecByKind(kind); !known {
			continue
		}
		if len(hours) > 0 {
			out[kind] = NormalizeMinutes(HoursToMinutes(hours))
		}
	}
	for kind, minutes := range minuteOverrides {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		if _, known := c.SpecByKind(kind); !known {
			continue
		}
		if len(minutes) > 0 {
			out[kind] = NormalizeMinutes(minutes)
		}
	}
	return out
}

// NextWakeMinutes 返回下一次唤醒时刻与要执行的任务族（分钟粒度）。
//
// 分钟粒度是排程的最小单位：签到窗口常常 10:30 才开，整点排程只能二选一
// ——要么 10:00 跑空，要么 11:00 晚半小时。
func NextWakeMinutes(now time.Time, schedule map[string][]int) ([]string, time.Time) {
	byMinute := map[int][]string{}
	minutes := make([]int, 0, len(schedule))
	for kind, list := range schedule {
		for _, minute := range NormalizeMinutes(list) {
			if _, okMinute := byMinute[minute]; !okMinute {
				minutes = append(minutes, minute)
			}
			byMinute[minute] = append(byMinute[minute], kind)
		}
	}
	if len(minutes) == 0 {
		return nil, now.Add(time.Hour)
	}
	sort.Ints(minutes)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for _, minute := range minutes {
		candidate := midnight.Add(time.Duration(minute) * time.Minute)
		if candidate.After(now) {
			sort.Strings(byMinute[minute])
			return byMinute[minute], candidate
		}
	}
	first := minutes[0]
	sort.Strings(byMinute[first])
	return byMinute[first], midnight.AddDate(0, 0, 1).Add(time.Duration(first) * time.Minute)
}

// NextWake 返回下一次唤醒时刻与要执行的任务族（整点入参，分钟实现）。
//
// 保留整点签名是为了不让存量调用点与测试跟着改：换算在入口处一次完成。
func NextWake(now time.Time, schedule map[string][]int) ([]string, time.Time) {
	minutes := make(map[string][]int, len(schedule))
	for kind, hours := range schedule {
		minutes[kind] = HoursToMinutes(hours)
	}
	return NextWakeMinutes(now, minutes)
}

func (c *Catalog) anyCheckin() bool {
	for _, vendor := range c.registry.Vendors() {
		if core.SupportsCheckin(vendor) {
			return true
		}
	}
	return false
}
