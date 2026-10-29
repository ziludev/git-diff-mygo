package shell

import "github.com/egoist/mygo/ui"

// iconName names one icon of lineIcons. The type keeps a typo a compile
// error instead of a blank glyph.
type iconName string

const (
	iconRefresh      iconName = "refresh"
	iconFetch        iconName = "fetch"
	iconPull         iconName = "pull"
	iconPush         iconName = "push"
	iconBranch       iconName = "branch"
	iconChevronDown  iconName = "chevron-down"
	iconChevronRight iconName = "chevron-right"
	iconMore         iconName = "more"
	iconPlus         iconName = "plus"
	iconMinus        iconName = "minus"
	iconUndo         iconName = "undo"
	iconTrash        iconName = "trash"
	iconCheck        iconName = "check"
	iconX            iconName = "x"
	iconArchive      iconName = "archive"
)

// lineIcons is the app's icon set: a small subset of Lucide, inlined as SVG
// documents so the app needs no icon dependency. Each icon draws in the text
// color of its container (ui.Icon), which keeps it in step with the theme and
// hover states. Parsing happens once, at startup.
var lineIcons = map[iconName]*ui.SVG{
	iconRefresh:      ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/></svg>`)),
	iconFetch:        ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4.4 15.3A7 7 0 1 1 15.7 8h1.8a4.5 4.5 0 0 1 2.4 8.3"/><path d="M12 12v9"/><path d="m8.5 17.5 3.5 3.5 3.5-3.5"/></svg>`)),
	iconPull:         ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M5 21h14"/></svg>`)),
	iconPush:         ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 21V9"/><path d="m7 14 5-5 5 5"/><path d="M5 3h14"/></svg>`)),
	iconBranch:       ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 3v12"/><circle cx="18" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><path d="M18 9a9 9 0 0 1-9 9"/></svg>`)),
	iconChevronDown:  ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>`)),
	iconChevronRight: ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m9 18 6-6-6-6"/></svg>`)),
	iconMore:         ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h.01"/><path d="M12 12h.01"/><path d="M19 12h.01"/></svg>`)),
	iconPlus:         ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 5v14"/><path d="M5 12h14"/></svg>`)),
	iconMinus:        ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/></svg>`)),
	iconUndo:         ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 14 4 9l5-5"/><path d="M4 9h10.5a5.5 5.5 0 0 1 0 11H11"/></svg>`)),
	iconTrash:        ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18"/><path d="M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/></svg>`)),
	iconCheck:        ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`)),
	iconX:            ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>`)),
	iconArchive:      ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="20" height="5" x="2" y="3" rx="1"/><path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8"/><path d="M10 12h4"/></svg>`)),
}

// icon shows one icon. Every constant above has an entry; a name without one
// would draw nothing.
func icon(c *ui.Context, name iconName) ui.Element {
	return ui.Icon(c, lineIcons[name])
}

// iconButton is a borderless button showing one line icon. label is the
// control's short name, for assistive technology and tests; tip explains it
// in the tooltip. A nil onClick leaves the click to the caller, which is how
// the more menu turns the same control into a menu button.
func iconButton(c *ui.Context, name iconName, label, tip string, disabled bool, onClick func()) ui.Element {
	t := c.Theme()
	btn := ui.ButtonBase(c).Size(22, 22).Radius(4).Center().
		Cursor(ui.CursorPointer).Disabled(disabled)
	btn.Children(func() {
		icon(c, name).FontSize(13).TextColor(t.TextMuted)
	})
	btn.Tooltip(tip)
	btn.Label(label)
	btn.OnClick(onClick)
	return btn
}
