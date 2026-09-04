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

// Package tui implements the interactive metadata browser: a Miller-columns
// drill-down of prefix -> key -> value -> resources, over a set's labels
// or annotations.
package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ahmetb/kubectl-drill/internal/analysis"
)

const missingPayload = "\x00missing"

// Nerd Font glyphs (rendered by Hack Nerd Font and similar patch sets).
const (
	icPrefixes  = "" //  sitemap
	icKeys      = "" //  tag
	icValues    = "" //  list-ul
	icResources = "" //  server
	icSearch    = "" //  magnifier
	icClear     = "" //  times
	icFilter    = "" //  funnel
	icIdentity  = "" //  check
	icMissing   = "" //  ban
	icCopy      = "" //  copy
	icInfo      = "" //  info circle
	icBolt      = "" //  bolt (distinctive mode)
	icTarget    = "" //  crosshairs (selector)
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))
	focusedBorder = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, true, false, false).BorderForeground(lipgloss.Color("12"))
	plainBorder   = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, true, false, false).BorderForeground(lipgloss.Color("8"))
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("12")).Foreground(lipgloss.Color("0"))
	cursorStyle   = lipgloss.NewStyle().Background(lipgloss.Color("8"))
	missingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	filterStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // active filter chip
	searchStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	clearStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

// item is one row in a column.
type item struct {
	title   string
	desc    string
	payload string // prefix, full key, value, or resource index
	missing bool   // pseudo-row: resources lacking the selected key
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

// delegate renders compact single-line rows with a right-aligned desc.
type delegate struct {
	width   int
	focused bool
}

func (d delegate) Height() int  { return 1 }
func (d delegate) Spacing() int { return 0 }
func (d delegate) Update(tea.Msg, *list.Model) tea.Cmd {
	return nil
}

func (d delegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(item)
	if !ok {
		return
	}
	inner := d.width - 2
	desc := it.desc
	titleRunes := []rune(it.title)
	maxTitle := inner - len([]rune(desc)) - 1
	if maxTitle < 1 {
		maxTitle = 1
	}
	if len(titleRunes) > maxTitle {
		titleRunes = append(titleRunes[:maxTitle-1], '…')
	}
	gap := inner - len(titleRunes) - len([]rune(desc))
	if gap < 1 {
		gap = 1
	}
	line := " " + string(titleRunes) + strings.Repeat(" ", gap) + desc
	if it.missing {
		line = missingStyle.Render(line)
	}
	if index == m.Index() {
		if d.focused {
			line = selectedStyle.Render(line)
		} else {
			line = cursorStyle.Render(line)
		}
	}
	fmt.Fprint(w, line)
}

type model struct {
	set       analysis.Set
	vary      bool
	focus     int
	cols      [4]list.Model
	delegates [4]delegate
	width     int
	height    int
	status    string
}

var columnTitles = []string{
	icPrefixes + " PREFIXES",
	icKeys + " KEYS",
	icValues + " VALUES",
	icResources + " RESOURCES",
}

// Run starts the interactive browser.
func Run(set analysis.Set) error {
	m := newModel(set)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

func newModel(set analysis.Set) model {
	m := model{set: set, width: 100, height: 30}
	colW, _ := m.geometry()
	for i := range m.cols {
		l := list.New(nil, delegate{width: 24}, 24, 10)
		l.SetShowTitle(false)
		l.SetShowStatusBar(false)
		l.SetShowHelp(false)
		l.SetShowPagination(true)
		// the list's own filter bar would inject an empty line above the rows
		// (and a padded one while filtering); the column header renders the
		// filter input instead, so rows stay at a fixed screen offset.
		l.SetShowFilter(false)
		l.KeyMap.Quit = key.NewBinding(key.WithKeys("ctrl+c")) // free up q for the app
		l.FilterInput.Prompt = ""
		l.FilterInput.Width = colW - 4
		m.cols[i] = l
	}
	m.rebuildFrom(0)
	m.updateFocus()
	return m
}

func (m model) Init() tea.Cmd { return nil }

// geometry returns the per-column content width and the horizontal pitch
// (content + right border) of each column on screen. The mouse handler relies
// on View laying columns out at exactly this pitch.
func (m model) geometry() (colW, pitch int) {
	colW = (m.width - 4) / 4
	if colW < 16 {
		colW = 16
	}
	return colW, colW + 1
}

// colHeight is the height of each list body; View renders one header line
// above it (hence part of the -6 budget the footer occupies the rest).
func (m model) colHeight() int {
	colH := m.height - 6
	if colH < 3 {
		colH = 3
	}
	return colH
}

func (m *model) stats() []analysis.KeyStat {
	stats := m.set.KeyStats()
	analysis.SortKeyStats(stats, "")
	if m.vary {
		stats = analysis.Distinctive(stats, m.set.Total())
	}
	return stats
}

// setItems replaces a column's items, re-applying any active filter. The
// list's SetItems defers filtering to a returned command; feeding the
// resulting message back through Update is the synchronous equivalent.
// Without it, rebuilding a filtered column would blank it until the
// (unscheduled) command ran. Update has a value receiver, so its result
// must be written back.
func setItems(l *list.Model, items []list.Item) {
	cmd := l.SetItems(items)
	if cmd != nil {
		if msg := cmd(); msg != nil {
			nl, _ := l.Update(msg)
			*l = nl
		}
	}
}

// rebuildFrom rebuilds columns col..3 from the current selections.
func (m *model) rebuildFrom(col int) {
	if col == 0 {
		var items []list.Item
		for _, g := range analysis.GroupByPrefix(m.stats()) {
			title := g.Prefix + "/"
			if g.Prefix == "" {
				title = "(no prefix)"
			}
			keys := fmt.Sprintf("%d keys", len(g.Keys))
			if len(g.Keys) == 1 {
				keys = "1 key"
			}
			items = append(items, item{
				title:   title,
				desc:    keys,
				payload: g.Prefix,
			})
		}
		setItems(&m.cols[0], items)
		if len(items) == 0 {
			setItems(&m.cols[1], nil)
		}
	}
	if col <= 1 {
		m.rebuildKeys()
	}
	if col <= 2 {
		m.rebuildValues()
	}
	m.rebuildResources()
}

func (m *model) selected(col int) (item, bool) {
	it, ok := m.cols[col].SelectedItem().(item)
	return it, ok
}

func (m *model) rebuildKeys() {
	prefixIt, ok := m.selected(0)
	if !ok {
		setItems(&m.cols[1], nil)
		return
	}
	total := m.set.Total()
	var items []list.Item
	for _, k := range m.stats() {
		if analysis.Prefix(k.Key) != prefixIt.payload {
			continue
		}
		title := analysis.ShortKey(k.Key)
		if k.Identity(total) {
			title += " " + icIdentity // identity marker
		}
		items = append(items, item{
			title:   title,
			desc:    fmt.Sprintf("%d/%d·%d", k.Coverage, total, k.Cardinality),
			payload: k.Key,
		})
	}
	setItems(&m.cols[1], items)
}

func (m *model) rebuildValues() {
	keyIt, ok := m.selected(1)
	if !ok {
		setItems(&m.cols[2], nil)
		return
	}
	vs := m.set.ValueStats(keyIt.payload, false)
	var items []list.Item
	for _, v := range vs.Values {
		items = append(items, item{
			title:   v.Value,
			desc:    fmt.Sprint(v.Count),
			payload: v.Value,
		})
	}
	if len(vs.Missing) > 0 {
		items = append(items, item{
			title:   icMissing + " (missing)",
			desc:    fmt.Sprint(len(vs.Missing)),
			payload: missingPayload,
			missing: true,
		})
	}
	setItems(&m.cols[2], items)
}

func (m *model) rebuildResources() {
	keyIt, ok := m.selected(1)
	if !ok {
		setItems(&m.cols[3], nil)
		return
	}
	valIt, ok := m.selected(2)
	if !ok {
		setItems(&m.cols[3], nil)
		return
	}
	var items []list.Item
	for _, r := range m.set.Resources {
		v, has := r.Map(m.set.Field)[keyIt.payload]
		if valIt.missing && !has {
			items = append(items, item{title: r.String(), payload: r.Name})
		} else if !valIt.missing && has && v == valIt.payload {
			items = append(items, item{title: r.String(), payload: r.Name})
		}
	}
	setItems(&m.cols[3], items)
}

// selector returns the label selector implied by the current selection.
// It is empty for annotations, which label selectors cannot match.
func (m model) selector() string {
	if m.set.Field == analysis.FieldAnnotations {
		return ""
	}
	keyIt, ok := m.selected(1)
	if !ok || m.focus < 1 {
		return ""
	}
	if valIt, ok := m.selected(2); ok && m.focus >= 2 {
		if valIt.missing {
			return "!" + keyIt.payload
		}
		return keyIt.payload + "=" + valIt.payload
	}
	return keyIt.payload
}

func (m *model) updateFocus() {
	for i := range m.cols {
		m.delegates[i].focused = i == m.focus
		m.cols[i].SetDelegate(m.delegates[i])
	}
}

// clearFilter discards the filter applied to col, if any, and reports whether
// one was removed. The underlying selection is unaffected.
func (m *model) clearFilter(col int) bool {
	if m.cols[col].FilterState() != list.FilterApplied {
		return false
	}
	m.cols[col].ResetFilter()
	return true
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		colW, _ := m.geometry()
		colH := m.colHeight()
		for i := range m.cols {
			m.cols[i].SetSize(colW, colH)
			m.delegates[i].width = colW
			m.cols[i].SetDelegate(m.delegates[i])
			m.cols[i].FilterInput.Width = colW - 4
		}
		return m, nil
	case tea.MouseMsg:
		// while a filter is being typed, keep interaction keyboard-only
		if m.cols[m.focus].FilterState() == list.Filtering {
			return m, nil
		}
		return m, m.handleMouse(msg)
	case tea.KeyMsg:
		// while a column filter is being typed, keys belong to that column
		if m.cols[m.focus].FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.cols[m.focus], cmd = m.cols[m.focus].Update(msg)
			if m.cols[m.focus].FilterState() != list.Filtering {
				m.rebuildFrom(m.focus + 1)
				return m, cmd
			}
			// bubbles applies live filtering by emitting FilterMatchesMsg from
			// a returned command; do it synchronously too so the narrowing is
			// visible even before that command runs.
			setItems(&m.cols[m.focus], m.cols[m.focus].Items())
			return m, cmd
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "left", "h", "esc", "backspace":
			// esc discards the focused column's filter before stepping back
			if msg.String() == "esc" && m.clearFilter(m.focus) {
				m.status = icClear + " filter cleared"
				return m, nil
			}
			if m.focus > 0 {
				m.focus--
				m.updateFocus()
			}
			return m, nil
		case "right", "l", "enter", "tab":
			if m.focus < 3 && len(m.cols[m.focus].Items()) > 0 {
				m.focus++
				m.updateFocus()
			}
			return m, nil
		case "x":
			if m.clearFilter(m.focus) {
				m.status = icClear + " filter cleared"
			} else {
				m.status = icInfo + " no filter to clear"
			}
			return m, nil
		case "X":
			cleared := 0
			for i := range m.cols {
				if m.clearFilter(i) {
					cleared++
				}
			}
			if cleared > 0 {
				m.status = fmt.Sprintf("%s cleared %d filters", icClear, cleared)
			} else {
				m.status = icInfo + " no filters to clear"
			}
			return m, nil
		case "v":
			m.vary = !m.vary
			m.rebuildFrom(0)
			if m.vary {
				m.status = icBolt + " showing distinctive keys only"
			} else {
				m.status = icBolt + " showing all keys"
			}
			return m, nil
		case "c":
			if m.set.Field == analysis.FieldAnnotations {
				m.status = icInfo + " -l selectors match labels only"
				return m, nil
			}
			sel := m.selector()
			if sel == "" {
				m.status = icInfo + " select a key first"
				return m, nil
			}
			if err := clipboard.WriteAll(sel); err != nil {
				m.status = icInfo + " clipboard unavailable; selector: " + sel
			} else {
				m.status = icCopy + " copied: -l " + sel
			}
			return m, nil
		}
	}

	prevIdx := m.cols[m.focus].Index()
	var cmd tea.Cmd
	m.cols[m.focus], cmd = m.cols[m.focus].Update(msg)
	if m.cols[m.focus].Index() != prevIdx {
		m.rebuildFrom(m.focus + 1)
	}
	return m, cmd
}

// handleMouse maps a mouse event onto the 4-column layout: the screen is
// line 0 = app header, line 1 = column titles (with clickable action icons
// at the right edge), lines 2..2+colHeight-1 = list rows. It returns the
// command produced by entering filter mode (cursor blink).
func (m *model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	colW, pitch := m.geometry()
	col := msg.X / pitch
	if col < 0 || col > 3 {
		return nil
	}
	colX := col * pitch

	if tea.MouseEvent(msg).IsWheel() {
		prev := m.cols[col].Index()
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.cols[col].CursorUp()
		case tea.MouseButtonWheelDown:
			m.cols[col].CursorDown()
		}
		if m.cols[col].Index() != prev {
			m.rebuildFrom(col + 1)
		}
		return nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}

	switch {
	case msg.Y == 1: // column title row
		// icon targets span 2 cells (icon + its left gap) so they stay
		// clickable even on terminals that render the glyphs wider
		clearX, clearHi := colX+colW-5, colX+colW-4
		searchX, searchHi := colX+colW-3, colX+colW-2
		switch {
		case m.cols[col].FilterState() == list.FilterApplied && msg.X >= clearX && msg.X <= clearHi:
			m.clearFilter(col)
			m.status = icClear + " filter cleared"
			return nil
		case msg.X >= searchX && msg.X <= searchHi:
			// magnifier: focus the column and open its filter prompt
			m.focus = col
			m.updateFocus()
			var cmd tea.Cmd
			m.cols[col], cmd = m.cols[col].Update(
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
			return cmd
		default:
			m.focus = col
			m.updateFocus()
			return nil
		}
	case msg.Y >= 2 && msg.Y < 2+m.colHeight():
		m.selectRow(col, msg.Y-2)
	}
	return nil
}

// selectRow moves the cursor of col to the visible row at the given offset
// within its current page, focuses the column, and rebuilds everything to
// its right.
func (m *model) selectRow(col, row int) {
	l := &m.cols[col]
	page, perPage := l.Paginator.Page, l.Paginator.PerPage
	onPage := min(perPage, len(l.VisibleItems())-page*perPage)
	if row >= onPage {
		return // clicked the padding/pagination area below the items
	}
	prev := l.Index()
	m.focus = col
	m.updateFocus()
	l.Select(page*perPage + row)
	if l.Index() != prev {
		m.rebuildFrom(col + 1)
	}
}

// truncate truncates s to max runes, appending an ellipsis when cut.
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 2 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

// columnHeader renders the title line of column i. The trailing action icons
// sit at fixed cells so the mouse handler can hit them without shared state:
// the magnifier at colW-2, and the clear button at colW-4 while a filter is
// applied. While a filter is being typed, the line becomes the filter input.
func (m model) columnHeader(i int) string {
	colW, _ := m.geometry()
	l := m.cols[i]

	if l.FilterState() == list.Filtering {
		return " " + searchStyle.Render(icSearch+" ") + l.FilterInput.View()
	}

	filtered := l.FilterState() == list.FilterApplied
	right := searchStyle.Render(icSearch) + " "
	if filtered {
		right = clearStyle.Render(icClear) + " " + right
	}
	left := fmt.Sprintf(" %s (%d)", columnTitles[i], len(l.VisibleItems()))
	chip := ""
	if filtered {
		chip = " " + filterStyle.Render(icFilter+" "+truncate(l.FilterValue(), colW/3))
	}

	maxLeft := colW - lipgloss.Width(chip) - lipgloss.Width(right) - 1
	if maxLeft < 1 {
		maxLeft = 1
	}
	left = truncate(left, maxLeft)
	pad := colW - lipgloss.Width(left) - lipgloss.Width(chip) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	return headerStyle.Render(left) + chip + strings.Repeat(" ", pad) + right
}

func (m model) View() string {
	total := m.set.Total()
	stats := m.stats()
	header := titleStyle.Render(fmt.Sprintf("%s %d resources · %s %d %s keys", icResources, total, icKeys, len(stats), m.set.Field.Noun()))
	if m.vary {
		header += headerStyle.Render("  " + icBolt + " distinctive only — v to toggle")
	}

	var rendered []string
	for i := range m.cols {
		border := plainBorder
		if i == m.focus {
			border = focusedBorder
		}
		col := m.columnHeader(i) + "\n" + m.cols[i].View()
		rendered = append(rendered, border.Render(col))
	}
	columns := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)

	sel := m.selector()
	footerLeft := ""
	if sel != "" {
		footerLeft = helpStyle.Render(icTarget+" selector: ") + sel
	}
	status := ""
	if m.status != "" {
		status = "  " + statusStyle.Render(m.status)
	}
	help := helpStyle.Render("↑↓/jk move · ←→/hl/tab drill · / " + icSearch + " filter · x " + icClear + " clear · v " + icBolt + " vary · c " + icCopy + " copy · q quit · click & wheel work")
	return header + "\n" + columns + "\n" + footerLeft + status + "\n" + help
}
