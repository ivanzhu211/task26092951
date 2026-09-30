// Package risktimeline 提供员工告警的风险时间线计算。
package risktimeline

import "sort"

// windowMS 是告警计数的滑动窗口长度：一小时（毫秒）。
const windowMS int64 = 3600000

// criticalThreshold 是进入 Critical 状态所需的窗口内告警数量。
const criticalThreshold = 3

// Range 表示一个左闭右开的时间区间 [StartMS, EndMS)，单位 UTC 毫秒。
type Range struct {
	StartMS int64
	EndMS   int64
}

// Timeline 保存全部已接收告警的内存状态。
// 调用方保证串行访问，因此内部不加锁。
type Timeline struct {
	seen  map[string]struct{}
	byEmp map[string][]int64
}

// New 创建一个空的时间线库。
func New() *Timeline {
	return &Timeline{
		seen:  make(map[string]struct{}),
		byEmp: make(map[string][]int64),
	}
}

// Ingest 录入一条告警。key 全库唯一，重复 key 无副作用地忽略。
func (t *Timeline) Ingest(key, employeeID string, timeMS int64) {
	if _, ok := t.seen[key]; ok {
		return
	}
	t.seen[key] = struct{}{}
	t.byEmp[employeeID] = append(t.byEmp[employeeID], timeMS)
}

// CriticalRanges 返回员工在 [fromMS, toMS) 内处于 Critical 状态的全部
// 最大连续区间，结果裁剪到查询范围并合并相邻相接区间。
func (t *Timeline) CriticalRanges(employeeID string, fromMS, toMS int64) []Range {
	if fromMS >= toMS {
		return nil
	}
	times := t.byEmp[employeeID]
	if len(times) < criticalThreshold {
		return nil
	}

	// 扫描线事件：告警在其时间戳开始计数（+1），恰好一小时后到期（-1）。
	events := make([]sweepEvent, 0, 2*len(times))
	for _, ts := range times {
		events = append(events, sweepEvent{at: ts, delta: 1})
		events = append(events, sweepEvent{at: ts + windowMS, delta: -1})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].at < events[j].at })

	var ranges []Range
	count := 0
	segStart := int64(0)
	for i := 0; i < len(events); {
		at := events[i].at
		// 同一毫秒内的所有到达和到期共同决定该时刻之后的状态。
		for i < len(events) && events[i].at == at {
			count += events[i].delta
			i++
		}
		next := toMS
		if i < len(events) && events[i].at < next {
			next = events[i].at
		}
		if count < criticalThreshold {
			continue
		}
		// 当前段为 [at, next)，裁剪到查询范围。
		start := at
		if start < fromMS {
			start = fromMS
		}
		end := next
		if end > toMS {
			end = toMS
		}
		if start >= end {
			continue
		}
		segStart = start
		if n := len(ranges); n > 0 && ranges[n-1].EndMS >= segStart {
			// 端点相接或重叠，合并。
			if ranges[n-1].EndMS < end {
				ranges[n-1].EndMS = end
			}
			continue
		}
		ranges = append(ranges, Range{StartMS: start, EndMS: end})
	}
	return ranges
}

type sweepEvent struct {
	at    int64
	delta int
}
