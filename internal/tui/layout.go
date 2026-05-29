// Package tui implements the cchv terminal user interface.
package tui

// Layout describes which responsive variant to render.
type Layout int

const (
	// LayoutWide renders three panes side-by-side: Sessions | Events | Inspector.
	// Requires ≥ 120 terminal columns.
	LayoutWide Layout = iota
	// LayoutMedium renders two panes with an Inspector drawer.
	// Used between 80 and 119 terminal columns.
	LayoutMedium
	// LayoutNarrow renders a single pane with a breadcrumb header.
	// Used below 80 terminal columns.
	LayoutNarrow
	// LayoutTooSmall triggers a "please resize" message.
	// Used below ~40 columns or ~10 rows.
	LayoutTooSmall
)

const (
	minCols = 40
	minRows = 10
	wideCols   = 120
	mediumCols = 80
)

// chooseLayout returns the Layout variant for the given terminal dimensions.
func chooseLayout(cols, rows int) Layout {
	if cols < minCols || rows < minRows {
		return LayoutTooSmall
	}
	if cols >= wideCols {
		return LayoutWide
	}
	if cols >= mediumCols {
		return LayoutMedium
	}
	return LayoutNarrow
}

// paneWidths returns (sessions, events, inspector) column widths for a Layout.
// The values are advisory; View() clips as needed. For LayoutNarrow and
// LayoutTooSmall, only the first value is meaningful (full terminal width).
func paneWidths(layout Layout, totalCols int) (sessions, events, inspector int) {
	const (
		dividers   = 2 // two │ chars in wide, one in medium
		sessionsW  = 22
		inspectorW = 30
	)
	switch layout {
	case LayoutWide:
		s := sessionsW
		i := inspectorW
		e := totalCols - s - i - dividers
		if e < 10 {
			e = 10
		}
		return s, e, i
	case LayoutMedium:
		s := sessionsW
		e := totalCols - s - 1
		if e < 10 {
			e = 10
		}
		return s, e, inspectorW
	default:
		return totalCols, totalCols, totalCols
	}
}
