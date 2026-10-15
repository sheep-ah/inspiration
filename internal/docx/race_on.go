//go:build race

package docx

// raceEnabled 表示当前构建启用了竞态检测（-race）。
// 竞态插桩会让耗时成倍增长，因此性能用例在 race 模式下只记录数据、不做阈值断言。
const raceEnabled = true
