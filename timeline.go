// Package alerttimeline 维护员工 Warning 告警的内存时间线，
// 并派生随时间变化的 Critical 状态区间。
package alerttimeline

import "sort"

// windowMS 是计数窗口长度：1 小时（毫秒）。
const windowMS int64 = 3600000

// criticalThreshold 是进入 Critical 状态所需的窗口内告警数量。
const criticalThreshold = 3

// Range 表示一个左闭右开的时间区间 [FromMS, ToMS)，单位为 UTC 毫秒。
type Range struct {
	FromMS int64
	ToMS   int64
}

// Timeline 保存所有已接收的唯一告警，调用方保证串行访问。
type Timeline struct {
	seen   map[string]struct{}
	alerts map[string][]int64
}

// New 创建一个空的 Timeline。
func New() *Timeline {
	return &Timeline{
		seen:   make(map[string]struct{}),
		alerts: make(map[string][]int64),
	}
}

// Ingest 录入一条告警。key 全库唯一：第一次写入生效，后续相同 key
// 即使员工或时间不同也无副作用地忽略。
func (t *Timeline) Ingest(key, employeeID string, timeMS int64) {
	if _, ok := t.seen[key]; ok {
		return
	}
	t.seen[key] = struct{}{}
	t.alerts[employeeID] = append(t.alerts[employeeID], timeMS)
}

// CriticalRanges 返回员工在 [fromMS, toMS) 内处于 Critical 状态的全部
// 最大连续区间，结果裁剪到查询范围并合并端点相接的相邻区间。
//
// 每次调用都基于当前已接收的全部告警按事件时间完整重算，与接收顺序无关。
func (t *Timeline) CriticalRanges(employeeID string, fromMS, toMS int64) []Range {
	if fromMS >= toMS {
		return nil
	}
	// 每条告警在时刻 s 开始计数，在 s+windowMS 不再计数，
	// 即对计数贡献区间 [s, s+windowMS)。聚合同一毫秒的所有到达与到期，
	// 使它们共同决定该时刻之后的状态，不产生零长度区间。
	deltas := make(map[int64]int64)
	for _, s := range t.alerts[employeeID] {
		deltas[s]++
		deltas[s+windowMS]--
	}
	times := make([]int64, 0, len(deltas))
	for ts := range deltas {
		times = append(times, ts)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })

	var ranges []Range
	var count int64
	open := false
	var start int64
	for _, ts := range times {
		count += deltas[ts]
		if open && count < criticalThreshold {
			// 状态在 ts 处离开 Critical，关闭区间 [start, ts)。
			open = false
			if r, ok := clipRange(start, ts, fromMS, toMS); ok {
				ranges = append(ranges, r)
			}
		} else if !open && count >= criticalThreshold {
			// 状态在 ts 处进入 Critical，从 ts 起开启区间。
			open = true
			start = ts
		}
	}
	if open {
		// 最后一段 Critical 延续到无穷远，裁剪到查询范围。
		if r, ok := clipRange(start, toMS, fromMS, toMS); ok {
			ranges = append(ranges, r)
		}
	}
	return ranges
}

// clipRange 把左闭右开区间 [s, e) 裁剪到 [from, to)，
// 裁剪结果为空时返回 false。
func clipRange(s, e, from, to int64) (Range, bool) {
	if s < from {
		s = from
	}
	if e > to {
		e = to
	}
	if s >= e {
		return Range{}, false
	}
	return Range{FromMS: s, ToMS: e}, true
}
