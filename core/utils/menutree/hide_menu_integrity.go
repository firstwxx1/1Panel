package menutree

import (
	"slices"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/global"
)

const (
	xpackMenuID    = "11"
	xpackMenuLabel = "Xpack-Menu"
	xpackMenuTitle = "pro.menu"
)

type retiredMenuIdentity struct {
	id    string
	label string
	paths []string
}

type proMenuSpec struct {
	canonical    dto.ShowMenu
	legacyLabels []string
	legacyPaths  []string
}

func PreserveMissingMenus(menus, fallback []dto.ShowMenu) ([]dto.ShowMenu, bool) {
	updated := cloneMenus(menus)
	changed := preserveMissingMenus(&updated, &updated, fallback)
	return updated, changed
}

func preserveMissingMenus(root, current *[]dto.ShowMenu, fallback []dto.ShowMenu) bool {
	changed := false
	for _, previous := range fallback {
		menuIndex := findMatchingMenu(*current, previous)
		if menuIndex < 0 {
			if containsMenu(*root, previous) {
				continue
			}
			*current = append(*current, cloneMenu(previous))
			changed = true
			continue
		}
		if preserveMissingMenus(root, &(*current)[menuIndex].Children, previous.Children) {
			changed = true
		}
	}
	return changed
}

func ReconcileHideMenuIntegrity(menus, fallback []dto.ShowMenu) ([]dto.ShowMenu, bool) {
	updated := cloneMenus(menus)
	sanitizedFallback := cloneMenus(fallback)
	changed := false

	for _, retired := range retiredMenuIdentities() {
		var removed bool
		updated, removed = removeRetiredMenus(updated, retired)
		changed = changed || removed
		sanitizedFallback, _ = removeRetiredMenus(sanitizedFallback, retired)
	}

	if xpackMenu := findMenuBy(updated, matchesXpackMenu); xpackMenu == nil {
		parent := defaultXpackMenu()
		if fbParent := findMenuBy(sanitizedFallback, matchesXpackMenu); fbParent != nil {
			parent.IsShow = fbParent.IsShow
			parent.Sort = fbParent.Sort
		}
		updated = append(updated, parent)
		changed = true
	} else if normalizeXpackParent(xpackMenu) {
		changed = true
	}

	for _, spec := range canonicalProMenuSpecs() {
		var itemChanged bool
		updated, itemChanged = ensureProMenu(updated, spec)
		changed = changed || itemChanged
	}

	return updated, changed
}

func defaultXpackMenu() dto.ShowMenu {
	return dto.ShowMenu{
		ID:       xpackMenuID,
		Disabled: false,
		Title:    xpackMenuTitle,
		IsShow:   true,
		Label:    xpackMenuLabel,
		Sort:     1100,
	}
}

func retiredMenuIdentities() []retiredMenuIdentity {
	identities := []retiredMenuIdentity{
		{id: "119", label: "Upage", paths: []string{"/xpack/upage"}},
		{id: "118", label: "XApp", paths: []string{"/xpack/app"}},
		{id: "123", label: "VirtualMachine", paths: []string{"/xpack/vm", "/enterprise/vm"}},
		{id: "120", label: "Cluster", paths: []string{"/xpack/cluster"}},
	}
	if !global.CONF.Base.IsEnterprise {
		identities = append(identities,
			retiredMenuIdentity{id: "121", label: "UserManagement", paths: []string{"/enterprise/users"}},
			retiredMenuIdentity{id: "122", label: "OpsReport", paths: []string{"/enterprise/ops-report"}},
		)
	}
	return identities
}

func canonicalProMenuSpecs() []proMenuSpec {
	return []proMenuSpec{
		{
			canonical:    dto.ShowMenu{ID: "112", Disabled: false, Title: "pro.waf.title", IsShow: true, Label: "ProWaf", Path: "/xpack/waf", Sort: 100},
			legacyLabels: []string{"Dashboard"},
			legacyPaths:  []string{"/xpack/waf/dashboard"},
		},
		{
			canonical:    dto.ShowMenu{ID: "113", Disabled: false, Title: "pro.monitor.title", IsShow: true, Label: "ProMonitor", Path: "/xpack/monitor", Sort: 200},
			legacyLabels: []string{"MonitorDashboard"},
			legacyPaths:  []string{"/xpack/monitor/dashboard"},
		},
		{
			canonical:    dto.ShowMenu{ID: "115", Disabled: false, Title: "pro.sync.title", IsShow: true, Label: "ProSync", Path: "/xpack/sync", Sort: 300},
			legacyLabels: []string{"Sync"},
			legacyPaths:  []string{"/xpack/sync"},
		},
		{
			canonical:    dto.ShowMenu{ID: "114", Disabled: false, Title: "pro.tamper.title", IsShow: true, Label: "ProTamper", Path: "/xpack/tamper", Sort: 400},
			legacyLabels: []string{"Tamper"},
			legacyPaths:  []string{"/xpack/tamper"},
		},
		{
			canonical:    dto.ShowMenu{ID: "116", Disabled: false, Title: "pro.alert.title", IsShow: true, Label: "ProAlert", Path: "/xpack/alert", Sort: 500},
			legacyLabels: []string{"AlertDashboard", "Alert"},
			legacyPaths:  []string{"/xpack/alert/dashboard"},
		},
		{
			canonical:    dto.ShowMenu{ID: "117", Disabled: false, Title: "pro.uisetting.title", IsShow: true, Label: "ProUiSetting", Path: "/xpack/uisetting", Sort: 600},
			legacyLabels: []string{"XSetting"},
			legacyPaths:  []string{"/xpack/setting"},
		},
		{
			canonical:    dto.ShowMenu{ID: "111", Disabled: false, Title: "pro.node.title", IsShow: true, Label: "NodeDashboard", Path: "/xpack/node", Sort: 700},
			legacyLabels: []string{"NodeDashboard", "Node"},
			legacyPaths:  []string{"/xpack/node/dashboard"},
		},
	}
}

func matchesXpackMenu(menu dto.ShowMenu) bool {
	return menu.ID == xpackMenuID || menu.Label == xpackMenuLabel
}

func normalizeXpackParent(menu *dto.ShowMenu) bool {
	changed := menu.ID != xpackMenuID ||
		menu.Label != xpackMenuLabel ||
		menu.Title != xpackMenuTitle ||
		menu.Disabled

	menu.ID = xpackMenuID
	menu.Label = xpackMenuLabel
	menu.Title = xpackMenuTitle
	menu.Disabled = false
	return changed
}

func findMatchingMenu(menus []dto.ShowMenu, target dto.ShowMenu) int {
	if target.ID != "" {
		for i := range menus {
			if menus[i].ID == target.ID {
				return i
			}
		}
	}
	if target.Label != "" {
		for i := range menus {
			if menus[i].Label == target.Label {
				return i
			}
		}
	}
	if target.Path != "" {
		for i := range menus {
			if menus[i].Path == target.Path {
				return i
			}
		}
	}
	return -1
}

func containsMenu(menus []dto.ShowMenu, target dto.ShowMenu) bool {
	if findMatchingMenu(menus, target) >= 0 {
		return true
	}
	for i := range menus {
		if containsMenu(menus[i].Children, target) {
			return true
		}
	}
	return false
}

func ensureProMenu(menus []dto.ShowMenu, spec proMenuSpec) ([]dto.ShowMenu, bool) {
	selected := findProMenu(menus, spec)
	if selected == nil {
		xpackMenu := findMenuBy(menus, matchesXpackMenu)
		if xpackMenu == nil {
			menus = append(menus, defaultXpackMenu())
			xpackMenu = &menus[len(menus)-1]
		}
		xpackMenu.Children = append(xpackMenu.Children, cloneMenu(spec.canonical))
		return menus, true
	}

	changed := normalizeSystemFields(selected, spec.canonical)
	deduplicated, deduplicatedChanged := deduplicateProMenu(menus, selected, spec)
	return deduplicated, changed || deduplicatedChanged
}

func findProMenu(menus []dto.ShowMenu, spec proMenuSpec) *dto.ShowMenu {
	matchers := []func(dto.ShowMenu) bool{
		func(menu dto.ShowMenu) bool { return spec.canonical.ID != "" && menu.ID == spec.canonical.ID },
		func(menu dto.ShowMenu) bool { return spec.canonical.Label != "" && menu.Label == spec.canonical.Label },
		func(menu dto.ShowMenu) bool { return menu.Label != "" && slices.Contains(spec.legacyLabels, menu.Label) },
		func(menu dto.ShowMenu) bool { return spec.canonical.Path != "" && menu.Path == spec.canonical.Path },
		func(menu dto.ShowMenu) bool { return menu.Path != "" && slices.Contains(spec.legacyPaths, menu.Path) },
	}
	for _, matches := range matchers {
		if menu := findMenuBy(menus, matches); menu != nil {
			return menu
		}
	}
	return nil
}

func findMenuBy(menus []dto.ShowMenu, matches func(dto.ShowMenu) bool) *dto.ShowMenu {
	for i := range menus {
		if matches(menus[i]) {
			return &menus[i]
		}
		if menu := findMenuBy(menus[i].Children, matches); menu != nil {
			return menu
		}
	}
	return nil
}

func deduplicateProMenu(menus []dto.ShowMenu, selected *dto.ShowMenu, spec proMenuSpec) ([]dto.ShowMenu, bool) {
	deduplicated := make([]dto.ShowMenu, 0, len(menus))
	changed := false
	for i := range menus {
		menu := &menus[i]
		if menu != selected && matchesProMenu(*menu, spec) {
			changed = true
			continue
		}
		children, childChanged := deduplicateProMenu(menu.Children, selected, spec)
		if childChanged {
			menu.Children = children
			changed = true
		}
		deduplicated = append(deduplicated, *menu)
	}
	return deduplicated, changed
}

func matchesProMenu(menu dto.ShowMenu, spec proMenuSpec) bool {
	if spec.canonical.ID != "" && menu.ID == spec.canonical.ID {
		return true
	}
	if spec.canonical.Label != "" && menu.Label == spec.canonical.Label {
		return true
	}
	if menu.Label != "" && slices.Contains(spec.legacyLabels, menu.Label) {
		return true
	}
	if spec.canonical.Path != "" && menu.Path == spec.canonical.Path {
		return true
	}
	if menu.Path != "" && slices.Contains(spec.legacyPaths, menu.Path) {
		return true
	}
	return false
}

func removeRetiredMenus(menus []dto.ShowMenu, identity retiredMenuIdentity) ([]dto.ShowMenu, bool) {
	if menus == nil {
		return nil, false
	}
	filtered := make([]dto.ShowMenu, 0, len(menus))
	changed := false
	for i := range menus {
		menu := &menus[i]
		children, childChanged := removeRetiredMenus(menu.Children, identity)
		if matchesRetiredIdentity(*menu, identity) {
			filtered = append(filtered, children...)
			changed = true
			continue
		}
		if childChanged {
			menu.Children = children
			changed = true
		}
		filtered = append(filtered, *menu)
	}
	return filtered, changed
}

func matchesRetiredIdentity(menu dto.ShowMenu, identity retiredMenuIdentity) bool {
	if identity.id != "" && menu.ID == identity.id {
		return true
	}
	if identity.label != "" && menu.Label == identity.label {
		return true
	}
	if menu.Path != "" && slices.Contains(identity.paths, menu.Path) {
		return true
	}
	return false
}

func normalizeSystemFields(menu *dto.ShowMenu, canonical dto.ShowMenu) bool {
	changed := menu.ID != canonical.ID ||
		menu.Label != canonical.Label ||
		menu.Disabled != canonical.Disabled ||
		menu.Title != canonical.Title ||
		menu.Path != canonical.Path

	menu.ID = canonical.ID
	menu.Label = canonical.Label
	menu.Disabled = canonical.Disabled
	menu.Title = canonical.Title
	menu.Path = canonical.Path
	return changed
}

func cloneMenus(menus []dto.ShowMenu) []dto.ShowMenu {
	if menus == nil {
		return nil
	}
	cloned := make([]dto.ShowMenu, len(menus))
	for i := range menus {
		cloned[i] = cloneMenu(menus[i])
	}
	return cloned
}

func cloneMenu(menu dto.ShowMenu) dto.ShowMenu {
	menu.Children = cloneMenus(menu.Children)
	return menu
}
