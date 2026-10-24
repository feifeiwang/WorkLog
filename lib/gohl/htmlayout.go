package gohl

import (
	"errors"
	"log"
	"syscall"
	"unsafe"
)

type NotifyHandler struct {
	Behaviors          map[string]*EventHandler
	OnCreateControl    func(params *NmhlCreateControl) uintptr
	OnControlCreated   func(params *NmhlCreateControl) uintptr
	OnDestroyControl   func(params *NmhlDestroyControl) uintptr
	OnLoadData         func(params *NmhlLoadData) uintptr
	OnDataLoaded       func(params *NmhlDataLoaded) uintptr
	OnDocumentComplete func() uintptr
}

type EventHandler struct {
	OnAttached      func(he HELEMENT)
	OnDetached      func(he HELEMENT)
	OnMouse         func(he HELEMENT, params *MouseParams) bool
	OnKey           func(he HELEMENT, params *KeyParams) bool
	OnFocus         func(he HELEMENT, params *FocusParams) bool
	OnDraw          func(he HELEMENT, params *DrawParams) bool
	OnTimer         func(he HELEMENT, params *TimerParams) bool
	OnBehaviorEvent func(he HELEMENT, params *BehaviorEventParams) bool
	OnMethodCall    func(he HELEMENT, params *MethodParams) bool
	OnDataArrived   func(he HELEMENT, params *DataArrivedParams) bool
	OnSize          func(he HELEMENT)
	OnScroll        func(he HELEMENT, params *ScrollParams) bool
	OnExchange      func(he HELEMENT, params *ExchangeParams) bool
	OnGesture       func(he HELEMENT, params *GestureParams) bool
}

func (e *EventHandler) Subscription() uint32 {
	var subscription uint32 = 0
	add := func(f interface{}, flag uint32) {
		if f != nil {
			subscription |= flag
		}
	}

	// OnAttached and OnDetached purposely omitted, since we must receive these events
	add(e.OnMouse, HANDLE_MOUSE)
	add(e.OnKey, HANDLE_KEY)
	add(e.OnFocus, HANDLE_FOCUS)
	add(e.OnDraw, HANDLE_DRAW)
	add(e.OnTimer, HANDLE_TIMER)
	add(e.OnBehaviorEvent, HANDLE_BEHAVIOR_EVENT)
	add(e.OnMethodCall, HANDLE_METHOD_CALL)
	add(e.OnDataArrived, HANDLE_DATA_ARRIVED)
	add(e.OnSize, HANDLE_SIZE)
	add(e.OnScroll, HANDLE_SCROLL)
	add(e.OnExchange, HANDLE_EXCHANGE)
	add(e.OnGesture, HANDLE_GESTURE)

	return subscription
}

// cstringToString converts a *byte (C char*) to a Go string
func cstringToString(cstr *byte) string {
	if cstr == nil {
		return ""
	}

	// Convert *byte to []byte
	b := make([]byte, 0)
	for p := uintptr(unsafe.Pointer(cstr)); ; p++ {
		ch := *(*byte)(unsafe.Pointer(p))
		if ch == 0 {
			break
		}
		b = append(b, ch)
	}

	return string(b)
}

const (
	TRUE  = 1
	FALSE = 0
)

var (
	notifyHandlers      = make(map[uintptr]*NotifyHandler, 8)
	windowEventHandlers = make(map[uint32]*EventHandler, 8)
	windowEventHandles  = make(map[uint32]cgoHandle, 8)
	eventHandlers       = make(map[HELEMENT]map[*EventHandler]cgoHandle, 128)
	behaviors           = make(map[*EventHandler]int, 32)
)

type HELEMENT uintptr
type HLDOM_RESULT int
type VALUE_RESULT uint32

type Point struct {
	X int32
	Y int32
}

type Size struct {
	Cx int32
	Cy int32
}

type Rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type JsonValue struct {
	T uint32
	U uint32
	D uint64
}

type InitializationParams struct {
	Cmd uint32
}

type MouseParams struct {
	Cmd         uint32 // MouseEvents
	Target      HELEMENT
	Pos         Point
	DocumentPos Point
	ButtonState uint32
	AltState    uint32
	CursorType  uint32
	IsOnIcon    int32

	Dragging     HELEMENT
	DraggingMode uint32
}

type KeyParams struct {
	Cmd      uint32 // KeyEvents
	Target   HELEMENT
	KeyCode  uint32
	AltState uint32
}

type FocusParams struct {
	Cmd          uint32 // FocusEvents
	Target       HELEMENT
	ByMouseClick int32 // boolean
	Cancel       int32 // boolean
}

type DrawParams struct {
	Cmd      uint32 // DrawEvents
	Hdc      uintptr
	Area     Rect
	reserved uint32
}

type TimerParams struct {
	TimerId uint64
}

type BehaviorEventParams struct {
	Cmd    uint32 // Behavior events
	Target HELEMENT
	Source HELEMENT
	Reason uint32
	Data   JsonValue
}

type MethodParams struct {
	MethodId uint32
}

// TODO: Add all the structures derived from MethodParams here...

type DataArrivedParams struct {
	Initiator HELEMENT
	Data      *byte
	DataSize  uint32
	DataType  uint32
	Status    uint32
	Uri       *uint16 // Wide character string
}

type ScrollParams struct {
	Cmd      uint32
	Target   HELEMENT
	Pos      int32
	Vertical int32 // bool
}

type ExchangeParams struct {
	Cmd       uint32
	Target    HELEMENT
	Pos       Point
	PosView   Point
	DataTypes uint32
	DragCmd   uint32
	FetchData uintptr // func pointer: typedef BOOL CALLBACK FETCH_EXCHANGE_DATA(EXCHANGE_PARAMS* params, UINT data_type, LPCBYTE* ppDataStart, UINT* pDataLength );
}

type GestureParams struct {
	Cmd       uint32
	Target    HELEMENT
	Pos       Point
	PosView   Point
	Flags     uint32
	DeltaTime uint32
	DeltaXY   Size
	DeltaV    float64
}

// Notify structures

type NMHDR struct {
	HwndFrom uint32
	IdFrom   uintptr
	Code     uint32
}

type NmhlCreateControl struct {
	Header         NMHDR
	Element        HELEMENT
	InHwndParent   uint32
	OutHwndControl uint32
	reserved1      int32
	reserved2      int32
}

type NmhlDestroyControl struct {
	Header           NMHDR
	Element          HELEMENT
	InOutHwndControl uint32
	reserved1        int32
}

type NmhlLoadData struct {
	Header      NMHDR
	Uri         *uint16
	OutData     uintptr
	OutDataSize int32
	DataType    uint32
	Principal   HELEMENT
	Initiator   HELEMENT
}

type NmhlDataLoaded struct {
	Header   NMHDR
	Uri      *uint16
	Data     uintptr
	DataSize int32
	DataType uint32
	Status   uint32
}

type NmhlAttachBehavior struct {
	Header        NMHDR
	Element       HELEMENT
	BehaviorName  *byte
	ElementProc   uintptr
	ElementTag    uintptr
	ElementEvents uint32
}

// Main event handler that dispatches to the right element handler
var goElementProc = syscall.NewCallback(func(tag uintptr, he unsafe.Pointer, evtg uint32, params unsafe.Pointer) uintptr {
	if tag == 0 || params == nil || he == nil {
		return 0
	}
	handler := cgoHandle(tag).Value().(*EventHandler)
	if handler == nil {
		return 0
	}
	handled := false

	switch evtg {
	case HANDLE_INITIALIZATION:
		if p := (*InitializationParams)(params); p.Cmd == BEHAVIOR_ATTACH {
			if handler.OnAttached != nil {
				handler.OnAttached(HELEMENT(he))
			}
		} else if p.Cmd == BEHAVIOR_DETACH {
			if handler.OnDetached != nil {
				handler.OnDetached(HELEMENT(he))
			}

			if behaviorRefCount, exists := behaviors[handler]; exists {
				behaviorRefCount--
				if behaviorRefCount == 0 {
					delete(behaviors, handler)
				} else {
					behaviors[handler] = behaviorRefCount
				}
			}
			cgoHandle(tag).Delete()
		}
		handled = true
	case HANDLE_MOUSE:
		if handler.OnMouse != nil {
			p := (*MouseParams)(params)
			handled = handler.OnMouse(HELEMENT(he), p)
		}
	case HANDLE_KEY:
		if handler.OnKey != nil {
			p := (*KeyParams)(params)
			handled = handler.OnKey(HELEMENT(he), p)
		}
	case HANDLE_FOCUS:
		if handler.OnFocus != nil {
			p := (*FocusParams)(params)
			handled = handler.OnFocus(HELEMENT(he), p)
		}
	case HANDLE_DRAW:
		if handler.OnDraw != nil {
			p := (*DrawParams)(params)
			handled = handler.OnDraw(HELEMENT(he), p)
		}
	case HANDLE_TIMER:
		log.Printf("[goElementProc] HANDLE_TIMER received, he=%v, params=%v", he, params)
		if handler.OnTimer != nil {
			p := (*TimerParams)(params)
			log.Printf("[goElementProc] TimerParams.TimerId=%d", p.TimerId)
			handled = handler.OnTimer(HELEMENT(he), p)
		}
	case HANDLE_BEHAVIOR_EVENT:
		if handler.OnBehaviorEvent != nil {
			p := (*BehaviorEventParams)(params)
			handled = handler.OnBehaviorEvent(HELEMENT(he), p)
		}
	case HANDLE_METHOD_CALL:
		if handler.OnMethodCall != nil {
			p := (*MethodParams)(params)
			handled = handler.OnMethodCall(HELEMENT(he), p)
		}
	case HANDLE_DATA_ARRIVED:
		if handler.OnDataArrived != nil {
			p := (*DataArrivedParams)(params)
			handled = handler.OnDataArrived(HELEMENT(he), p)
		}
	case HANDLE_SIZE:
		if handler.OnSize != nil {
			handler.OnSize(HELEMENT(he))
		}
	case HANDLE_SCROLL:
		if handler.OnScroll != nil {
			p := (*ScrollParams)(params)
			handled = handler.OnScroll(HELEMENT(he), p)
		}
	case HANDLE_EXCHANGE:
		if handler.OnExchange != nil {
			p := (*ExchangeParams)(params)
			handled = handler.OnExchange(HELEMENT(he), p)
		}
	case HANDLE_GESTURE:
		if handler.OnGesture != nil {
			p := (*GestureParams)(params)
			handled = handler.OnGesture(HELEMENT(he), p)
		}
	default:
		// Unknown event type, just ignore
	}

	if handled {
		return uintptr(TRUE)
	}
	return uintptr(FALSE)
})

var goNotifyProc = syscall.NewCallback(func(msg uint32, wparam uintptr, lparam uintptr, vparam uintptr) uintptr {
	if lparam == 0 {
		return 0
	}
	handler, exists := notifyHandlers[vparam]
	if !exists || handler == nil {
		return 0
	}
	phdr := (*NMHDR)(unsafe.Pointer(lparam))
	if phdr == nil {
		return 0
	}

	switch phdr.Code {
	case HLN_CREATE_CONTROL:
		if handler.OnCreateControl != nil {
			return handler.OnCreateControl((*NmhlCreateControl)(unsafe.Pointer(lparam)))
		}
	case HLN_CONTROL_CREATED:
		if handler.OnControlCreated != nil {
			return handler.OnControlCreated((*NmhlCreateControl)(unsafe.Pointer(lparam)))
		}
	case HLN_DESTROY_CONTROL:
		if handler.OnDestroyControl != nil {
			return handler.OnDestroyControl((*NmhlDestroyControl)(unsafe.Pointer(lparam)))
		}
	case HLN_LOAD_DATA:
		if handler.OnLoadData != nil {
			return handler.OnLoadData((*NmhlLoadData)(unsafe.Pointer(lparam)))
		}
	case HLN_DATA_LOADED:
		if handler.OnDataLoaded != nil {
			return handler.OnDataLoaded((*NmhlDataLoaded)(unsafe.Pointer(lparam)))
		}
	case HLN_DOCUMENT_COMPLETE:
		if handler.OnDocumentComplete != nil {
			return handler.OnDocumentComplete()
		}
	case HLN_ATTACH_BEHAVIOR:
		params := (*NmhlAttachBehavior)(unsafe.Pointer(lparam))
		key := cstringToString(params.BehaviorName)
		log.Printf("[HLN_ATTACH_BEHAVIOR] behaviorName=%s", key)
		if params.Element == BAD_HELEMENT {
			return 0
		}

		// 1. 先检查用户自定义 behaviors
		if handler.Behaviors != nil {
			if behavior, exists := handler.Behaviors[key]; exists {
				log.Printf("[HLN_ATTACH_BEHAVIOR] Found custom behavior: %s", key)
				if refCount, exists := behaviors[behavior]; exists {
					behaviors[behavior] = refCount + 1
				} else {
					behaviors[behavior] = 1
				}
				tag := cgoNewHandle(behavior)
				params.ElementProc = uintptr(goElementProc)
				params.ElementTag = uintptr(tag)
				params.ElementEvents = behavior.Subscription()
				return 1
			}
		}

		// 2. 检查内置 behaviors (tabs, popup, etc.)
		if behavior, exists := builtinBehaviors[key]; exists {
			log.Printf("[HLN_ATTACH_BEHAVIOR] Found builtin behavior: %s", key)
			if refCount, exists := behaviors[behavior]; exists {
				behaviors[behavior] = refCount + 1
			} else {
				behaviors[behavior] = 1
			}
			tag := cgoNewHandle(behavior)
			params.ElementProc = uintptr(goElementProc)
			params.ElementTag = uintptr(tag)
			params.ElementEvents = behavior.Subscription()
			return 1
		}

		log.Printf("[HLN_ATTACH_BEHAVIOR] Behavior not found: %s", key)
		return 0
	}
	return 0
})

var goSelectCallback = syscall.NewCallback(func(he unsafe.Pointer, param uintptr) uintptr {
	if he == nil || param == 0 {
		return 0
	}
	slicePtr := cgoHandle(param).Value().(*[]*Element)
	e := NewElementFromHandle(HELEMENT(he))
	*slicePtr = append(*slicePtr, e)
	return 0
})

var goElementComparator = syscall.NewCallback(func(he1 unsafe.Pointer, he2 unsafe.Pointer, arg uintptr) int {
	if he1 == nil || he2 == nil || arg == 0 {
		return 0
	}
	cmp := *(*func(*Element, *Element) int)(unsafe.Pointer(arg))
	return cmp(NewElementFromHandle(HELEMENT(he1)), NewElementFromHandle(HELEMENT(he2)))
})

// Main htmlayout wndproc
func ProcNoDefault(hwnd, msg uint32, wparam, lparam uintptr) (uintptr, bool) {
	var handled bool = false
	result := HTMLayoutProcND(uintptr(hwnd), msg, wparam, lparam, &handled)
	return uintptr(result), handled
}

// Load html contents into window
func LoadHtml(hwnd uint32, data []byte, baseUrl string) error {
	if len(data) > 0 {
		baseUrlPtr := stringToUtf16Ptr(baseUrl)
		if !HTMLayoutLoadHtmlEx(uintptr(hwnd), &data[0], uint32(len(data)), baseUrlPtr) {
			return errors.New("HTMLayoutLoadHtmlEx failed")
		}
	}
	return nil
}

// Load resource (file or url) into window
func LoadResource(hwnd uint32, uri string) error {
	uriPtr := stringToUtf16Ptr(uri)
	if !HTMLayoutLoadFile(uintptr(hwnd), uriPtr) {
		return errors.New("HTMLayoutLoadFile failed")
	}
	return nil
}

func SetOption(hwnd uint32, option uint, value uint) bool {
	return HTMLayoutSetOption(uintptr(hwnd), uint32(option), uint32(value))
}

const (
	HTMLAYOUT_SMOOTH_SCROLL      = 1
	HTMLAYOUT_CONNECTION_TIMEOUT = 2
	HTMLAYOUT_HTTPS_ERROR        = 3
	HTMLAYOUT_FONT_SMOOTHING     = 4
	HTMLAYOUT_ANIMATION_THREAD   = 5
	HTMLAYOUT_TRANSPARENT_WINDOW = 6
)

// Call this from your NotifyHandler.HandleLoadData method if you want htmlayout to
// process the data right away so you don't have to provide a buffer in the NmhlLoadData structure.
func DataReady(hwnd uint32, uri *uint16, data []byte) bool {
	return HTMLayoutDataReady(uintptr(hwnd), uri, &data[0], uint32(len(data)))
}

func AttachWindowEventHandler(hwnd uint32, handler *EventHandler) {
	key := uintptr(hwnd)

	if oldHandle, exists := windowEventHandles[hwnd]; exists {
		oldTag := uintptr(oldHandle)
		HTMLayoutWindowDetachEventHandler(key, uintptr(goElementProc), oldTag)
		delete(windowEventHandlers, hwnd)
		delete(windowEventHandles, hwnd)
		oldHandle.Delete()
		log.Printf("[AttachWindowEventHandler] hwnd=%d old handler detached", hwnd)
	}

	handle := cgoNewHandle(handler)
	tag := uintptr(handle)

	windowEventHandlers[hwnd] = handler
	windowEventHandles[hwnd] = handle

	subscription := handler.Subscription()
	subscription &= ^uint32(DISABLE_INITIALIZATION & 0xffffffff)

	ret := HTMLayoutWindowAttachEventHandler(key, uintptr(goElementProc), tag, subscription)
	if ret != HLDOM_OK {
		domPanic(ret, "Failed to attach event handler to window")
	}
	log.Printf("[AttachWindowEventHandler] hwnd=%d attached successfully, handle=%d", hwnd, handle)
}

func DetachWindowEventHandler(hwnd uint32) {
	key := uintptr(hwnd)
	handle, exists := windowEventHandles[hwnd]
	if !exists {
		log.Printf("[DetachWindowEventHandler] hwnd=%d not found in windowEventHandles, map has %d entries", hwnd, len(windowEventHandles))
		return
	}
	tag := uintptr(handle)
	delete(windowEventHandlers, hwnd)
	delete(windowEventHandles, hwnd)
	HTMLayoutWindowDetachEventHandler(key, uintptr(goElementProc), tag)
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[DetachWindowEventHandler] handle already deleted: %v", r)
			}
		}()
		handle.Delete()
		log.Printf("[DetachWindowEventHandler] hwnd=%d detached successfully", hwnd)
	}()
}

func AttachNotifyHandler(hwnd uint32, handler *NotifyHandler) {
	key := uintptr(hwnd)
	// Overwrite if it exists
	notifyHandlers[key] = handler
	HTMLayoutSetCallback(key, uintptr(goNotifyProc), key)
}

func DetachNotifyHandler(hwnd uint32) {
	key := uintptr(hwnd)
	if _, exists := notifyHandlers[key]; exists {
		HTMLayoutSetCallback(key, 0, 0)
		delete(notifyHandlers, key)
	}
}

func DumpObjectCounts() {
	log.Print("Window notify handlers (", len(notifyHandlers), "): ", notifyHandlers)
	log.Print("Window event handlers (", len(windowEventHandlers), "): ", windowEventHandlers)
	log.Print("Element event handlers (", len(eventHandlers), "): ", eventHandlers)
}
