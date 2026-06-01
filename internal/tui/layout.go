// Package tui implements the hv terminal user interface.
package tui

// Layout describes which responsive variant to render.
//
// Phase 8 redesign (8a): the old three-column Wide/Medium model is replaced by
// a two-column slide model with two depth states — Browse and Drill — both
// active at ≥80 columns. Below 80 columns the Narrow stacked single-pane
// ladder is unchanged.
type Layout int

const (
	// LayoutBrowse renders two panes: Sessions(left, ~36ch) │ Events(right).
	// Active at ≥80 terminal columns. Replaces the old LayoutWide + LayoutMedium.
	LayoutBrowse Layout = iota

	// LayoutDrill is used internally when depthDrill==true in LayoutBrowse mode.
	// In Drill the rendering is Events(left) │ Inspector(right, ~46ch).
	// It is NOT returned by chooseLayout — it is set on the model as a depth flag
	// (model.depthDrill) while layout remains LayoutBrowse. Kept here so that
	// layout-aware code can reference the constant if needed; view.go switches on
	// model.depthDrill directly.
	LayoutDrill

	// LayoutNarrow renders a single pane with a breadcrumb header.
	// Used below 80 terminal columns.
	LayoutNarrow

	// LayoutTooSmall triggers a "please resize" message.
	// Used below ~40 columns or ~10 rows.
	LayoutTooSmall

	// Retained for backward-compatibility with tests that reference these names.
	// Both map to LayoutBrowse behavior in the new model.
	LayoutWide   = LayoutBrowse
	LayoutMedium = LayoutBrowse
)

const (
	minCols    = 40
	minRows    = 10
	mediumCols = 80 // ≥80 → two-column; below → stacked
)

// chooseLayout returns the Layout variant for the given terminal dimensions.
// The returned value is always one of LayoutBrowse, LayoutNarrow, or LayoutTooSmall.
// The Browse ↔ Drill depth distinction is tracked separately via model.depthDrill.
func chooseLayout(cols, rows int) Layout {
	if cols < minCols || rows < minRows {
		return LayoutTooSmall
	}
	if cols >= mediumCols {
		return LayoutBrowse
	}
	return LayoutNarrow
}

// paneWidths returns (sessions, events, inspector) column widths for a Layout.
//
// Browse mode (depthDrill==false): sessions=36, events=totalCols−36−1 (divider).
// Drill mode (depthDrill==true):   events=totalCols−46−1, inspector=46.
// Narrow: all three return totalCols (caller uses one pane at a time).
func paneWidths(layout Layout, totalCols int) (sessions, events, inspector int) {
	const (
		divider    = 1 // single │ between the two panes
		sessionsW  = 36
		inspectorW = 46
	)
	switch layout {
	case LayoutBrowse:
		// Browse: Sessions │ Events
		s := sessionsW
		e := totalCols - s - divider
		if e < 10 {
			e = 10
		}
		return s, e, inspectorW
	default:
		return totalCols, totalCols, totalCols
	}
}

// paneWidthsDrill returns (events, inspector) for Drill mode.
// Events takes the left slot; Inspector takes the right fixed slot.
func paneWidthsDrill(totalCols int) (events, inspector int) {
	const (
		divider    = 1
		inspectorW = 46
	)
	e := totalCols - inspectorW - divider
	if e < 10 {
		e = 10
	}
	return e, inspectorW
}
