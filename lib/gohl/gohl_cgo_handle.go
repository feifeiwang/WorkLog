package gohl

// 本文件是对 Go 标准库 runtime/cgo（cgo.Handle）的纯 Go 替代实现。
//
// 背景：Gohl 原版使用 runtime/cgo 的 Handle 机制把 Go 对象指针传递给
// Windows syscall 回调，但该机制在 CGO_ENABLED=0（纯 Go 交叉编译）时
// 无法链接。本 shim 用一张并发安全的全局表实现同样的功能，使得整个
// 库可以纯 Go 交叉编译，产出的 exe 不依赖任何 C 运行时。
//
// 语义与 runtime/cgo 保持一致：
//   - NewHandle 返回从 1 开始单调递增的句柄，绝不返回 0（0 表示无效句柄）
//   - 句柄对应的对象在 Delete 之前一直有效
//   - 对无效/已删除的句柄调用 Value 会 panic

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type cgoHandle uintptr

var (
	cgoHandleMu    sync.RWMutex
	cgoHandleTable = make(map[cgoHandle]interface{})
	cgoHandleSeq   uintptr
)

func cgoNewHandle(v interface{}) cgoHandle {
	h := cgoHandle(atomic.AddUintptr(&cgoHandleSeq, 1))
	cgoHandleMu.Lock()
	cgoHandleTable[h] = v
	cgoHandleMu.Unlock()
	return h
}

func (h cgoHandle) Value() interface{} {
	cgoHandleMu.RLock()
	v, ok := cgoHandleTable[h]
	cgoHandleMu.RUnlock()
	if !ok {
		panic(fmt.Sprintf("gohl: cgoHandle misuse: handle %d not found", uintptr(h)))
	}
	return v
}

func (h cgoHandle) Delete() {
	cgoHandleMu.Lock()
	delete(cgoHandleTable, h)
	cgoHandleMu.Unlock()
}
