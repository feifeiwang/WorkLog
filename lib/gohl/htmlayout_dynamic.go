package gohl

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	htmlayoutLib *syscall.DLL
)

func init() {
	extractResources()
	initHtmlayoutLib()
}

func initHtmlayoutLib() {
	if resourcesDir == "" {
		fmt.Println("资源目录未初始化")
		return
	}

	dllPath := filepath.Join(resourcesDir, "htmlayout.dll")

	if _, err := os.Stat(dllPath); os.IsNotExist(err) {
		fmt.Printf("htmlayout.dll 不存在: %s\n", dllPath)
		return
	}

	var err error
	htmlayoutLib, err = syscall.LoadDLL(dllPath)
	if err != nil {
		fmt.Printf("加载 htmlayout.dll 失败: %v\n", err)
		return
	}

	fmt.Printf("成功加载 htmlayout.dll: %s\n", dllPath)

	initHtmlayoutFunctions()
}

// 全局函数地址
var (
	procHTMLayoutProcND                   *syscall.Proc
	procHTMLayoutLoadHtmlEx               *syscall.Proc
	procHTMLayoutLoadFile                 *syscall.Proc
	procHTMLayoutSetOption                *syscall.Proc
	procHTMLayoutDataReady                *syscall.Proc
	procHTMLayoutWindowAttachEventHandler *syscall.Proc
	procHTMLayoutWindowDetachEventHandler *syscall.Proc
	procHTMLayoutSetCallback              *syscall.Proc
	procHTMLayoutGetRootElement           *syscall.Proc
	procHTMLayoutCreateElement            *syscall.Proc
	procHTMLayoutFindElement              *syscall.Proc
	procHTMLayoutGetFocusElement          *syscall.Proc
	procHTMLayoutAttachEventHandler       *syscall.Proc
	procHTMLayoutAttachEventHandlerEx     *syscall.Proc
	procHTMLayoutDetachEventHandler       *syscall.Proc
	procHTMLayoutUpdateElementEx          *syscall.Proc
	procHTMLayoutSetCapture               *syscall.Proc
	procHTMLayoutShowPopup                *syscall.Proc
	procHTMLayoutShowPopupAt              *syscall.Proc
	procHTMLayoutHidePopup                *syscall.Proc
	procHTMLayoutSelectElements           *syscall.Proc
	procHTMLayoutSelectParent             *syscall.Proc
	procHTMLayoutSendEvent                *syscall.Proc
	procHTMLayoutPostEvent                *syscall.Proc
	procHTMLayoutGetChildrenCount         *syscall.Proc
	procHTMLayoutGetNthChild              *syscall.Proc
	procHTMLayoutGetElementIndex          *syscall.Proc
	procHTMLayoutGetParentElement         *syscall.Proc
	procHTMLayoutInsertElement            *syscall.Proc
	procHTMLayoutDetachElement            *syscall.Proc
	procHTMLayoutDeleteElement            *syscall.Proc
	procHTMLayoutCloneElement             *syscall.Proc
	procHTMLayoutSwapElements             *syscall.Proc
	procHTMLayoutSetEventRoot             *syscall.Proc
	procHTMLayoutScrollToView             *syscall.Proc
	procHTMLayoutGetElementUID            *syscall.Proc
	procHTMLayoutGetElementByUID          *syscall.Proc
	procHTMLayoutCallBehaviorMethod       *syscall.Proc
	procHTMLayoutCombineURL               *syscall.Proc
	procHTMLayoutSortElements             *syscall.Proc
	procHTMLayoutSetTimerEx               *syscall.Proc
	procHTMLayoutGetElementHwnd           *syscall.Proc
	procHTMLayoutGetElementHtml           *syscall.Proc
	procHTMLayoutGetElementType           *syscall.Proc
	procHTMLayoutSetElementHtml           *syscall.Proc
	procHTMLayoutSetElementInnerText      *syscall.Proc
	procHTMLayoutGetElementInnerText      *syscall.Proc
	procHTMLayoutGetAttributeByName       *syscall.Proc
	procHTMLayoutSetAttributeByName       *syscall.Proc
	procHTMLayoutGetNthAttribute          *syscall.Proc
	procHTMLayoutGetAttributeCount        *syscall.Proc
	procHTMLayoutGetStyleAttribute        *syscall.Proc
	procHTMLayoutSetStyleAttribute        *syscall.Proc
	procHTMLayoutGetElementState          *syscall.Proc
	procHTMLayoutSetElementState          *syscall.Proc
	procHTMLayoutMoveElement              *syscall.Proc
	procHTMLayoutMoveElementEx            *syscall.Proc
	procHTMLayoutGetElementLocation       *syscall.Proc
	procHTMLayout_UseElement              *syscall.Proc
	procHTMLayout_UnuseElement            *syscall.Proc
)

func initHtmlayoutFunctions() {
	if htmlayoutLib == nil {
		return
	}

	procHTMLayoutProcND = mustFindProc("HTMLayoutProcND")
	procHTMLayoutLoadHtmlEx = mustFindProc("HTMLayoutLoadHtmlEx")
	procHTMLayoutLoadFile = mustFindProc("HTMLayoutLoadFile")
	procHTMLayoutSetOption = mustFindProc("HTMLayoutSetOption")
	procHTMLayoutDataReady = mustFindProc("HTMLayoutDataReady")
	procHTMLayoutWindowAttachEventHandler = mustFindProc("HTMLayoutWindowAttachEventHandler")
	procHTMLayoutWindowDetachEventHandler = mustFindProc("HTMLayoutWindowDetachEventHandler")
	procHTMLayoutSetCallback = mustFindProc("HTMLayoutSetCallback")
	procHTMLayoutGetRootElement = mustFindProc("HTMLayoutGetRootElement")
	procHTMLayoutCreateElement = mustFindProc("HTMLayoutCreateElement")
	procHTMLayoutFindElement = mustFindProc("HTMLayoutFindElement")
	procHTMLayoutGetFocusElement = mustFindProc("HTMLayoutGetFocusElement")
	procHTMLayoutAttachEventHandler = mustFindProc("HTMLayoutAttachEventHandler")
	procHTMLayoutAttachEventHandlerEx = mustFindProc("HTMLayoutAttachEventHandlerEx")
	procHTMLayoutDetachEventHandler = mustFindProc("HTMLayoutDetachEventHandler")
	procHTMLayoutUpdateElementEx = mustFindProc("HTMLayoutUpdateElementEx")
	procHTMLayoutSetCapture = mustFindProc("HTMLayoutSetCapture")
	procHTMLayoutShowPopup = mustFindProc("HTMLayoutShowPopup")
	procHTMLayoutShowPopupAt = mustFindProc("HTMLayoutShowPopupAt")
	procHTMLayoutHidePopup = mustFindProc("HTMLayoutHidePopup")
	procHTMLayoutSelectElements = mustFindProc("HTMLayoutSelectElements")
	procHTMLayoutSelectParent = mustFindProc("HTMLayoutSelectParent")
	procHTMLayoutSendEvent = mustFindProc("HTMLayoutSendEvent")
	procHTMLayoutPostEvent = mustFindProc("HTMLayoutPostEvent")
	procHTMLayoutGetChildrenCount = mustFindProc("HTMLayoutGetChildrenCount")
	procHTMLayoutGetNthChild = mustFindProc("HTMLayoutGetNthChild")
	procHTMLayoutGetElementIndex = mustFindProc("HTMLayoutGetElementIndex")
	procHTMLayoutGetParentElement = mustFindProc("HTMLayoutGetParentElement")
	procHTMLayoutInsertElement = mustFindProc("HTMLayoutInsertElement")
	procHTMLayoutDetachElement = mustFindProc("HTMLayoutDetachElement")
	procHTMLayoutDeleteElement = mustFindProc("HTMLayoutDeleteElement")
	procHTMLayoutCloneElement = mustFindProc("HTMLayoutCloneElement")
	procHTMLayoutSwapElements = mustFindProc("HTMLayoutSwapElements")
	procHTMLayoutSetEventRoot = mustFindProc("HTMLayoutSetEventRoot")
	procHTMLayoutScrollToView = mustFindProc("HTMLayoutScrollToView")
	procHTMLayoutGetElementUID = mustFindProc("HTMLayoutGetElementUID")
	procHTMLayoutGetElementByUID = mustFindProc("HTMLayoutGetElementByUID")
	procHTMLayoutCallBehaviorMethod = mustFindProc("HTMLayoutCallBehaviorMethod")
	procHTMLayoutCombineURL = mustFindProc("HTMLayoutCombineURL")
	procHTMLayoutSortElements = mustFindProc("HTMLayoutSortElements")
	procHTMLayoutSetTimerEx = mustFindProc("HTMLayoutSetTimerEx")
	procHTMLayoutGetElementHwnd = mustFindProc("HTMLayoutGetElementHwnd")
	procHTMLayoutGetElementHtml = mustFindProc("HTMLayoutGetElementHtml")
	procHTMLayoutGetElementType = mustFindProc("HTMLayoutGetElementType")
	procHTMLayoutSetElementHtml = mustFindProc("HTMLayoutSetElementHtml")
	procHTMLayoutSetElementInnerText = mustFindProc("HTMLayoutSetElementInnerText")
	procHTMLayoutGetElementInnerText = mustFindProc("HTMLayoutGetElementInnerText")
	procHTMLayoutGetAttributeByName = mustFindProc("HTMLayoutGetAttributeByName")
	procHTMLayoutSetAttributeByName = mustFindProc("HTMLayoutSetAttributeByName")
	procHTMLayoutGetNthAttribute = mustFindProc("HTMLayoutGetNthAttribute")
	procHTMLayoutGetAttributeCount = mustFindProc("HTMLayoutGetAttributeCount")
	procHTMLayoutGetStyleAttribute = mustFindProc("HTMLayoutGetStyleAttribute")
	procHTMLayoutSetStyleAttribute = mustFindProc("HTMLayoutSetStyleAttribute")
	procHTMLayoutGetElementState = mustFindProc("HTMLayoutGetElementState")
	procHTMLayoutSetElementState = mustFindProc("HTMLayoutSetElementState")
	procHTMLayoutMoveElement = mustFindProc("HTMLayoutMoveElement")
	procHTMLayoutMoveElementEx = mustFindProc("HTMLayoutMoveElementEx")
	procHTMLayoutGetElementLocation = mustFindProc("HTMLayoutGetElementLocation")
	procHTMLayout_UseElement = mustFindProc("HTMLayout_UseElement")
	procHTMLayout_UnuseElement = mustFindProc("HTMLayout_UnuseElement")
}

func mustFindProc(name string) *syscall.Proc {
	proc, err := htmlayoutLib.FindProc(name)
	if err != nil {
		fmt.Printf("找不到函数 %s: %v\n", name, err)
		return nil
	}
	fmt.Printf("加载函数 %s: %v\n", name, proc.Addr())
	return proc
}

// HTMLayoutProcND 调用 HTMLayoutProcND 函数
func HTMLayoutProcND(hwnd uintptr, msg uint32, wparam uintptr, lparam uintptr, handled *bool) int {
	if procHTMLayoutProcND == nil {
		return 0
	}
	ret, _, _ := procHTMLayoutProcND.Call(hwnd, uintptr(msg), wparam, lparam, uintptr(unsafe.Pointer(handled)))
	return int(ret)
}

// HTMLayoutLoadHtmlEx 调用 HTMLayoutLoadHtmlEx 函数
func HTMLayoutLoadHtmlEx(hwnd uintptr, data *byte, dataSize uint32, baseUrl *uint16) bool {
	if procHTMLayoutLoadHtmlEx == nil {
		return false
	}
	ret, _, _ := procHTMLayoutLoadHtmlEx.Call(hwnd, uintptr(unsafe.Pointer(data)), uintptr(dataSize), uintptr(unsafe.Pointer(baseUrl)))
	return ret != 0
}

// HTMLayoutLoadFile 调用 HTMLayoutLoadFile 函数
func HTMLayoutLoadFile(hwnd uintptr, uri *uint16) bool {
	if procHTMLayoutLoadFile == nil {
		return false
	}
	ret, _, _ := procHTMLayoutLoadFile.Call(hwnd, uintptr(unsafe.Pointer(uri)))
	return ret != 0
}

// HTMLayoutSetOption 调用 HTMLayoutSetOption 函数
func HTMLayoutSetOption(hwnd uintptr, option uint32, value uint32) bool {
	if procHTMLayoutSetOption == nil {
		return false
	}
	ret, _, _ := procHTMLayoutSetOption.Call(hwnd, uintptr(option), uintptr(value))
	return ret != 0
}

// HTMLayoutDataReady 调用 HTMLayoutDataReady 函数
func HTMLayoutDataReady(hwnd uintptr, uri *uint16, data *byte, dataSize uint32) bool {
	if procHTMLayoutDataReady == nil {
		return false
	}
	ret, _, _ := procHTMLayoutDataReady.Call(hwnd, uintptr(unsafe.Pointer(uri)), uintptr(unsafe.Pointer(data)), uintptr(dataSize))
	return ret != 0
}

// HTMLayoutWindowAttachEventHandler 调用 HTMLayoutWindowAttachEventHandler 函数
func HTMLayoutWindowAttachEventHandler(hwnd uintptr, proc uintptr, tag uintptr, subscription uint32) int {
	if procHTMLayoutWindowAttachEventHandler == nil {
		return 0
	}
	ret, _, _ := procHTMLayoutWindowAttachEventHandler.Call(hwnd, proc, tag, uintptr(subscription))
	return int(ret)
}

// HTMLayoutWindowDetachEventHandler 调用 HTMLayoutWindowDetachEventHandler 函数
func HTMLayoutWindowDetachEventHandler(hwnd uintptr, proc uintptr, tag uintptr) int {
	if procHTMLayoutWindowDetachEventHandler == nil {
		return 0
	}
	ret, _, _ := procHTMLayoutWindowDetachEventHandler.Call(hwnd, proc, tag)
	return int(ret)
}

// HTMLayoutSetCallback 调用 HTMLayoutSetCallback 函数
func HTMLayoutSetCallback(hwnd uintptr, callback uintptr, param uintptr) {
	if procHTMLayoutSetCallback == nil {
		return
	}
	procHTMLayoutSetCallback.Call(hwnd, callback, param)
}

// HTMLayoutGetRootElement 调用 HTMLayoutGetRootElement 函数
func HTMLayoutGetRootElement(hwnd uintptr, handle *uintptr) int {
	if procHTMLayoutGetRootElement == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetRootElement.Call(hwnd, uintptr(unsafe.Pointer(handle)))
	return int(ret)
}

// HTMLayoutCreateElement 调用 HTMLayoutCreateElement 函数
func HTMLayoutCreateElement(tag *byte, text *uint16, handle *uintptr) int {
	if procHTMLayoutCreateElement == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutCreateElement.Call(uintptr(unsafe.Pointer(tag)), uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(handle)))
	return int(ret)
}

// HTMLayoutFindElement 调用 HTMLayoutFindElement 函数
func HTMLayoutFindElement(hwnd uintptr, pt struct{ X, Y int32 }, handle *uintptr) int {
	if procHTMLayoutFindElement == nil {
		return -1
	}
	ptValue := uintptr(uint32(pt.X)) | (uintptr(uint32(pt.Y)) << 32)
	ret, _, _ := procHTMLayoutFindElement.Call(hwnd, ptValue, uintptr(unsafe.Pointer(handle)))
	return int(ret)
}

// HTMLayoutGetFocusElement 调用 HTMLayoutGetFocusElement 函数
func HTMLayoutGetFocusElement(hwnd uintptr, handle *uintptr) int {
	if procHTMLayoutGetFocusElement == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetFocusElement.Call(hwnd, uintptr(unsafe.Pointer(handle)))
	return int(ret)
}

// HTMLayoutAttachEventHandler 调用 HTMLayoutAttachEventHandler 函数
func HTMLayoutAttachEventHandler(handle uintptr, proc uintptr, tag uintptr) int {
	if procHTMLayoutAttachEventHandler == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutAttachEventHandler.Call(handle, proc, tag)
	return int(ret)
}

// HTMLayoutAttachEventHandlerEx 调用 HTMLayoutAttachEventHandlerEx 函数
func HTMLayoutAttachEventHandlerEx(handle uintptr, proc uintptr, tag uintptr, subscription uint32) int {
	if procHTMLayoutAttachEventHandlerEx == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutAttachEventHandlerEx.Call(handle, proc, tag, uintptr(subscription))
	return int(ret)
}

// HTMLayoutDetachEventHandler 调用 HTMLayoutDetachEventHandler 函数
func HTMLayoutDetachEventHandler(handle uintptr, proc uintptr, tag uintptr) int {
	if procHTMLayoutDetachEventHandler == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutDetachEventHandler.Call(handle, proc, tag)
	return int(ret)
}

// HTMLayoutUpdateElementEx 调用 HTMLayoutUpdateElementEx 函数
func HTMLayoutUpdateElementEx(handle uintptr, flags uint32) int {
	if procHTMLayoutUpdateElementEx == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutUpdateElementEx.Call(handle, uintptr(flags))
	return int(ret)
}

// HTMLayoutSetCapture 调用 HTMLayoutSetCapture 函数
func HTMLayoutSetCapture(handle uintptr) int {
	if procHTMLayoutSetCapture == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutSetCapture.Call(handle)
	return int(ret)
}

// HTMLayoutShowPopup 调用 HTMLayoutShowPopup 函数
func HTMLayoutShowPopup(popupHandle uintptr, anchorHandle uintptr, placement uint32) int {
	if procHTMLayoutShowPopup == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutShowPopup.Call(popupHandle, anchorHandle, uintptr(placement))
	return int(ret)
}

// HTMLayoutShowPopupAt 调用 HTMLayoutShowPopupAt 函数
func HTMLayoutShowPopupAt(popupHandle uintptr, pt struct{ X, Y int32 }, mode uint32) int {
	if procHTMLayoutShowPopupAt == nil {
		return -1
	}
	ptValue := uintptr(uint32(pt.X)) | (uintptr(uint32(pt.Y)) << 32)
	ret, _, _ := procHTMLayoutShowPopupAt.Call(popupHandle, ptValue, uintptr(mode))
	return int(ret)
}

// HTMLayoutHidePopup 调用 HTMLayoutHidePopup 函数
func HTMLayoutHidePopup(popupHandle uintptr) int {
	if procHTMLayoutHidePopup == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutHidePopup.Call(popupHandle)
	return int(ret)
}

// HTMLayoutSelectElements 调用 HTMLayoutSelectElements 函数
func HTMLayoutSelectElements(handle uintptr, selector *byte, callback uintptr, param uintptr) int {
	if procHTMLayoutSelectElements == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutSelectElements.Call(handle, uintptr(unsafe.Pointer(selector)), callback, param)
	return int(ret)
}

// HTMLayoutSelectParent 调用 HTMLayoutSelectParent 函数
func HTMLayoutSelectParent(handle uintptr, selector *byte, depth uint32, parent *uintptr) int {
	if procHTMLayoutSelectParent == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutSelectParent.Call(handle, uintptr(unsafe.Pointer(selector)), uintptr(depth), uintptr(unsafe.Pointer(parent)))
	return int(ret)
}

// HTMLayoutSendEvent 调用 HTMLayoutSendEvent 函数
func HTMLayoutSendEvent(handle uintptr, eventCode uint32, sourceHandle uintptr, reason uintptr, handled *bool) int {
	if procHTMLayoutSendEvent == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutSendEvent.Call(handle, uintptr(eventCode), sourceHandle, reason, uintptr(unsafe.Pointer(handled)))
	return int(ret)
}

// HTMLayoutPostEvent 调用 HTMLayoutPostEvent 函数
func HTMLayoutPostEvent(handle uintptr, eventCode uint32, sourceHandle uintptr, reason uint32) int {
	if procHTMLayoutPostEvent == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutPostEvent.Call(handle, uintptr(eventCode), sourceHandle, uintptr(reason))
	return int(ret)
}

// HTMLayoutGetChildrenCount 调用 HTMLayoutGetChildrenCount 函数
func HTMLayoutGetChildrenCount(handle uintptr, count *uint32) int {
	if procHTMLayoutGetChildrenCount == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetChildrenCount.Call(handle, uintptr(unsafe.Pointer(count)))
	return int(ret)
}

// HTMLayoutGetNthChild 调用 HTMLayoutGetNthChild 函数
func HTMLayoutGetNthChild(handle uintptr, index uint32, child *uintptr) int {
	if procHTMLayoutGetNthChild == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetNthChild.Call(handle, uintptr(index), uintptr(unsafe.Pointer(child)))
	return int(ret)
}

// HTMLayoutGetElementIndex 调用 HTMLayoutGetElementIndex 函数
func HTMLayoutGetElementIndex(handle uintptr, index *int32) int {
	if procHTMLayoutGetElementIndex == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetElementIndex.Call(handle, uintptr(unsafe.Pointer(index)))
	return int(ret)
}

// HTMLayoutGetParentElement 调用 HTMLayoutGetParentElement 函数
func HTMLayoutGetParentElement(handle uintptr, parent *uintptr) int {
	if procHTMLayoutGetParentElement == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetParentElement.Call(handle, uintptr(unsafe.Pointer(parent)))
	return int(ret)
}

// HTMLayoutInsertElement 调用 HTMLayoutInsertElement 函数
func HTMLayoutInsertElement(childHandle uintptr, parentHandle uintptr, index uint32) int {
	if procHTMLayoutInsertElement == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutInsertElement.Call(childHandle, parentHandle, uintptr(index))
	return int(ret)
}

// HTMLayoutDetachElement 调用 HTMLayoutDetachElement 函数
func HTMLayoutDetachElement(handle uintptr) int {
	if procHTMLayoutDetachElement == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutDetachElement.Call(handle)
	return int(ret)
}

// HTMLayoutDeleteElement 调用 HTMLayoutDeleteElement 函数
func HTMLayoutDeleteElement(handle uintptr) int {
	if procHTMLayoutDeleteElement == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutDeleteElement.Call(handle)
	return int(ret)
}

// HTMLayoutCloneElement 调用 HTMLayoutCloneElement 函数
func HTMLayoutCloneElement(handle uintptr, clone *uintptr) int {
	if procHTMLayoutCloneElement == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutCloneElement.Call(handle, uintptr(unsafe.Pointer(clone)))
	return int(ret)
}

// HTMLayoutSwapElements 调用 HTMLayoutSwapElements 函数
func HTMLayoutSwapElements(handle1 uintptr, handle2 uintptr) int {
	if procHTMLayoutSwapElements == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutSwapElements.Call(handle1, handle2)
	return int(ret)
}

// HTMLayoutSetEventRoot 调用 HTMLayoutSetEventRoot 函数
func HTMLayoutSetEventRoot(handle uintptr, prevRoot *uintptr) int {
	if procHTMLayoutSetEventRoot == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutSetEventRoot.Call(handle, uintptr(unsafe.Pointer(prevRoot)))
	return int(ret)
}

// HTMLayoutScrollToView 调用 HTMLayoutScrollToView 函数
func HTMLayoutScrollToView(handle uintptr, flags uint32) int {
	if procHTMLayoutScrollToView == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutScrollToView.Call(handle, uintptr(flags))
	return int(ret)
}

// HTMLayoutGetElementUID 调用 HTMLayoutGetElementUID 函数
func HTMLayoutGetElementUID(handle uintptr, uid *uint32) int {
	if procHTMLayoutGetElementUID == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetElementUID.Call(handle, uintptr(unsafe.Pointer(uid)))
	return int(ret)
}

// HTMLayoutGetElementByUID 调用 HTMLayoutGetElementByUID 函数
func HTMLayoutGetElementByUID(hwnd uintptr, uid uint32, handle *uintptr) int {
	if procHTMLayoutGetElementByUID == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetElementByUID.Call(hwnd, uintptr(uid), uintptr(unsafe.Pointer(handle)))
	return int(ret)
}

// HTMLayoutCallBehaviorMethod 调用 HTMLayoutCallBehaviorMethod 函数
// 注意：params 必须是一级结构体指针（uintptr 形式），不能传指针的指针
func HTMLayoutCallBehaviorMethod(handle uintptr, params uintptr) int {
	if procHTMLayoutCallBehaviorMethod == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutCallBehaviorMethod.Call(handle, params)
	return int(ret)
}

// HTMLayoutCombineURL 调用 HTMLayoutCombineURL 函数
func HTMLayoutCombineURL(handle uintptr, buffer *uint16, maxLen uint32) {
	if procHTMLayoutCombineURL == nil {
		return
	}
	procHTMLayoutCombineURL.Call(handle, uintptr(unsafe.Pointer(buffer)), uintptr(maxLen))
}

// HTMLayoutSortElements 调用 HTMLayoutSortElements 函数
func HTMLayoutSortElements(handle uintptr, start uint32, end uint32, comparator uintptr, arg uintptr) int {
	if procHTMLayoutSortElements == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutSortElements.Call(handle, uintptr(start), uintptr(end), comparator, arg)
	return int(ret)
}

// HTMLayoutSetTimerEx 调用 HTMLayoutSetTimerEx 函数
func HTMLayoutSetTimerEx(handle uintptr, ms uint32, timerId uintptr) int {
	if procHTMLayoutSetTimerEx == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutSetTimerEx.Call(handle, uintptr(ms), timerId)
	return int(ret)
}

// HTMLayoutGetElementHwnd 调用 HTMLayoutGetElementHwnd 函数
func HTMLayoutGetElementHwnd(handle uintptr, hwnd *uintptr, flags int32) int {
	if procHTMLayoutGetElementHwnd == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetElementHwnd.Call(handle, uintptr(unsafe.Pointer(hwnd)), uintptr(flags))
	return int(ret)
}

// HTMLayoutGetElementHtml 调用 HTMLayoutGetElementHtml 函数
func HTMLayoutGetElementHtml(handle uintptr, data **byte, asText bool) int {
	if procHTMLayoutGetElementHtml == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetElementHtml.Call(handle, uintptr(unsafe.Pointer(data)), uintptr(boolToInt(asText)))
	return int(ret)
}

// HTMLayoutGetElementType 调用 HTMLayoutGetElementType 函数
func HTMLayoutGetElementType(handle uintptr, typeName **byte) int {
	if procHTMLayoutGetElementType == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetElementType.Call(handle, uintptr(unsafe.Pointer(typeName)))
	return int(ret)
}

// HTMLayoutSetElementHtml 调用 HTMLayoutSetElementHtml 函数
func HTMLayoutSetElementHtml(handle uintptr, html *byte, htmlSize uint32, mode uint32) int {
	if procHTMLayoutSetElementHtml == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutSetElementHtml.Call(handle, uintptr(unsafe.Pointer(html)), uintptr(htmlSize), uintptr(mode))
	return int(ret)
}

// HTMLayoutSetElementInnerText 调用 HTMLayoutSetElementInnerText 函数
func HTMLayoutSetElementInnerText(handle uintptr, text *byte, textSize uint32) int {
	if procHTMLayoutSetElementInnerText == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutSetElementInnerText.Call(handle, uintptr(unsafe.Pointer(text)), uintptr(textSize))
	return int(ret)
}

// HTMLayoutGetElementInnerText 调用 HTMLayoutGetElementInnerText 函数
func HTMLayoutGetElementInnerText(handle uintptr, data **byte) int {
	if procHTMLayoutGetElementInnerText == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetElementInnerText.Call(handle, uintptr(unsafe.Pointer(data)))
	return int(ret)
}

// HTMLayoutGetAttributeByName 调用 HTMLayoutGetAttributeByName 函数
func HTMLayoutGetAttributeByName(handle uintptr, name *byte, value **uint16) int {
	if procHTMLayoutGetAttributeByName == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetAttributeByName.Call(handle, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(value)))
	return int(ret)
}

// HTMLayoutSetAttributeByName 调用 HTMLayoutSetAttributeByName 函数
func HTMLayoutSetAttributeByName(handle uintptr, name *byte, value *uint16) int {
	if procHTMLayoutSetAttributeByName == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutSetAttributeByName.Call(handle, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(value)))
	return int(ret)
}

// HTMLayoutGetNthAttribute 调用 HTMLayoutGetNthAttribute 函数
func HTMLayoutGetNthAttribute(handle uintptr, index uint32, name **byte, value **uint16) int {
	if procHTMLayoutGetNthAttribute == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetNthAttribute.Call(handle, uintptr(index), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(value)))
	return int(ret)
}

// HTMLayoutGetAttributeCount 调用 HTMLayoutGetAttributeCount 函数
func HTMLayoutGetAttributeCount(handle uintptr, count *uint32) int {
	if procHTMLayoutGetAttributeCount == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetAttributeCount.Call(handle, uintptr(unsafe.Pointer(count)))
	return int(ret)
}

// HTMLayoutGetStyleAttribute 调用 HTMLayoutGetStyleAttribute 函数
func HTMLayoutGetStyleAttribute(handle uintptr, name *byte, value **uint16) int {
	if procHTMLayoutGetStyleAttribute == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetStyleAttribute.Call(handle, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(value)))
	return int(ret)
}

// HTMLayoutSetStyleAttribute 调用 HTMLayoutSetStyleAttribute 函数
func HTMLayoutSetStyleAttribute(handle uintptr, name *byte, value *uint16) int {
	if procHTMLayoutSetStyleAttribute == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutSetStyleAttribute.Call(handle, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(value)))
	return int(ret)
}

// HTMLayoutGetElementState 调用 HTMLayoutGetElementState 函数
func HTMLayoutGetElementState(handle uintptr, state *uint32) int {
	if procHTMLayoutGetElementState == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetElementState.Call(handle, uintptr(unsafe.Pointer(state)))
	return int(ret)
}

// HTMLayoutSetElementState 调用 HTMLayoutSetElementState 函数
func HTMLayoutSetElementState(handle uintptr, addBits uint32, clearBits uint32, shouldUpdate bool) int {
	if procHTMLayoutSetElementState == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutSetElementState.Call(handle, uintptr(addBits), uintptr(clearBits), uintptr(boolToInt(shouldUpdate)))
	return int(ret)
}

// HTMLayoutMoveElement 调用 HTMLayoutMoveElement 函数
func HTMLayoutMoveElement(handle uintptr, x int32, y int32) int {
	if procHTMLayoutMoveElement == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutMoveElement.Call(handle, uintptr(x), uintptr(y))
	return int(ret)
}

// HTMLayoutMoveElementEx 调用 HTMLayoutMoveElementEx 函数
func HTMLayoutMoveElementEx(handle uintptr, x int32, y int32, w int32, h int32) int {
	if procHTMLayoutMoveElementEx == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutMoveElementEx.Call(handle, uintptr(x), uintptr(y), uintptr(w), uintptr(h))
	return int(ret)
}

// HTMLayoutGetElementLocation 调用 HTMLayoutGetElementLocation 函数
func HTMLayoutGetElementLocation(handle uintptr, rect *struct{ Left, Top, Right, Bottom int32 }, flags uint32) int {
	if procHTMLayoutGetElementLocation == nil {
		return -1
	}
	ret, _, _ := procHTMLayoutGetElementLocation.Call(handle, uintptr(unsafe.Pointer(rect)), uintptr(flags))
	return int(ret)
}

// HTMLayout_UseElement 调用 HTMLayout_UseElement 函数
func HTMLayout_UseElement(handle uintptr) int {
	if procHTMLayout_UseElement == nil {
		return -1
	}
	ret, _, _ := procHTMLayout_UseElement.Call(handle)
	return int(ret)
}

// HTMLayout_UnuseElement 调用 HTMLayout_UnuseElement 函数
func HTMLayout_UnuseElement(handle uintptr) int {
	if procHTMLayout_UnuseElement == nil {
		return -1
	}
	ret, _, _ := procHTMLayout_UnuseElement.Call(handle)
	return int(ret)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
