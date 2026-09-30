# risktimeline

员工 Warning 告警的风险时间线库（Go）。状态保存在内存中，调用方保证串行访问；
告警只增不撤销。任何查询结果都等价于把已接收的唯一告警按事件时间完整重算，
与接收顺序无关。

## 规则

- 时刻 `t` 的计数范围：`(t-3600000, t]` 毫秒内的告警。
- 计数达到 3 为 Critical，否则为 Warning；该状态随时间变化，不是永久升级。
- 告警在自己的时间戳开始计数，恰好一小时后不再计数。

## API

```go
tl := risktimeline.New()

// 录入告警。key 全库唯一，重复 key（即使员工/时间不同）无副作用地忽略。
tl.Ingest("alert-key-1", "emp-001", 1700000000000)

// 查询 [fromMS, toMS) 内的 Critical 最大连续区间，左闭右开，
// 结果已裁剪到查询范围并合并端点相接的相邻区间。
ranges := tl.CriticalRanges("emp-001", fromMS, toMS)
for _, r := range ranges {
    // r.StartMS <= t < r.EndMS 期间该员工处于 Critical
}
```

## 测试

```sh
go test ./...
```
