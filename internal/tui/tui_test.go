// Copyright 2026 Baseten Labs, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ahmetb/kubectl-drill/internal/analysis"
)

func testSet() analysis.Set {
	mk := func(name string, labels ...string) analysis.Resource {
		m := make(map[string]string)
		for i := 0; i < len(labels); i += 2 {
			m[labels[i]] = labels[i+1]
		}
		return analysis.Resource{Kind: "Node", Name: name, Labels: m}
	}
	return analysis.Set{Resources: []analysis.Resource{
		mk("node-0", "kubernetes.io/hostname", "node-0", "kubernetes.io/os", "linux", "example.com/pool", "cpu", "uniform.io/x", "1"),
		mk("node-1", "kubernetes.io/hostname", "node-1", "kubernetes.io/os", "linux", "example.com/pool", "cpu", "uniform.io/x", "1"),
		mk("node-2", "kubernetes.io/hostname", "node-2", "kubernetes.io/os", "linux", "example.com/pool", "arm", "uniform.io/x", "1"),
	}}
}

func TestAnnotationsMode(t *testing.T) {
	mk := func(name, cc string) analysis.Resource {
		return analysis.Resource{Kind: "Node", Name: name, Annotations: map[string]string{
			"example.com/owner":       "platform",
			"example.com/cost-center": cc,
		}}
	}
	set := analysis.Set{Field: analysis.FieldAnnotations, Resources: []analysis.Resource{
		mk("node-0", "cc-0"),
		mk("node-1", "cc-1"),
	}}
	m := newModel(set)
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	v := m.View()
	for _, want := range []string{"annotation keys", "example.com/", "cost-center"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q\n%s", want, v)
		}
	}

	// drill down: values and resources come from the annotation maps
	m = update(m, special(tea.KeyRight), special(tea.KeyRight))
	if got := m.selector(); got != "" {
		t.Errorf("annotations have no -l selector, got %q", got)
	}
	m = update(m, special(tea.KeyRight))
	if n := len(m.cols[3].Items()); n != 1 {
		t.Errorf("resources with cost-center=cc-0 = %d, want 1", n)
	}

	// copy is a labels-only feature
	m = update(m, runeKey("c"))
	if !strings.Contains(m.status, "labels only") {
		t.Errorf("status = %q, want the labels-only notice", m.status)
	}
}

func runeKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func special(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func click(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

func wheel(button tea.MouseButton) tea.MouseMsg {
	return tea.MouseMsg{Action: tea.MouseActionPress, Button: button, X: 10, Y: 10}
}

func update(m model, msgs ...tea.Msg) model {
	for _, msg := range msgs {
		nm, _ := m.Update(msg)
		m = nm.(model)
	}
	return m
}

func TestInitialView(t *testing.T) {
	m := newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	v := m.View()
	for _, want := range []string{"PREFIXES", "KEYS", "VALUES", "RESOURCES", "3 resources", "4 label keys", "kubernetes.io/", "example.com/"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q\n%s", want, v)
		}
	}
}

func TestDrillDownAndSelector(t *testing.T) {
	m := newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// focus starts on prefixes; drill right into keys
	m = update(m, special(tea.KeyRight))
	if m.focus != 1 {
		t.Fatalf("focus = %d, want 1", m.focus)
	}
	// first prefix alphabetically is example.com; its key column should list pool
	if got := m.selector(); got != "example.com/pool" {
		t.Errorf("selector after key selection = %q, want example.com/pool", got)
	}
	// drill into values
	m = update(m, special(tea.KeyRight))
	if got := m.selector(); got != "example.com/pool=cpu" {
		t.Errorf("selector after value selection = %q, want example.com/pool=cpu", got)
	}
	// drill into resources: 2 nodes have pool=cpu
	m = update(m, special(tea.KeyRight))
	if n := len(m.cols[3].Items()); n != 2 {
		t.Errorf("resources column = %d items, want 2", n)
	}
	// move down to the second value (arm), resources should follow
	m = update(m, special(tea.KeyLeft))
	m = update(m, special(tea.KeyDown))
	if got := m.selector(); got != "example.com/pool=arm" {
		t.Errorf("selector = %q, want example.com/pool=arm", got)
	}
	if n := len(m.cols[3].Items()); n != 1 {
		t.Errorf("resources for arm = %d, want 1", n)
	}
}

func TestVaryToggle(t *testing.T) {
	m := newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	before := len(m.cols[0].Items())
	m = update(m, runeKey("v"))
	after := len(m.cols[0].Items())
	if after >= before {
		t.Errorf("vary should hide uniform-only prefixes: before=%d after=%d", before, after)
	}
	if !strings.Contains(m.View(), "distinctive only") {
		t.Error("vary indicator missing from view")
	}
	m = update(m, runeKey("v"))
	if len(m.cols[0].Items()) != before {
		t.Error("vary toggle should restore all prefixes")
	}
}

func TestCopySelector(t *testing.T) {
	m := newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, runeKey("c"))
	if !strings.Contains(m.status, "select a key first") {
		t.Errorf("status = %q", m.status)
	}
	m = update(m, special(tea.KeyRight))
	m = update(m, runeKey("c"))
	// clipboard may be unavailable in headless environments; either way the
	// status must mention the selector
	if !strings.Contains(m.status, "example.com/pool") {
		t.Errorf("status = %q, want it to mention the selector", m.status)
	}
}

func TestMissingRow(t *testing.T) {
	set := testSet()
	delete(set.Resources[2].Labels, "example.com/pool")
	m := newModel(set)
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, special(tea.KeyRight), special(tea.KeyRight))
	items := m.cols[2].Items()
	last := items[len(items)-1].(item)
	if !last.missing {
		t.Fatalf("last value row should be the (missing) pseudo-row: %+v", last)
	}
	if got := m.selector(); got != "example.com/pool=cpu" {
		t.Errorf("selector = %q", got)
	}
	// select the missing row -> !key selector, and resources lacking the key
	m = update(m, special(tea.KeyDown))
	if got := m.selector(); got != "!example.com/pool" {
		t.Errorf("missing selector = %q, want !example.com/pool", got)
	}
	m = update(m, special(tea.KeyRight))
	if n := len(m.cols[3].Items()); n != 1 {
		t.Errorf("missing resources = %d, want 1", n)
	}
}

func TestEscNavigatesBack(t *testing.T) {
	m := newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, special(tea.KeyRight), special(tea.KeyRight), special(tea.KeyRight))
	if m.focus != 3 {
		t.Fatalf("focus = %d, want 3", m.focus)
	}
	// esc and backspace step left like the arrow keys do
	m = update(m, special(tea.KeyEsc))
	if m.focus != 2 {
		t.Errorf("focus after esc = %d, want 2", m.focus)
	}
	m = update(m, special(tea.KeyBackspace))
	if m.focus != 1 {
		t.Errorf("focus after backspace = %d, want 1", m.focus)
	}
	// esc on the first column is a no-op, not a quit
	m = update(m, special(tea.KeyEsc), special(tea.KeyEsc))
	if m.focus != 0 {
		t.Errorf("focus = %d, want 0", m.focus)
	}
}

func TestColumnHeadersShowCounts(t *testing.T) {
	m := newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	v := m.View()
	for _, want := range []string{"PREFIXES (3)", "KEYS (1)", "VALUES (2)"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q\n%s", want, v)
		}
	}
}

func TestQuit(t *testing.T) {
	m := newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	_, cmd := m.Update(runeKey("q"))
	if cmd == nil {
		t.Fatal("q should produce a quit command")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("expected tea.Quit, got %v", msg)
	}
}

// geometry at 120x40: colW=29, pitch=30; column i spans x in [30i, 30i+29],
// its magnifier icon sits at 30i+27, its clear icon at 30i+25; rows start at y=2.

func TestClickRowSelectsAndDrills(t *testing.T) {
	m := newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// click the first key row: focus moves and selection follows
	m = update(m, click(35, 2))
	if m.focus != 1 {
		t.Fatalf("focus after click = %d, want 1", m.focus)
	}
	if got := m.selector(); got != "example.com/pool" {
		t.Errorf("selector after click = %q, want example.com/pool", got)
	}

	// click the second value row (arm) straight from the prefixes view
	m = update(m, click(65, 3))
	if m.focus != 2 {
		t.Fatalf("focus after click = %d, want 2", m.focus)
	}
	if got := m.selector(); got != "example.com/pool=arm" {
		t.Errorf("selector = %q, want example.com/pool=arm", got)
	}
	if n := len(m.cols[3].Items()); n != 1 {
		t.Errorf("resources after clicks = %d, want 1", n)
	}

	// clicking the pagination/padding area below short lists is a no-op
	before := m.cols[3].Index()
	m = update(m, click(95, 33))
	if m.cols[3].Index() != before {
		t.Errorf("click on padding changed index: %d -> %d", before, m.cols[3].Index())
	}
}

func TestClickTitleFocusesColumn(t *testing.T) {
	m := newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, click(45, 1)) // somewhere in the KEYS title, not on an icon
	if m.focus != 1 {
		t.Fatalf("focus = %d, want 1", m.focus)
	}
	if m.cols[1].FilterState() != list.Unfiltered {
		t.Errorf("title click should not open the filter, state = %v", m.cols[1].FilterState())
	}
}

func TestClickMagnifierStartsFilter(t *testing.T) {
	m := newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, click(57, 1)) // magnifier of the KEYS column
	if m.focus != 1 {
		t.Fatalf("focus = %d, want 1", m.focus)
	}
	if m.cols[1].FilterState() != list.Filtering {
		t.Fatalf("magnifier click should start filtering, state = %v", m.cols[1].FilterState())
	}
	// typed text is visible while filtering
	m = update(m, runeKey("oo"))
	if !strings.Contains(m.View(), "oo") {
		t.Error("filter text not visible while typing")
	}
	m = update(m, special(tea.KeyEnter))
	if m.cols[1].FilterState() != list.FilterApplied {
		t.Fatalf("enter should apply the filter, state = %v", m.cols[1].FilterState())
	}
	if n := len(m.cols[1].VisibleItems()); n != 1 {
		t.Errorf("filtered keys = %d, want 1", n)
	}
}

func applyFilter(m model, col int, text string) model {
	m = update(m, runeKey("/"), runeKey(text), special(tea.KeyEnter))
	return m
}

func TestFilterDiscard(t *testing.T) {
	m := newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = applyFilter(m, 0, "kube")
	if n := len(m.cols[0].VisibleItems()); n != 1 {
		t.Fatalf("filtered prefixes = %d, want 1", n)
	}
	if !strings.Contains(m.View(), icFilter) {
		t.Error("applied filter chip missing from view")
	}

	// x clears the focused column's filter
	m = update(m, runeKey("x"))
	if m.cols[0].FilterState() != list.Unfiltered {
		t.Error("x should clear the focused column's filter")
	}
	if n := len(m.cols[0].VisibleItems()); n != 3 {
		t.Errorf("prefixes after clear = %d, want 3", n)
	}
	if !strings.Contains(m.View(), "filter cleared") {
		t.Error("clear status missing from view")
	}

	// esc clears the filter before navigating back
	m = applyFilter(m, 0, "kube")
	m = update(m, special(tea.KeyEsc))
	if m.cols[0].FilterState() != list.Unfiltered {
		t.Error("esc should clear the focused column's filter")
	}
	if m.focus != 0 {
		t.Errorf("esc should stay put after clearing a filter, focus = %d", m.focus)
	}

	// clicking the clear chip icon discards the filter
	m = applyFilter(m, 0, "kube")
	m = update(m, click(25, 1))
	if m.cols[0].FilterState() != list.Unfiltered {
		t.Error("clear icon click should discard the filter")
	}
}

func TestClearAllFilters(t *testing.T) {
	m := newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = applyFilter(m, 0, "kube")
	m = update(m, special(tea.KeyRight)) // focus keys
	// prefixes are filtered to kubernetes.io, so keys are hostname and os
	m = applyFilter(m, 1, "os")
	if m.cols[0].FilterState() != list.FilterApplied || m.cols[1].FilterState() != list.FilterApplied {
		t.Fatal("expected filters on prefix and key columns")
	}
	m = update(m, runeKey("X"))
	for i := range m.cols {
		if m.cols[i].FilterState() != list.Unfiltered {
			t.Errorf("column %d still filtered after X", i)
		}
	}
}

func TestFilterSurvivesRebuild(t *testing.T) {
	// toggling vary rebuilds the prefixes column; an applied filter must
	// survive the rebuild instead of blanking the column. example.com stays
	// visible in distinctive mode (its key varies), kubernetes.io does not.
	m := newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = applyFilter(m, 0, "example")
	m = update(m, runeKey("v"))
	if n := len(m.cols[0].VisibleItems()); n != 1 {
		t.Errorf("filtered prefixes after vary rebuild = %d, want 1", n)
	}
}

func TestWheelScrollsHoveredColumn(t *testing.T) {
	m := newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, wheel(tea.MouseButtonWheelDown))
	if m.cols[0].Index() != 1 {
		t.Fatalf("index after wheel down = %d, want 1", m.cols[0].Index())
	}
	if m.focus != 0 {
		t.Errorf("wheel should not move focus, focus = %d", m.focus)
	}
	// selection change rebuilds downstream columns
	if got := m.selector(); got != "" { // focus is still on prefixes: no selector
		t.Errorf("selector = %q, want empty", got)
	}
	m = update(m, special(tea.KeyRight))
	if got := m.selector(); got != "kubernetes.io/hostname" {
		t.Errorf("selector after wheel + drill = %q, want kubernetes.io/hostname", got)
	}
	m = update(m, wheel(tea.MouseButtonWheelUp))
	if m.cols[0].Index() != 0 {
		t.Errorf("index after wheel up = %d, want 0", m.cols[0].Index())
	}
}

func TestClickMathMatchesRenderedRows(t *testing.T) {
	// regression: clicks must land on the row that is actually rendered at
	// that screen position. Locate the row's coordinates in View output and
	// click there, rather than assuming the layout math.
	m := newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	v := m.View()

	find := func(s string) (int, int) {
		for y, ln := range strings.Split(v, "\n") {
			if x := strings.Index(ln, s); x >= 0 {
				return x, y
			}
		}
		t.Fatalf("row %q not found in view:\n%s", s, v)
		return 0, 0
	}

	x, y := find("kubernetes.io/")
	m = update(m, click(x, y))
	sel, ok := m.selected(0)
	if !ok || sel.payload != "kubernetes.io" {
		t.Errorf("click on rendered kubernetes.io/ row selected %+v", sel)
	}
	// keys column should now list kubernetes.io keys
	m = update(m, special(tea.KeyRight))
	if got := m.selector(); got != "kubernetes.io/hostname" {
		t.Errorf("selector = %q, want kubernetes.io/hostname", got)
	}

	// same check for a value row, with fresh drill state
	m = newModel(testSet())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, special(tea.KeyRight)) // keys: example.com/pool
	x, y = find("arm")
	m = update(m, click(x, y))
	if got := m.selector(); got != "example.com/pool=arm" {
		t.Errorf("click on rendered arm row gave selector %q, want example.com/pool=arm", got)
	}
}
