//go:build !linux || !cgo

package main

// 非 linux/cgo 环境下没有 nsenter 构造函数，exec-container 不可用
