package alerttimeline

import (
	"reflect"
	"testing"
	"time"
)

const minuteMS int64 = 60000

// base 是一个固定的 UTC 起点。
var base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

func at(minutes int64) int64 { return base + minutes*minuteMS }

func rng(fromMin, toMin int64) Range { return Range{FromMS: at(fromMin), ToMS: at(toMin)} }

func assertRanges(t *testing.T, got, want []Range) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// 场景一：分钟偏移 0、10、20、65、80、85，随后补入第 40 分钟的告警。
func TestMinuteOffsetsAndLateArrival(t *testing.T) {
	tl := New()
	minutes := []int64{0, 10, 20, 65, 80, 85}
	keys := []string{"a0", "a10", "a20", "a65", "a80", "a85"}
	for i, m := range minutes {
		tl.Ingest(keys[i], "emp1", at(m))
	}

	// 完整查询。
	assertRanges(t, tl.CriticalRanges("emp1", at(0), at(200)),
		[]Range{rng(20, 60), rng(65, 70), rng(85, 125)})

	// 查询范围截断。
	assertRanges(t, tl.CriticalRanges("emp1", at(30), at(90)),
		[]Range{rng(30, 60), rng(65, 70), rng(85, 90)})
	assertRanges(t, tl.CriticalRanges("emp1", at(60), at(65)), nil)
	assertRanges(t, tl.CriticalRanges("emp1", at(21), at(59)),
		[]Range{rng(21, 59)})

	// 补入第 40 分钟的告警后，过去的查询结果改变并合并为一个区间。
	tl.Ingest("a40", "emp1", at(40))
	assertRanges(t, tl.CriticalRanges("emp1", at(0), at(200)),
		[]Range{rng(20, 125)})
	assertRanges(t, tl.CriticalRanges("emp1", at(50), at(100)),
		[]Range{rng(50, 100)})
}

// 场景二：同一毫秒到达三条告警、恰好一小时到期。
func TestSameMillisecondArrivalAndExpiry(t *testing.T) {
	tl := New()
	tl.Ingest("s1", "empA", base)
	tl.Ingest("s2", "empA", base)
	tl.Ingest("s3", "empA", base)

	// 同一毫秒的到达共同决定状态：从 base 起 Critical，恰好一小时后结束。
	assertRanges(t, tl.CriticalRanges("empA", base-1000, base+3600000+1000),
		[]Range{{FromMS: base, ToMS: base + 3600000}})

	// 到期时刻本身不再计数。
	assertRanges(t, tl.CriticalRanges("empA", base+3600000, base+3600000+1), nil)
	// 到期前一毫秒仍处于 Critical。
	assertRanges(t, tl.CriticalRanges("empA", base+3600000-1, base+3600000),
		[]Range{{FromMS: base + 3600000 - 1, ToMS: base + 3600000}})
	// 到达前一毫秒不是 Critical。
	assertRanges(t, tl.CriticalRanges("empA", base-1, base), nil)
}

// 场景三：跨员工、跨时间重放同一 key，不得改变任何员工的时间线。
func TestReplaySameKeyIsIgnored(t *testing.T) {
	tl := New()
	tl.Ingest("r1", "empA", at(0))
	tl.Ingest("r2", "empA", at(1))
	tl.Ingest("r3", "empA", at(2))
	// 员工 B 的告警按乱序录入，结果应与接收顺序无关。
	tl.Ingest("r4", "empB", at(10))
	tl.Ingest("r5", "empB", at(5))
	tl.Ingest("r6", "empB", at(0))

	wantA := []Range{rng(2, 60)}
	wantB := []Range{rng(10, 60)}
	assertRanges(t, tl.CriticalRanges("empA", at(0), at(200)), wantA)
	assertRanges(t, tl.CriticalRanges("empB", at(0), at(200)), wantB)

	// 跨员工重放：相同 key 换员工、换时间，必须无副作用。
	tl.Ingest("r1", "empB", at(50))
	tl.Ingest("r4", "empA", at(50))
	// 跨时间重放：相同 key 同员工、不同时间。
	tl.Ingest("r2", "empA", at(99))
	// 完全相同的重放。
	tl.Ingest("r3", "empA", at(2))

	assertRanges(t, tl.CriticalRanges("empA", at(0), at(200)), wantA)
	assertRanges(t, tl.CriticalRanges("empB", at(0), at(200)), wantB)
	// 不存在的员工没有任何区间。
	assertRanges(t, tl.CriticalRanges("empC", at(0), at(200)), nil)
}

// 边界：空查询范围与不足三条告警。
func TestEdgeCases(t *testing.T) {
	tl := New()
	tl.Ingest("e1", "emp", at(0))
	tl.Ingest("e2", "emp", at(1))
	assertRanges(t, tl.CriticalRanges("emp", at(0), at(200)), nil)
	assertRanges(t, tl.CriticalRanges("emp", at(10), at(10)), nil)
	assertRanges(t, tl.CriticalRanges("emp", at(10), at(5)), nil)
}
