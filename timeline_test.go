package risktimeline

import (
	"reflect"
	"testing"
)

const minuteMS int64 = 60000

// 以固定 UTC 起点为基准的分钟偏移。
func atMin(base, min int64) int64 { return base + min*minuteMS }

func TestCriticalRangesScenario(t *testing.T) {
	base := int64(1700000000000) // 固定 UTC 起点
	tl := New()
	emp := "emp-1"
	for i, min := range []int64{0, 10, 20, 65, 80, 85} {
		tl.Ingest(keyOf(i), emp, atMin(base, min))
	}

	full := tl.CriticalRanges(emp, atMin(base, 0), atMin(base, 200))
	want := []Range{
		{atMin(base, 20), atMin(base, 60)},
		{atMin(base, 65), atMin(base, 70)},
		{atMin(base, 85), atMin(base, 125)},
	}
	if !reflect.DeepEqual(full, want) {
		t.Fatalf("full query = %v, want %v", full, want)
	}

	// 查询范围截断：结果必须裁剪到 [from, to)。
	clipped := tl.CriticalRanges(emp, atMin(base, 30), atMin(base, 90))
	wantClipped := []Range{
		{atMin(base, 30), atMin(base, 60)},
		{atMin(base, 65), atMin(base, 70)},
		{atMin(base, 85), atMin(base, 90)},
	}
	if !reflect.DeepEqual(clipped, wantClipped) {
		t.Fatalf("clipped query = %v, want %v", clipped, wantClipped)
	}

	// 补入第 40 分钟的告警后，各段应合并为一个连续区间。
	tl.Ingest("k-late-40", emp, atMin(base, 40))
	merged := tl.CriticalRanges(emp, atMin(base, 0), atMin(base, 200))
	wantMerged := []Range{{atMin(base, 20), atMin(base, 125)}}
	if !reflect.DeepEqual(merged, wantMerged) {
		t.Fatalf("merged query = %v, want %v", merged, wantMerged)
	}
}

func keyOf(i int) string {
	return "k-" + string(rune('a'+i))
}

func TestSameMillisecondArrivalAndExpiry(t *testing.T) {
	base := int64(1700000000000)
	tl := New()
	emp := "emp-same-ms"

	// 三个不同 key 在同一毫秒到达。
	tl.Ingest("same-1", emp, base)
	tl.Ingest("same-2", emp, base)
	tl.Ingest("same-3", emp, base)

	got := tl.CriticalRanges(emp, base-1000, base+2*windowMS)
	want := []Range{{base, base + windowMS}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("same-ms = %v, want %v", got, want)
	}

	// 恰好一小时后到期：边界时刻不产生零长度区间。
	exact := tl.CriticalRanges(emp, base+windowMS, base+windowMS+1)
	if len(exact) != 0 {
		t.Fatalf("at exact expiry = %v, want empty", exact)
	}

	// 同一毫秒内既有到期又有到达，共同决定状态，不产生零长度区间。
	tl2 := New()
	tl2.Ingest("e-1", emp, base)
	tl2.Ingest("e-2", emp, base)
	tl2.Ingest("e-3", emp, base+windowMS) // 前两条到期与第三条到达同刻
	if got := tl2.CriticalRanges(emp, base-1, base+2*windowMS); len(got) != 0 {
		t.Fatalf("arrival+expiry same ms = %v, want empty", got)
	}
}

func TestReplaySameKeyIsIgnored(t *testing.T) {
	base := int64(1700000000000)
	tl := New()
	empA, empB := "emp-a", "emp-b"

	tl.Ingest("r-1", empA, base)
	tl.Ingest("r-2", empA, base+minuteMS)
	tl.Ingest("r-3", empA, base+2*minuteMS)
	before := tl.CriticalRanges(empA, base, base+windowMS)

	// 跨员工、跨时间重放同一 key：必须无副作用。
	tl.Ingest("r-1", empB, base+5*minuteMS)
	tl.Ingest("r-2", empB, base+6*minuteMS)
	tl.Ingest("r-3", empA, base+7*minuteMS)

	after := tl.CriticalRanges(empA, base, base+windowMS)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("empA changed after replay: before %v, after %v", before, after)
	}
	if got := tl.CriticalRanges(empB, base, base+2*windowMS); len(got) != 0 {
		t.Fatalf("empB affected by replayed keys: %v", got)
	}
}

func TestOrderIndependence(t *testing.T) {
	base := int64(1700000000000)
	mins := []int64{85, 0, 80, 20, 65, 10} // 乱序录入
	tl := New()
	for i, min := range mins {
		tl.Ingest(keyOf(i), "emp-x", atMin(base, min))
	}
	got := tl.CriticalRanges("emp-x", atMin(base, 0), atMin(base, 200))
	want := []Range{
		{atMin(base, 20), atMin(base, 60)},
		{atMin(base, 65), atMin(base, 70)},
		{atMin(base, 85), atMin(base, 125)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("out-of-order ingest = %v, want %v", got, want)
	}
}
