package admin

// 分钟级排程的设置链路：写入、归一化、优先级、回落。
//
// 优先级是这一项最容易出错的地方——分钟覆盖与整点列表同时存在时，如果
// 整点胜出，用户「设了 10:35」会被打回 10:00，而且页面看不出原因。

import (
	"encoding/json"
	"net/http"
	"testing"

	"freetier2api-plugin/cpasdk/pluginapi"
	"freetier2api-plugin/internal/core"
)

// taskVendor 在 fakeVendor 之上声明一个任务族。
//
// 排程设置的写入有 SpecByKind 闸门（防止手滑写入不存在的任务族），空注册表
// 下所有排程都会被拦掉——所以这组用例必须带一个声明了任务族的供应商。
type taskVendor struct {
	fakeVendor
}

func (v *taskVendor) TaskSpecs() []core.TaskSpec {
	return []core.TaskSpec{{Kind: "checkin", Label: "每日签到", DefaultHours: []int{10}}}
}

// newTaskService 构造一个认识 "checkin" 任务族的服务。
func newTaskService(t *testing.T) *Service {
	t.Helper()
	deps := newStubDeps(t)
	vendor := &taskVendor{fakeVendor{Meta: core.Meta{VendorID: "taskprobe", VendorName: "任务探针"}}}
	if errRegister := deps.registry.Register(vendor); errRegister != nil {
		t.Fatalf("register task vendor: %v", errRegister)
	}
	return newServiceWithDeps(t, deps)
}

func settingsRequest(t *testing.T, service *Service, body map[string]any) {
	t.Helper()
	raw, errMarshal := json.Marshal(body)
	if errMarshal != nil {
		t.Fatalf("marshal: %v", errMarshal)
	}
	response := service.handleSettingsRequest(pluginapi.ManagementRequest{
		Method: http.MethodPost, Body: raw,
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.StatusCode, response.Body)
	}
}

func scheduleOf(t *testing.T, service *Service, kind string) []int {
	t.Helper()
	settings := service.tasks.EffectiveSettings()
	raw, okRaw := settings["schedule"].(map[string][]int)
	if !okRaw {
		t.Fatalf("EffectiveSettings 缺少 schedule：%#v", settings)
	}
	return raw[kind]
}

func containsMinute(minutes []int, want int) bool {
	for _, minute := range minutes {
		if minute == want {
			return true
		}
	}
	return false
}

func TestSettingsStoreNormalizedMinutes(t *testing.T) {
	service := newTaskService(t)
	settingsRequest(t, service, map[string]any{
		"task_schedule_minutes": map[string][]int{"checkin": {635, 635, 1440, -1}},
	})
	snapshot := service.deps.State()
	minutes := snapshot.TaskScheduleMinutes["checkin"]
	if len(minutes) != 1 || minutes[0] != 635 {
		t.Fatalf("分钟排程未归一化入库：%#v", minutes)
	}
}

func TestMinuteScheduleWinsOverHours(t *testing.T) {
	service := newTaskService(t)
	settingsRequest(t, service, map[string]any{
		"task_schedules":        map[string][]int{"checkin": {9}},
		"task_schedule_minutes": map[string][]int{"checkin": {635}},
	})
	schedule := scheduleOf(t, service, "checkin")
	if !containsMinute(schedule, 635) {
		t.Fatalf("分钟覆盖没有生效：%#v", schedule)
	}
	if containsMinute(schedule, 540) {
		t.Fatalf("整点设置仍然残留，分钟优先级失效：%#v", schedule)
	}
}

func TestHoursResetClearsMinuteOverride(t *testing.T) {
	service := newTaskService(t)
	settingsRequest(t, service, map[string]any{
		"task_schedule_minutes": map[string][]int{"checkin": {635}},
	})
	// 用户改回整点：必须把分钟覆盖清掉，否则「改成整点」不生效。
	settingsRequest(t, service, map[string]any{
		"task_schedules": map[string][]int{"checkin": {10}},
	})
	if _, okMinute := service.deps.State().TaskScheduleMinutes["checkin"]; okMinute {
		t.Fatal("回到整点后分钟覆盖应被清除")
	}
	schedule := scheduleOf(t, service, "checkin")
	if !containsMinute(schedule, 600) {
		t.Fatalf("整点设置没有生效：%#v", schedule)
	}
}

func TestSettingsExposeReadableSchedule(t *testing.T) {
	service := newTaskService(t)
	settingsRequest(t, service, map[string]any{
		"task_schedule_minutes": map[string][]int{"checkin": {635}},
	})
	settings := service.tasks.EffectiveSettings()
	readable, okReadable := settings["schedule_hhmm"].(map[string][]string)
	if !okReadable {
		t.Fatalf("缺少可读排程 schedule_hhmm：%#v", settings)
	}
	labels := readable["checkin"]
	found := false
	for _, label := range labels {
		if label == "10:35" {
			found = true
		}
	}
	if !found {
		t.Fatalf("可读排程里没有 10:35：%#v", labels)
	}
}
