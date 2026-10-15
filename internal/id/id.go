// Package id 生成随机标识符。
//
// 只用于本地数据的主键，不参与安全决策，因此使用 crypto/rand 生成 UUID v4 字符串，
// 不引入第三方 uuid 依赖。
package id

import (
	"crypto/rand"
	"encoding/hex"
)

// New 返回一个 UUID v4 字符串。
func New() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand 失败属于系统级异常；退化为全零前缀也不会导致数据错乱
		// （主键冲突会被数据库拒绝），但正常情况不会发生。
		panic("无法生成随机标识符: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10

	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return string(out[:])
}
