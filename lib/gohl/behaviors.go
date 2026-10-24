package gohl

import (
	"log"
)

var (
	builtinBehaviors = make(map[string]*EventHandler)
)

func init() {
	builtinBehaviors["tabs"] = TabsBehavior()
	builtinBehaviors["light-box-dialog"] = LightBoxDialogBehavior()
	builtinBehaviors["hyperlink"] = HyperlinkBehavior()
}

func TabsBehavior() *EventHandler {
	return &EventHandler{
		OnAttached: func(he HELEMENT) {
			elem := NewElementFromHandle(he)
			if elem != nil {
				initTabs(elem)
			}
		},
		OnMouse: func(he HELEMENT, params *MouseParams) bool {
			cmd := params.Cmd & 0xFF
			if cmd != MOUSE_DOWN && cmd != MOUSE_DCLICK {
				return false
			}

			tabsEl := NewElementFromHandle(he)
			if tabsEl == nil {
				return false
			}

			var targetEl *Element
			if params.Target != BAD_HELEMENT {
				targetEl = NewElementFromHandle(params.Target)
			}

			tabEl := findTargetTab(targetEl, tabsEl)
			if tabEl == nil {
				return false
			}

			return selectTab(tabsEl, tabEl)
		},
		OnKey: func(he HELEMENT, params *KeyParams) bool {
			if params.Cmd != KEY_DOWN {
				return false
			}

			tabsEl := NewElementFromHandle(he)
			if tabsEl == nil {
				return false
			}

			currentTab := findCurrentTab(tabsEl)
			if currentTab == nil {
				return false
			}

			switch params.KeyCode {
			case 0x09:
				if params.AltState&CONTROL_KEY_PRESSED != 0 {
					dir := 1
					if params.AltState&SHIFT_KEY_PRESSED != 0 {
						dir = -1
					}
					return selectTabRelative(tabsEl, currentTab, dir)
				}
			case 0x25:
				return selectTabRelative(tabsEl, currentTab, -1)
			case 0x27:
				return selectTabRelative(tabsEl, currentTab, 1)
			case 0x24:
				return selectFirstTab(tabsEl)
			case 0x23:
				return selectLastTab(tabsEl)
			}
			return false
		},
		OnBehaviorEvent: func(he HELEMENT, params *BehaviorEventParams) bool {
			if params.Cmd == ACTIVATE_CHILD {
				tabsEl := NewElementFromHandle(he)
				if tabsEl == nil {
					return false
				}
				var targetEl *Element
				if params.Target != BAD_HELEMENT {
					targetEl = NewElementFromHandle(params.Target)
				}
				tabEl := findTargetTab(targetEl, tabsEl)
				if tabEl != nil {
					return selectTab(tabsEl, tabEl)
				}
			}
			return false
		},
	}
}

func initTabs(tabsEl *Element) {
	strip := tabsEl.SelectFirst(".strip")
	if strip == nil {
		log.Println("[tabs] strip not found")
		return
	}

	selectedTab := strip.SelectFirst("[panel][selected]")
	if selectedTab == nil {
		selectedTab = strip.SelectFirst("[panel]")
		if selectedTab == nil {
			log.Println("[tabs] no tabs found")
			return
		}
	}

	panelName, _ := selectedTab.Attr("panel")
	if panelName == "" {
		log.Println("[tabs] tab has no panel attribute")
		return
	}

	panel := tabsEl.SelectFirst("[name=\"" + panelName + "\"]")
	if panel == nil {
		log.Println("[tabs] panel not found:", panelName)
		return
	}

	strip.SetState(STATE_CURRENT, true)
	selectedTab.SetState(STATE_CURRENT, true)
	panel.SetState(STATE_EXPANDED, true)
}

func findTargetTab(target *Element, tabsEl *Element) *Element {
	if target == nil || tabsEl == nil {
		return nil
	}

	if target.Handle() == tabsEl.Handle() {
		return nil
	}

	if panel, exists := target.Attr("panel"); exists && panel != "" {
		return target
	}

	parent := target.Parent()
	if parent == nil {
		return nil
	}

	return findTargetTab(parent, tabsEl)
}

func findCurrentTab(tabsEl *Element) *Element {
	strip := tabsEl.SelectFirst(".strip")
	if strip == nil {
		return nil
	}
	return strip.SelectFirst("[panel]:current")
}

func selectTab(tabsEl *Element, tabEl *Element) bool {
	if tabsEl == nil || tabEl == nil {
		return false
	}

	panelName, exists := tabEl.Attr("panel")
	if !exists || panelName == "" {
		return false
	}

	panel := tabsEl.SelectFirst("[name=\"" + panelName + "\"]")
	if panel == nil {
		log.Println("[tabs] panel not found:", panelName)
		return false
	}

	strip := tabsEl.SelectFirst(".strip")
	if strip == nil {
		return false
	}

	oldTab := strip.SelectFirst("[panel]:current")
	if oldTab != nil {
		if oldTab.Handle() == tabEl.Handle() {
			return true
		}
		oldPanelName, _ := oldTab.Attr("panel")
		oldPanel := tabsEl.SelectFirst("[name=\"" + oldPanelName + "\"]")
		if oldPanel != nil {
			oldTab.SetState(STATE_CURRENT, false)
			oldTab.RemoveAttr("selected")
			oldPanel.SetState(STATE_EXPANDED, false)
			oldPanel.SetState(STATE_COLLAPSED, true)
		}
	}

	tabEl.SetState(STATE_CURRENT, true)
	tabEl.SetAttr("selected", "")
	panel.SetState(STATE_COLLAPSED, false)
	panel.SetState(STATE_EXPANDED, true)

	return true
}

func selectTabRelative(tabsEl *Element, currentTab *Element, direction int) bool {
	if tabsEl == nil || currentTab == nil {
		return false
	}

	strip := tabsEl.SelectFirst(".strip")
	if strip == nil {
		return false
	}

	tabs := strip.Select("[panel]")
	if len(tabs) == 0 {
		return false
	}

	currentIndex := -1
	for i, tab := range tabs {
		if tab.Handle() == currentTab.Handle() {
			currentIndex = i
			break
		}
	}

	if currentIndex < 0 {
		return false
	}

	newIndex := currentIndex + direction
	if newIndex < 0 {
		newIndex = len(tabs) - 1
	} else if newIndex >= len(tabs) {
		newIndex = 0
	}

	return selectTab(tabsEl, tabs[newIndex])
}

func selectFirstTab(tabsEl *Element) bool {
	strip := tabsEl.SelectFirst(".strip")
	if strip == nil {
		return false
	}
	firstTab := strip.SelectFirst("[panel]")
	if firstTab == nil {
		return false
	}
	return selectTab(tabsEl, firstTab)
}

func selectLastTab(tabsEl *Element) bool {
	strip := tabsEl.SelectFirst(".strip")
	if strip == nil {
		return false
	}
	tabs := strip.Select("[panel]")
	if len(tabs) == 0 {
		return false
	}
	return selectTab(tabsEl, tabs[len(tabs)-1])
}

type LightBoxDialog struct {
	SavedParent *Element
	SavedIndex  uint
	FocusUid    uint32
}

var dialogStates = make(map[HELEMENT]*LightBoxDialog)

func LightBoxDialogBehavior() *EventHandler {
	return &EventHandler{
		OnAttached: func(he HELEMENT) {
			dialogStates[he] = &LightBoxDialog{}
		},
		OnDetached: func(he HELEMENT) {
			delete(dialogStates, he)
		},
		OnKey: func(he HELEMENT, params *KeyParams) bool {
			if params.Cmd != KEY_DOWN {
				return false
			}

			dialog := NewElementFromHandle(he)
			if dialog == nil {
				return false
			}

			switch params.KeyCode {
			case 0x0D:
				defBtn := dialog.SelectFirst("[role='ok-button']")
				if defBtn != nil {
					defBtn.CallBehaviorMethod(DO_CLICK)
					return true
				}
			case 0x1B:
				cancelBtn := dialog.SelectFirst("[role='cancel-button']")
				if cancelBtn != nil {
					cancelBtn.CallBehaviorMethod(DO_CLICK)
					return true
				}
			}
			return false
		},
		OnBehaviorEvent: func(he HELEMENT, params *BehaviorEventParams) bool {
			if params.Cmd != BUTTON_CLICK {
				return false
			}

			if params.Target == BAD_HELEMENT {
				return false
			}

			button := NewElementFromHandle(params.Target)
			if button == nil {
				return false
			}

			role, _ := button.Attr("role")
			if role == "ok-button" || role == "cancel-button" {
				hideDialog(he)
				return false
			}
			return false
		},
	}
}

func ShowDialog(he HELEMENT) {
	state, exists := dialogStates[he]
	if !exists || state.SavedParent != nil {
		return
	}

	dialog := NewElementFromHandle(he)
	if dialog == nil {
		return
	}

	state.SavedParent = dialog.Parent()
	state.SavedIndex = dialog.Index()

	root := dialog.Root()
	if root == nil {
		return
	}

	focusEl := root.SelectFirst(":focus")
	if focusEl != nil {
		state.FocusUid = focusEl.GetElementUid()
	}

	shim := CreateElement("div", "")
	if shim == nil {
		return
	}
	shim.SetAttr("class", "shim")
	root.AppendChild(shim)

	shim.InsertChild(dialog, 0)
	dialog.SetStyle("display", "block")

	dialog.SetEventRoot()
}

func hideDialog(he HELEMENT) {
	state, exists := dialogStates[he]
	if !exists || state.SavedParent == nil {
		return
	}

	dialog := NewElementFromHandle(he)
	if dialog == nil {
		return
	}

	state.SavedParent.InsertChild(dialog, state.SavedIndex)

	root := dialog.Root()
	if root != nil {
		shim := root.SelectFirst("div.shim")
		if shim != nil {
			shim.Detach()
		}
	}

	dialog.RemoveStyle("display")
	dialog.ResetEventRoot()

	if state.FocusUid != 0 {
		focusEl := ElementByUid(dialog.RootHwnd(), state.FocusUid)
		if focusEl != nil {
			focusEl.SetState(STATE_FOCUS, true)
		}
	}

	state.SavedParent = nil
	state.SavedIndex = 0
	state.FocusUid = 0
}

func HideDialog(he HELEMENT) {
	hideDialog(he)
}

func HyperlinkBehavior() *EventHandler {
	return &EventHandler{
		OnMouse: func(he HELEMENT, params *MouseParams) bool {
			link := NewElementFromHandle(he)
			if link == nil {
				return false
			}

			cmd := params.Cmd & 0xFF
			switch cmd {
			case MOUSE_DOWN:
				if params.ButtonState&MAIN_MOUSE_BUTTON != 0 {
					link.SetState(STATE_CURRENT, true)
					return true
				}
			case MOUSE_UP:
				if link.State(STATE_CURRENT) {
					link.SetState(STATE_CURRENT, false)
					link.PostEvent(HYPERLINK_CLICK, link, BY_MOUSE_CLICK)
					return true
				}
			}
			return false
		},
		OnKey: func(he HELEMENT, params *KeyParams) bool {
			if params.Cmd != KEY_UP {
				return false
			}
			if params.KeyCode != ' ' && params.KeyCode != 0x0D {
				return false
			}

			link := NewElementFromHandle(he)
			if link == nil {
				return false
			}
			link.PostEvent(HYPERLINK_CLICK, link, BY_KEY_CLICK)
			return true
		},
		OnFocus: func(he HELEMENT, params *FocusParams) bool {
			return true
		},
	}
}

func GetBuiltinBehaviors() map[string]*EventHandler {
	return builtinBehaviors
}
