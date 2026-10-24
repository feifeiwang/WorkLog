package gohl

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var BAD_HELEMENT = HELEMENT(unsafe.Pointer(uintptr(0)))

var errorToString = map[HLDOM_RESULT]string{
	HLDOM_OK:                "HLDOM_OK",
	HLDOM_INVALID_HWND:      "HLDOM_INVALID_HWND",
	HLDOM_INVALID_HANDLE:    "HLDOM_INVALID_HANDLE",
	HLDOM_PASSIVE_HANDLE:    "HLDOM_PASSIVE_HANDLE",
	HLDOM_INVALID_PARAMETER: "HLDOM_INVALID_PARAMETER",
	HLDOM_OPERATION_FAILED:  "HLDOM_OPERATION_FAILED",
	HLDOM_OK_NOT_HANDLED:    "HLDOM_OK_NOT_HANDLED",
}

var valueErrorToString = map[VALUE_RESULT]string{
	HV_OK_TRUE:           "HV_OK_TRUE",
	HV_OK:                "HV_OK",
	HV_BAD_PARAMETER:     "HV_BAD_PARAMETER",
	HV_INCOMPATIBLE_TYPE: "HV_INCOMPATIBLE_TYPE",
}

var whitespaceSplitter = regexp.MustCompile(`(\S+)`)

// DomError represents an htmlayout error with an associated
// dom error code
type DomError struct {
	Result  HLDOM_RESULT
	Message string
}

func (e *DomError) Error() string {
	return fmt.Sprintf("%s: %s", errorToString[e.Result], e.Message)
}

func domResultAsString(result HLDOM_RESULT) string {
	return errorToString[result]
}

func domPanic(result int, message ...interface{}) {
	panic(&DomError{HLDOM_RESULT(result), fmt.Sprint(message...)})
}

type ValueError struct {
	Result  VALUE_RESULT
	Message string
}

func (e *ValueError) Error() string {
	return fmt.Sprintf("%s: %s", valueErrorToString[e.Result], e.Message)
}

func valuePanic(result uint32, message ...interface{}) {
	panic(&ValueError{VALUE_RESULT(result), fmt.Sprint(message...)})
}

// Returns the utf-16 encoding of the utf-8 string s,
// with a terminating NUL added.
func stringToUtf16(s string) []uint16 {
	return utf16.Encode([]rune(s + "\x00"))
}

// Returns the utf-8 encoding of the utf-16 sequence s,
// with a terminating NUL removed.
func utf16ToString(s *uint16) string {
	if s == nil {
		panic("null cstring")
	}
	us := make([]uint16, 0, 256)
	for p := uintptr(unsafe.Pointer(s)); ; p += 2 {
		u := *(*uint16)(unsafe.Pointer(p))
		if u == 0 {
			return string(utf16.Decode(us))
		}
		us = append(us, u)
	}
}

func utf16ToStringLength(s *uint16, length int) string {
	if s == nil {
		panic("null cstring")
	}
	us := make([]uint16, 0, 256)
	for p, i := uintptr(unsafe.Pointer(s)), 0; i < length; p, i = p+2, i+1 {
		u := *(*uint16)(unsafe.Pointer(p))
		us = append(us, u)
	}
	return string(utf16.Decode(us))
}

// Returns pointer to the utf-16 encoding of
// the utf-8 string s, with a terminating NUL added.
func stringToUtf16Ptr(s string) *uint16 {
	return &stringToUtf16(s)[0]
}

func use(handle HELEMENT) {
	if dr := HTMLayout_UseElement(uintptr(handle)); dr != HLDOM_OK {
		domPanic(dr, "UseElement")
	}
}

func unuse(handle HELEMENT) {
	if handle != BAD_HELEMENT {
		defer func() {
			if r := recover(); r != nil {
			}
		}()
		HTMLayout_UnuseElement(uintptr(handle))
	}
}

/*
Element

Represents a single DOM element, owns and manages a Handle
*/
type Element struct {
	handle HELEMENT
}

// Constructors
func NewElementFromHandle(h HELEMENT) *Element {
	if h == BAD_HELEMENT {
		return nil
	}
	e := &Element{BAD_HELEMENT}
	e.setHandle(h)
	runtime.SetFinalizer(e, (*Element).finalize)
	return e
}

func NewElement(tagName string) *Element {
	var handle uintptr = 0
	tagBytes := append([]byte(tagName), 0)
	if ret := HTMLayoutCreateElement(&tagBytes[0], nil, &handle); ret != HLDOM_OK {
		domPanic(ret, "Failed to create new element")
	}
	return NewElementFromHandle(HELEMENT(handle))
}

func RootElement(hwnd uint32) *Element {
	var handle uintptr = 0
	if ret := HTMLayoutGetRootElement(uintptr(hwnd), &handle); ret != HLDOM_OK {
		domPanic(ret, "Failed to get root element")
	}
	return NewElementFromHandle(HELEMENT(handle))
}

func FindElement(hwnd uint32, x, y int) *Element {
	var handle uintptr = 0
	pt := struct{ X, Y int32 }{int32(x), int32(y)}
	if ret := HTMLayoutFindElement(uintptr(hwnd), pt, &handle); ret != HLDOM_OK {
		return nil
	}
	if handle == 0 {
		return nil
	}
	return NewElementFromHandle(HELEMENT(handle))
}

func FocusedElement(hwnd uint32) *Element {
	var handle uintptr = 0
	if ret := HTMLayoutGetFocusElement(uintptr(hwnd), &handle); ret != HLDOM_OK {
		domPanic(ret, "Failed to get focus element")
	}
	if handle != 0 {
		return NewElementFromHandle(HELEMENT(handle))
	}
	return nil
}

// Finalizer method, only to be called from Release or by
// the Go runtime
func (e *Element) finalize() {
	// Detach handlers
	if attachedHandlers, hasHandlers := eventHandlers[e.handle]; hasHandlers {
		for _, tag := range attachedHandlers {
			HTMLayoutDetachEventHandler(uintptr(e.handle), uintptr(unsafe.Pointer(goElementProc)), uintptr(tag))
			// Note: tag.Delete() is called in BEHAVIOR_DETACH, so we don't need to call it here
		}
		delete(eventHandlers, e.handle)
	}

	// Note: We don't call unuse() here because the element may have already been
	// destroyed by HTMLayout. HTMLayout manages its own reference counting internally.
	e.handle = BAD_HELEMENT
}

func (e *Element) Release() {
	// Unregister the finalizer so that it does not get called by Go
	// and then explicitly finalize this element
	runtime.SetFinalizer(e, nil)
	e.finalize()
}

func (e *Element) setHandle(h HELEMENT) {
	use(h)
	e.handle = h
}

func (e *Element) Handle() HELEMENT {
	return e.handle
}

func (e *Element) Equals(other *Element) bool {
	return other != nil && e.handle == other.handle
}

// This is the same as AttachHandler, except that behaviors are singleton instances stored
// in a master map.  They may be shared among many elements since they have no state.
// The only reason we keep a separate set of the behaviors is so that the event handler
// dispatch method can tell if an event handler is a behavior or a regular handler.
func (e *Element) attachBehavior(handler *EventHandler) {
	tag := cgoNewHandle(handler)
	if subscription := handler.Subscription(); subscription == HANDLE_ALL {
		if ret := HTMLayoutAttachEventHandler(uintptr(e.handle), uintptr(unsafe.Pointer(goElementProc)), uintptr(tag)); ret != HLDOM_OK {
			tag.Delete()
			domPanic(ret, "Failed to attach event handler to element")
		}
	} else {
		if ret := HTMLayoutAttachEventHandlerEx(uintptr(e.handle), uintptr(unsafe.Pointer(goElementProc)), uintptr(tag), uint32(subscription)); ret != HLDOM_OK {
			tag.Delete()
			domPanic(ret, "Failed to attach event handler to element")
		}
	}
}

func (e *Element) AttachHandler(handler *EventHandler) {
	attachedHandlers, hasAttachments := eventHandlers[e.handle]
	if hasAttachments {
		if _, exists := attachedHandlers[handler]; exists {
			return
		}
	}

	subscription := handler.Subscription()
	subscription &= ^uint32(DISABLE_INITIALIZATION & 0xffffffff)

	tag := cgoNewHandle(handler)
	if subscription == HANDLE_ALL {
		if ret := HTMLayoutAttachEventHandler(uintptr(e.handle), uintptr(unsafe.Pointer(goElementProc)), uintptr(tag)); ret != HLDOM_OK {
			tag.Delete()
			domPanic(ret, "Failed to attach event handler to element")
		}
	} else {
		if ret := HTMLayoutAttachEventHandlerEx(uintptr(e.handle), uintptr(unsafe.Pointer(goElementProc)), uintptr(tag), uint32(subscription)); ret != HLDOM_OK {
			tag.Delete()
			domPanic(ret, "Failed to attach event handler to element")
		}
	}

	if !hasAttachments {
		eventHandlers[e.handle] = make(map[*EventHandler]cgoHandle, 8)
	}
	eventHandlers[e.handle][handler] = tag
}

func (e *Element) DetachHandler(handler *EventHandler) {
	if attachedHandlers, exists := eventHandlers[e.handle]; exists {
		if tag, exists := attachedHandlers[handler]; exists {
			if ret := HTMLayoutDetachEventHandler(uintptr(e.handle), uintptr(unsafe.Pointer(goElementProc)), uintptr(tag)); ret != HLDOM_OK {
				domPanic(ret, "Failed to detach event handler from element")
			}
			tag.Delete()
			delete(attachedHandlers, handler)
			if len(attachedHandlers) == 0 {
				delete(eventHandlers, e.handle)
			}
			return
		}
	}
	panic("cannot detach, handler was not registered")
}

func (e *Element) Update(restyle, restyleDeep, remeasure, remeasureDeep, render bool) {
	var flags uint32
	if restyle {
		if restyleDeep {
			flags |= RESET_STYLE_DEEP
		} else {
			flags |= RESET_STYLE_THIS
		}
	}
	if remeasure {
		if remeasureDeep {
			flags |= MEASURE_DEEP
		} else {
			flags |= MEASURE_INPLACE
		}
	}
	if render {
		flags |= REDRAW_NOW
	}
	if ret := HTMLayoutUpdateElementEx(uintptr(e.handle), flags); ret != HLDOM_OK {
		domPanic(ret, "Failed to update element")
	}
}

func (e *Element) Capture() {
	if ret := HTMLayoutSetCapture(uintptr(e.handle)); ret != HLDOM_OK {
		domPanic(ret, "Failed to set capture for element")
	}
}

func (e *Element) ReleaseCapture() {
	user32 := syscall.NewLazyDLL("user32.dll")
	releaseCapture := user32.NewProc("ReleaseCapture")
	if _, _, err := releaseCapture.Call(); err != nil {
		panic("Failed to release capture for element")
	}
}

func (e *Element) ShowPopup(anchor *Element, placement uint) {
	if anchor == nil {
		return
	}
	if ret := HTMLayoutShowPopup(uintptr(e.handle), uintptr(anchor.handle), uint32(placement)); ret != HLDOM_OK {
		domPanic(ret, "Failed to show popup")
	}
}

func (e *Element) ShowPopupAt(x, y int32, animate bool) {
	pt := struct{ X, Y int32 }{X: x, Y: y}
	mode := uint32(0)
	if animate {
		mode = 1
	}
	if ret := HTMLayoutShowPopupAt(uintptr(e.handle), pt, mode); ret != HLDOM_OK {
		domPanic(ret, "Failed to show popup at position")
	}
}

func (e *Element) HidePopup() {
	if ret := HTMLayoutHidePopup(uintptr(e.handle)); ret != HLDOM_OK {
		domPanic(ret, "Failed to hide popup")
	}
}

func (e *Element) GetElementById(id string) *Element {
	elems := e.Select("#" + id)
	if len(elems) > 0 {
		return elems[0]
	}
	return nil
}

func (e *Element) GetElementsByTagName(tag string) []*Element {
	return e.Select(tag)
}

func (e *Element) Show() *Element {
	e.AddClass("show")
	e.RemoveStyle("display")
	e.Update(true, false, true, false, true)
	return e
}

func (e *Element) Hide() *Element {
	e.RemoveClass("show")
	e.SetStyle("display", "none")
	e.Update(true, false, true, false, true)
	return e
}

// Functions for querying elements

func (e *Element) Select(selector string) []*Element {
	selectorBytes := append([]byte(selector), 0)
	results := make([]*Element, 0, 32)
	handle := cgoNewHandle(&results)
	defer handle.Delete()
	if ret := HTMLayoutSelectElements(uintptr(e.handle), &selectorBytes[0], uintptr(unsafe.Pointer(goSelectCallback)), uintptr(handle)); ret != HLDOM_OK {
		domPanic(ret, "Failed to select dom elements, selector: '", selector, "'")
	}
	return results
}

// Searches up the parent chain to find the first element that matches the given selector.
// Includes the element in the search.  Depth indicates how far the search should progress.
// Depth = 1 means only consider this element.  Depth = 0 means search all the way up to the
// root.  Any other positive value of depth limits the length of the search.
func (e *Element) SelectParentLimit(selector string, depth int) *Element {
	selectorBytes := append([]byte(selector), 0)
	var parent uintptr
	if ret := HTMLayoutSelectParent(uintptr(e.handle), &selectorBytes[0], uint32(depth), &parent); ret != HLDOM_OK {
		domPanic(ret, "Failed to select parent dom elements, selector: '", selector, "'")
	}
	if parent != 0 {
		return NewElementFromHandle(HELEMENT(unsafe.Pointer(parent)))
	}
	return nil
}

func (e *Element) SelectParent(selector string) *Element {
	return e.SelectParentLimit(selector, 0)
}

// For delivering programmatic events to this element.
// Returns true if the event was handled, false otherwise
func (e *Element) SendEvent(eventCode uint, source *Element, reason uint32) bool {
	var handled bool = false
	if ret := HTMLayoutSendEvent(uintptr(e.handle), uint32(eventCode), uintptr(source.handle), uintptr(reason), &handled); ret != HLDOM_OK {
		domPanic(ret, "Failed to send event")
	}
	return handled
}

// For asynchronously delivering programmatic events to this element.
func (e *Element) PostEvent(eventCode uint, source *Element, reason uint32) {
	if ret := HTMLayoutPostEvent(uintptr(e.handle), uint32(eventCode), uintptr(source.handle), uint32(reason)); ret != HLDOM_OK {
		domPanic(ret, "Failed to post event")
	}
}

//
// DOM structure accessors/modifiers:
//

func (e *Element) ChildCount() uint {
	var count uint32
	if ret := HTMLayoutGetChildrenCount(uintptr(e.handle), &count); ret != HLDOM_OK {
		domPanic(ret, "Failed to get child count")
	}
	return uint(count)
}

func (e *Element) Child(index uint) *Element {
	var child uintptr
	if ret := HTMLayoutGetNthChild(uintptr(e.handle), uint32(index), &child); ret != HLDOM_OK {
		domPanic(ret, "Failed to get child at index: ", index)
	}
	return NewElementFromHandle(HELEMENT(unsafe.Pointer(child)))
}

func (e *Element) Children() []*Element {
	slice := make([]*Element, 0, 32)
	for i := uint(0); i < e.ChildCount(); i++ {
		slice = append(slice, e.Child(i))
	}
	return slice
}

func (e *Element) Index() uint {
	var index int32
	if ret := HTMLayoutGetElementIndex(uintptr(e.handle), &index); ret != HLDOM_OK {
		domPanic(ret, "Failed to get element's index")
	}
	return uint(index)
}

func (e *Element) Parent() *Element {
	var parent uintptr
	if ret := HTMLayoutGetParentElement(uintptr(e.handle), &parent); ret != HLDOM_OK {
		domPanic(ret, "Failed to get parent")
	}
	if parent != 0 {
		return NewElementFromHandle(HELEMENT(unsafe.Pointer(parent)))
	}
	return nil
}

func (e *Element) InsertChild(child *Element, index uint) {
	if ret := HTMLayoutInsertElement(uintptr(child.handle), uintptr(e.handle), uint32(index)); ret != HLDOM_OK {
		domPanic(ret, "Failed to insert child element at index: ", index)
	}
}

func (e *Element) AppendChild(child *Element) {
	count := e.ChildCount()
	if ret := HTMLayoutInsertElement(uintptr(child.handle), uintptr(e.handle), uint32(count)); ret != HLDOM_OK {
		domPanic(ret, "Failed to append child element")
	}
}

func (e *Element) Detach() {
	if ret := HTMLayoutDetachElement(uintptr(e.handle)); ret != HLDOM_OK {
		domPanic(ret, "Failed to detach element from dom")
	}
}

func (e *Element) Delete() {
	if ret := HTMLayoutDeleteElement(uintptr(e.handle)); ret != HLDOM_OK {
		domPanic(ret, "Failed to delete element from dom")
	}
	e.finalize()
}

// Makes a deep clone of the receiver, the resulting subtree is not attached to the dom.
func (e *Element) Clone() *Element {
	var clone uintptr
	if ret := HTMLayoutCloneElement(uintptr(e.handle), &clone); ret != HLDOM_OK {
		domPanic(ret, "Failed to clone element")
	}
	return NewElementFromHandle(HELEMENT(unsafe.Pointer(clone)))
}

func (e *Element) Swap(other *Element) {
	if ret := HTMLayoutSwapElements(uintptr(e.handle), uintptr(other.handle)); ret != HLDOM_OK {
		domPanic(ret, "Failed to swap elements")
	}
}

func (e *Element) Root() *Element {
	var root uintptr
	if ret := HTMLayoutGetRootElement(uintptr(e.Hwnd()), &root); ret != HLDOM_OK {
		domPanic(ret, "Failed to get root element")
	}
	if root != 0 {
		return NewElementFromHandle(HELEMENT(unsafe.Pointer(root)))
	}
	return nil
}

func (e *Element) SetEventRoot() *Element {
	var prevRoot uintptr
	if ret := HTMLayoutSetEventRoot(uintptr(e.handle), &prevRoot); ret != HLDOM_OK {
		domPanic(ret, "Failed to set event root")
	}
	if prevRoot != 0 {
		return NewElementFromHandle(HELEMENT(unsafe.Pointer(prevRoot)))
	}
	return nil
}

func (e *Element) ResetEventRoot() {
	HTMLayoutSetEventRoot(0, nil)
}

func (e *Element) ScrollToView(toTop bool) {
	flags := uint32(0)
	if toTop {
		flags = 1
	}
	if ret := HTMLayoutScrollToView(uintptr(e.handle), flags); ret != HLDOM_OK {
		domPanic(ret, "Failed to scroll element into view")
	}
}

func (e *Element) GetElementUid() uint32 {
	var uid uint32
	if ret := HTMLayoutGetElementUID(uintptr(e.handle), &uid); ret != HLDOM_OK {
		domPanic(ret, "Failed to get element uid")
	}
	return uid
}

func ElementByUid(hwnd uint32, uid uint32) *Element {
	var he uintptr
	if ret := HTMLayoutGetElementByUID(uintptr(hwnd), uid, &he); ret != HLDOM_OK {
		return nil
	}
	if he != 0 {
		return NewElementFromHandle(HELEMENT(unsafe.Pointer(he)))
	}
	return nil
}

func CreateElement(tag string, text string) *Element {
	tagBytes := append([]byte(tag), 0)
	textUtf16, _ := syscall.UTF16PtrFromString(text)
	var he uintptr
	if ret := HTMLayoutCreateElement(&tagBytes[0], (*uint16)(unsafe.Pointer(textUtf16)), &he); ret != HLDOM_OK {
		domPanic(ret, "Failed to create element")
	}
	return NewElementFromHandle(HELEMENT(unsafe.Pointer(he)))
}

func (e *Element) CallBehaviorMethod(methodID uint) bool {
	params := struct {
		methodID uint32
	}{
		methodID: uint32(methodID),
	}
	paramsPtr := uintptr(unsafe.Pointer(&params))
	ret := HTMLayoutCallBehaviorMethod(uintptr(e.handle), paramsPtr)
	return ret == HLDOM_OK
}

func (e *Element) CombineUrl(url string, maxLen int) string {
	buf := make([]uint16, maxLen)
	for i, c := range url {
		if i >= maxLen-1 {
			break
		}
		buf[i] = uint16(c)
	}
	HTMLayoutCombineURL(uintptr(e.handle), &buf[0], uint32(maxLen))
	result := make([]uint16, 0, maxLen)
	for _, c := range buf {
		if c == 0 {
			break
		}
		result = append(result, c)
	}
	return string(utf16.Decode(result))
}

// Sorts 'count' child elements starting at index 'start'.  Uses comparator to define the
// order.  Comparator should return -1, or 0, or 1 to indicate less, equal or greater
func (e *Element) SortChildrenRange(start, count uint, comparator func(*Element, *Element) int) {
	end := start + count
	arg := uintptr(unsafe.Pointer(&comparator))
	if ret := HTMLayoutSortElements(uintptr(e.handle), uint32(start), uint32(end), uintptr(unsafe.Pointer(goElementComparator)), arg); ret != HLDOM_OK {
		domPanic(ret, "Failed to sort elements")
	}
}

func (e *Element) SortChildren(comparator func(*Element, *Element) int) {
	e.SortChildrenRange(0, e.ChildCount(), comparator)
}

func (e *Element) SetTimer(ms uint, timerId uintptr) {
	if ret := HTMLayoutSetTimerEx(uintptr(e.handle), uint32(ms), timerId); ret != HLDOM_OK {
		domPanic(ret, "Failed to set timer")
	}
}

func (e *Element) CancelTimer() {
	e.SetTimer(0, 0)
}

func (e *Element) Hwnd() uint32 {
	var hwnd uintptr
	if ret := HTMLayoutGetElementHwnd(uintptr(e.handle), &hwnd, 0); ret != HLDOM_OK {
		domPanic(ret, "Failed to get element's hwnd")
	}
	return uint32(hwnd)
}

func (e *Element) RootHwnd() uint32 {
	var hwnd uintptr
	if ret := HTMLayoutGetElementHwnd(uintptr(e.handle), &hwnd, 1); ret != HLDOM_OK {
		domPanic(ret, "Failed to get element's root hwnd")
	}
	return uint32(hwnd)
}

func (e *Element) Html() string {
	var data *byte
	ret := HTMLayoutGetElementHtml(uintptr(e.handle), &data, false)
	if ret != HLDOM_OK || data == nil {
		return ""
	}
	s := unsafe.Slice(data, 65536)
	for i, c := range s {
		if c == 0 {
			return string(s[:i])
		}
	}
	return string(s)
}

func (e *Element) OuterHtml() string {
	var data *byte
	ret := HTMLayoutGetElementHtml(uintptr(e.handle), &data, true)
	if ret != HLDOM_OK || data == nil {
		return ""
	}
	s := unsafe.Slice(data, 65536)
	for i, c := range s {
		if c == 0 {
			return string(s[:i])
		}
	}
	return string(s)
}

func (e *Element) Type() string {
	var data *byte
	ret := HTMLayoutGetElementType(uintptr(e.handle), &data)
	if ret != HLDOM_OK || data == nil {
		return ""
	}
	s := unsafe.Slice(data, 256)
	for i, c := range s {
		if c == 0 {
			return string(s[:i])
		}
	}
	return string(s)
}

func (e *Element) SetHtml(html string) {
	if html == "" {
		for e.ChildCount() > 0 {
			child := e.Child(0)
			if child == nil {
				break
			}
			child.Delete()
		}
		return
	}
	htmlBytes := []byte(html)
	if ret := HTMLayoutSetElementHtml(uintptr(e.handle), &htmlBytes[0], uint32(len(html)), SIH_REPLACE_CONTENT); ret != HLDOM_OK {
		domPanic(ret, "Failed to replace element's html")
	}
}

func (e *Element) PrependHtml(prefix string) {
	prefixBytes := []byte(prefix)
	if ret := HTMLayoutSetElementHtml(uintptr(e.handle), &prefixBytes[0], uint32(len(prefix)), SIH_INSERT_AT_START); ret != HLDOM_OK {
		domPanic(ret, "Failed to prepend to element's html")
	}
}

func (e *Element) AppendHtml(suffix string) {
	suffixBytes := []byte(suffix)
	if ret := HTMLayoutSetElementHtml(uintptr(e.handle), &suffixBytes[0], uint32(len(suffix)), SIH_APPEND_AFTER_LAST); ret != HLDOM_OK {
		domPanic(ret, "Failed to append to element's html")
	}
}

func (e *Element) SetText(text string) {
	if text == "" {
		var empty = []byte{0}
		if ret := HTMLayoutSetElementInnerText(uintptr(e.handle), &empty[0], 0); ret != HLDOM_OK {
			domPanic(ret, "Failed to replace element's text")
		}
		return
	}
	textBytes := []byte(text)
	if ret := HTMLayoutSetElementInnerText(uintptr(e.handle), &textBytes[0], uint32(len(text))); ret != HLDOM_OK {
		domPanic(ret, "Failed to replace element's text")
	}
}

func (e *Element) Text() string {
	var data *byte
	ret := HTMLayoutGetElementInnerText(uintptr(e.handle), &data)
	if ret != HLDOM_OK || data == nil {
		return ""
	}
	s := unsafe.Slice(data, 65536)
	for i, c := range s {
		if c == 0 {
			return string(s[:i])
		}
	}
	return string(s)
}

//
// HTML attribute accessors/modifiers:
//

// Returns the value of attr and a boolean indicating whether or not that attr exists.
// If the boolean is true, then the returned string is valid.
func (e *Element) Attr(key string) (string, bool) {
	var szValue *uint16
	keyBytes := append([]byte(key), 0)
	ret := HTMLayoutGetAttributeByName(uintptr(e.handle), &keyBytes[0], &szValue)
	if ret != HLDOM_OK {
		return "", false
	}
	if szValue != nil {
		return utf16ToString(szValue), true
	}
	return "", false
}

func (e *Element) AttrAsFloat(key string) (float64, bool, error) {
	var f float64
	var err error
	if s, exists := e.Attr(key); !exists {
		return 0.0, false, nil
	} else if f, err = strconv.ParseFloat(s, 64); err != nil {
		return 0.0, true, err
	}
	return float64(f), true, nil
}

func (e *Element) AttrAsInt(key string) (int, bool, error) {
	var i int
	var err error
	if s, exists := e.Attr(key); !exists {
		return 0, false, nil
	} else if i, err = strconv.Atoi(s); err != nil {
		return 0, true, err
	}
	return i, true, nil
}

func (e *Element) SetAttr(key string, value interface{}) {
	keyBytes := append([]byte(key), 0)
	var ret int = HLDOM_OK
	switch v := value.(type) {
	case string:
		ret = HTMLayoutSetAttributeByName(uintptr(e.handle), &keyBytes[0], stringToUtf16Ptr(v))
	case float32:
		ret = HTMLayoutSetAttributeByName(uintptr(e.handle), &keyBytes[0], stringToUtf16Ptr(strconv.FormatFloat(float64(v), 'g', -1, 64)))
	case float64:
		ret = HTMLayoutSetAttributeByName(uintptr(e.handle), &keyBytes[0], stringToUtf16Ptr(strconv.FormatFloat(float64(v), 'g', -1, 64)))
	case int:
		ret = HTMLayoutSetAttributeByName(uintptr(e.handle), &keyBytes[0], stringToUtf16Ptr(strconv.Itoa(v)))
	case int32:
		ret = HTMLayoutSetAttributeByName(uintptr(e.handle), &keyBytes[0], stringToUtf16Ptr(strconv.FormatInt(int64(v), 10)))
	case int64:
		ret = HTMLayoutSetAttributeByName(uintptr(e.handle), &keyBytes[0], stringToUtf16Ptr(strconv.FormatInt(v, 10)))
	case nil:
		ret = HTMLayoutSetAttributeByName(uintptr(e.handle), &keyBytes[0], nil)
	default:
		panic(fmt.Sprintf("Don't know how to format this argument type: %s", reflect.TypeOf(v)))
	}
	if ret != HLDOM_OK {
		domPanic(ret, "Failed to set attribute: "+key)
	}
}

func (e *Element) RemoveAttr(key string) {
	e.SetAttr(key, nil)
}

func (e *Element) AttrByIndex(index int) (string, string) {
	var szValue *uint16
	var szName *byte
	ret := HTMLayoutGetNthAttribute(uintptr(e.handle), uint32(index), &szName, &szValue)
	if ret != HLDOM_OK {
		return "", ""
	}
	name := ""
	if szName != nil {
		name = string(unsafe.Slice(szName, 256))
		for i, c := range name {
			if c == 0 {
				name = name[:i]
				break
			}
		}
	}
	value := ""
	if szValue != nil {
		value = utf16ToString(szValue)
	}
	return name, value
}

func (e *Element) AttrCount() uint {
	var count uint32 = 0
	if ret := HTMLayoutGetAttributeCount(uintptr(e.handle), &count); ret != HLDOM_OK {
		domPanic(ret, "Failed to get attribute count")
	}
	return uint(count)
}

//
// CSS style attribute accessors/mutators
//

// Returns the value of the style and a boolean indicating whether or not that style exists.
// If the boolean is true, then the returned string is valid.
func (e *Element) Style(key string) (string, bool) {
	var szValue *uint16
	keyBytes := append([]byte(key), 0)
	ret := HTMLayoutGetStyleAttribute(uintptr(e.handle), &keyBytes[0], &szValue)
	if ret != HLDOM_OK {
		return "", false
	}
	if szValue != nil {
		return utf16ToString(szValue), true
	}
	return "", false
}

func (e *Element) SetStyle(key string, value interface{}) {
	keyBytes := append([]byte(key), 0)
	var valuePtr *uint16 = nil

	switch v := value.(type) {
	case string:
		valuePtr = stringToUtf16Ptr(v)
	case float32:
		valuePtr = stringToUtf16Ptr(strconv.FormatFloat(float64(v), 'g', -1, 64))
	case float64:
		valuePtr = stringToUtf16Ptr(strconv.FormatFloat(float64(v), 'g', -1, 64))
	case int:
		valuePtr = stringToUtf16Ptr(strconv.Itoa(v))
	case int32:
		valuePtr = stringToUtf16Ptr(strconv.FormatInt(int64(v), 10))
	case int64:
		valuePtr = stringToUtf16Ptr(strconv.FormatInt(v, 10))
	case nil:
		valuePtr = nil
	default:
		panic(fmt.Sprintf("Don't know how to format this argument type: %s", reflect.TypeOf(v)))
	}

	if ret := HTMLayoutSetStyleAttribute(uintptr(e.handle), &keyBytes[0], valuePtr); ret != HLDOM_OK {
		domPanic(ret, "Failed to set style: "+key)
	}
}

func (e *Element) RemoveStyle(key string) {
	e.SetStyle(key, nil)
}

func (e *Element) ClearStyles(key string) {
	if ret := HTMLayoutSetStyleAttribute(uintptr(e.handle), nil, nil); ret != HLDOM_OK {
		domPanic(ret, "Failed to clear all styles")
	}
}

//
// Element state manipulation
//

// Gets the whole set of state flags for this element
func (e *Element) StateFlags() uint32 {
	var state uint32
	if ret := HTMLayoutGetElementState(uintptr(e.handle), &state); ret != HLDOM_OK {
		domPanic(ret, "Failed to get element state flags")
	}
	return state
}

// Replaces the whole set of state flags with the specified value
func (e *Element) SetStateFlags(flags uint32) {
	shouldUpdate := true
	if ret := HTMLayoutSetElementState(uintptr(e.handle), flags, ^flags, shouldUpdate); ret != HLDOM_OK {
		domPanic(ret, "Failed to set element state flags")
	}
}

// Returns true if the specified flag is "on"
func (e *Element) State(flag uint32) bool {
	return e.StateFlags()&flag != 0
}

// Sets the specified flag to "on" or "off" according to the value of the provided boolean
func (e *Element) SetState(flag uint32, on bool) {
	addBits := uint32(0)
	clearBits := uint32(0)
	if on {
		addBits = flag
	} else {
		clearBits = flag
	}
	shouldUpdate := true
	if ret := HTMLayoutSetElementState(uintptr(e.handle), addBits, clearBits, shouldUpdate); ret != HLDOM_OK {
		domPanic(ret, "Failed to set element state flag")
	}
}

//
// Functions for retrieving/setting the various dimensions of an element
//

func (e *Element) Move(x, y int) {
	if ret := HTMLayoutMoveElement(uintptr(e.handle), int32(x), int32(y)); ret != HLDOM_OK {
		domPanic(ret, "Failed to move element")
	}
}

func (e *Element) Resize(x, y, w, h int) {
	if ret := HTMLayoutMoveElementEx(uintptr(e.handle), int32(x), int32(y), int32(w), int32(h)); ret != HLDOM_OK {
		domPanic(ret, "Failed to resize element")
	}
}

func (e *Element) getRect(rectTypeFlags uint32) (left, top, right, bottom int) {
	r := struct{ Left, Top, Right, Bottom int32 }{}
	if ret := HTMLayoutGetElementLocation(uintptr(e.handle), &r, rectTypeFlags); ret != HLDOM_OK {
		domPanic(ret, "Failed to get element rect")
	}
	return int(r.Left), int(r.Top), int(r.Right), int(r.Bottom)
}

func (e *Element) ContentBox() (left, top, right, bottom int) {
	return e.getRect(CONTENT_BOX)
}

func (e *Element) ContentViewBox() (left, top, right, bottom int) {
	return e.getRect(CONTENT_BOX | VIEW_RELATIVE)
}

func (e *Element) ContentBoxSize() (width, height int) {
	l, t, r, b := e.getRect(CONTENT_BOX)
	return int(r - l), int(b - t)
}

func (e *Element) PaddingBox() (left, top, right, bottom int) {
	return e.getRect(PADDING_BOX)
}

func (e *Element) PaddingViewBox() (left, top, right, bottom int) {
	return e.getRect(PADDING_BOX | VIEW_RELATIVE)
}

func (e *Element) PaddingBoxSize() (width, height int) {
	l, t, r, b := e.getRect(PADDING_BOX)
	return int(r - l), int(b - t)
}

func (e *Element) BorderBox() (left, top, right, bottom int) {
	return e.getRect(BORDER_BOX)
}

func (e *Element) BorderViewBox() (left, top, right, bottom int) {
	return e.getRect(BORDER_BOX | VIEW_RELATIVE)
}

func (e *Element) BorderBoxSize() (width, height int) {
	l, t, r, b := e.getRect(BORDER_BOX)
	return int(r - l), int(b - t)
}

func (e *Element) MarginBox() (left, top, right, bottom int) {
	return e.getRect(MARGIN_BOX)
}

func (e *Element) MarginViewBox() (left, top, right, bottom int) {
	return e.getRect(MARGIN_BOX | VIEW_RELATIVE)
}

func (e *Element) MarginBoxSize() (width, height int) {
	l, t, r, b := e.getRect(MARGIN_BOX)
	return int(r - l), int(b - t)
}

//
// Functions for retrieving/setting the value in widget input controls
//

type textValueParams struct {
	MethodId uint32
	Text     *uint16
	Length   uint32
}

func (e *Element) ValueAsString() (string, error) {
	args := &textValueParams{MethodId: GET_TEXT_VALUE}
	argsPtr := uintptr(unsafe.Pointer(args))
	ret := HTMLayoutCallBehaviorMethod(uintptr(e.handle), argsPtr)
	if ret == HLDOM_OK_NOT_HANDLED {
		return "", errors.New("HLDOM_OK_NOT_HANDLED: This type of element does not provide data in this way")
	} else if ret != HLDOM_OK {
		return "", errors.New("Could not get text value")
	}
	if args.Text == nil {
		return "", errors.New("Nil string pointer")
	}
	return utf16ToStringLength(args.Text, int(args.Length)), nil
}

func (e *Element) SetValue(value interface{}) error {
	switch v := value.(type) {
	case string:
		// Length 必须是 UTF-16 码元数（与 GET_TEXT_VALUE 返回口径一致），
		// 不能传 UTF-8 字节数——中文 3 字节/字会让控件多读尾部垃圾字符
		u16 := stringToUtf16(v)
		args := &textValueParams{
			MethodId: SET_TEXT_VALUE,
			Text:     &u16[0],
			Length:   uint32(len(u16) - 1), // 去掉结尾 NUL
		}
		argsPtr := uintptr(unsafe.Pointer(args))
		ret := HTMLayoutCallBehaviorMethod(uintptr(e.handle), argsPtr)
		if ret == HLDOM_OK_NOT_HANDLED {
			return errors.New("HLDOM_OK_NOT_HANDLED: This type of element does not accept data in this way")
		} else if ret != HLDOM_OK {
			return errors.New("Could not set text value")
		}
		return nil
	default:
		return errors.New("Don't know how to set values of this type")
	}
}

//
// The following are not strictly wrappers of htmlayout functions, but rather convenience
// functions that are helpful in common use cases
//

func (e *Element) Describe() string {
	s := e.Type()
	if value, exists := e.Attr("id"); exists {
		s += "#" + value
	}
	if value, exists := e.Attr("class"); exists {
		values := strings.Split(value, " ")
		for _, v := range values {
			s += "." + v
		}
	}
	return s
}

// Returns the first of the child elements matching the selector.  If no elements
// match, the function panics
func (e *Element) SelectFirst(selector string) *Element {
	results := e.Select(selector)
	if len(results) == 0 {
		panic(fmt.Sprintf("No elements match selector '%s'", selector))
	}
	return results[0]
}

// Returns the only child element that matches the selector.  If no elements match
// or more than one element matches, the function panics
func (e *Element) SelectUnique(selector string) *Element {
	results := e.Select(selector)
	if len(results) == 0 {
		panic(fmt.Sprintf("No elements match selector '%s'", selector))
	} else if len(results) > 1 {
		panic(fmt.Sprintf("More than one element match selector '%s'", selector))
	}
	return results[0]
}

// A wrapper of SelectUnique that auto-prepends a hash to the provided id.
// Useful when selecting elements base on a programmatically retrieved id (which does
// not already have the hash on it)
func (e *Element) SelectId(id string) *Element {
	return e.SelectUnique(fmt.Sprintf("#%s", id))
}

//
// Functions for manipulating the set of classes applied to this element:
//

// Returns true if the specified class is among those listed in the "class" attribute.
func (e *Element) HasClass(class string) bool {
	if classList, exists := e.Attr("class"); !exists {
		return false
	} else if classes := whitespaceSplitter.FindAllString(classList, -1); classes == nil {
		return false
	} else {
		for _, item := range classes {
			if class == item {
				return true
			}
		}
	}
	return false
}

// Adds the specified class to the classes listed in the "class" attribute, or does nothing
// if this class is already included in the list.
func (e *Element) AddClass(class string) {
	if classList, exists := e.Attr("class"); !exists {
		e.SetAttr("class", class)
	} else if classes := whitespaceSplitter.FindAllString(classList, -1); classes == nil {
		e.SetAttr("class", class)
	} else {
		for _, item := range classes {
			if class == item {
				return
			}
		}
		classes = append(classes, class)
		e.SetAttr("class", strings.Join(classes, " "))
	}
}

// Removes the specified class from the classes listed in the "class" attribute, or does nothing
// if this class is not included in the list.
func (e *Element) RemoveClass(class string) {
	if classList, exists := e.Attr("class"); exists {
		if classes := whitespaceSplitter.FindAllString(classList, -1); classes != nil {
			for i, item := range classes {
				if class == item {
					// Delete the item from the list
					classes = append(classes[:i], classes[i+1:]...)
					e.SetAttr("class", strings.Join(classes, " "))
					return
				}
			}
		}
	}
}
