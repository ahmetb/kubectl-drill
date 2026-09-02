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

// Package tui implements the interactive label browser: a Miller-columns
// drill-down of prefix -> key -> value -> resources.
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

	"github.com/ahmetb/kubectl-labels/internal/analysis"
)

const missingPayload = "\x00missing"

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

var columnTitles = []string{"PREFIXES", "KEYS", "VALUES", "RESOURCES"}

// Run starts the interactive browser.
func Run(set analysis.Set) error {
	m := newModel(set)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func newModel(set analysis.Set) model {
	m := model{set: set, width: 100, height: 30}
	for i := range m.cols {
		l := list.New(nil, delegate{width: 24}, 24, 10)
		l.SetShowTitle(false)
		l.SetShowStatusBar(false)
		l.SetShowHelp(false)
		l.SetShowPagination(true)
		l.KeyMap.Quit = key.NewBinding(key.WithKeys("ctrl+c")) // free up q for the app
		m.cols[i] = l
	}
	m.rebuildFrom(0)
	m.updateFocus()
	return m
}

func (m model) Init() tea.Cmd { return nil }

func (m *model) stats() []analysis.KeyStat {
	stats := m.set.KeyStats()
	analysis.SortKeyStats(stats, "")
	if m.vary {
		stats = analysis.Distinctive(stats, m.set.Total())
	}
	return stats
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
		m.cols[0].SetItems(items)
		if len(items) == 0 {
			m.cols[1].SetItems(nil)
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
		m.cols[1].SetItems(nil)
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
			title += " ◦" // identity marker
		}
		items = append(items, item{
			title:   title,
			desc:    fmt.Sprintf("%d/%d·%d", k.Coverage, total, k.Cardinality),
			payload: k.Key,
		})
	}
	m.cols[1].SetItems(items)
}

func (m *model) rebuildValues() {
	keyIt, ok := m.selected(1)
	if !ok {
		m.cols[2].SetItems(nil)
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
			title:   "(missing)",
			desc:    fmt.Sprint(len(vs.Missing)),
			payload: missingPayload,
			missing: true,
		})
	}
	m.cols[2].SetItems(items)
}

func (m *model) rebuildResources() {
	keyIt, ok := m.selected(1)
	if !ok {
		m.cols[3].SetItems(nil)
		return
	}
	valIt, ok := m.selected(2)
	if !ok {
		m.cols[3].SetItems(nil)
		return
	}
	var items []list.Item
	for _, r := range m.set.Resources {
		v, has := r.Labels[keyIt.payload]
		if valIt.missing && !has {
			items = append(items, item{title: r.String(), payload: r.Name})
		} else if !valIt.missing && has && v == valIt.payload {
			items = append(items, item{title: r.String(), payload: r.Name})
		}
	}
	m.cols[3].SetItems(items)
}

// selector returns the label selector implied by the current selection.
func (m model) selector() string {
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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		colW := (m.width - 4) / 4
		if colW < 16 {
			colW = 16
		}
		colH := m.height - 6
		if colH < 3 {
			colH = 3
		}
		for i := range m.cols {
			m.cols[i].SetSize(colW, colH)
			m.delegates[i].width = colW
			m.cols[i].SetDelegate(m.delegates[i])
		}
		return m, nil
	case tea.KeyMsg:
		// while a column filter is being typed, keys belong to that column
		if m.cols[m.focus].FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.cols[m.focus], cmd = m.cols[m.focus].Update(msg)
			if m.cols[m.focus].FilterState() != list.Filtering {
				m.rebuildFrom(m.focus + 1)
			}
			return m, cmd
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "left", "h", "esc", "backspace":
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
		case "v":
			m.vary = !m.vary
			m.rebuildFrom(0)
			if m.vary {
				m.status = "showing distinctive keys only"
			} else {
				m.status = "showing all keys"
			}
			return m, nil
		case "c":
			sel := m.selector()
			if sel == "" {
				m.status = "select a key first"
				return m, nil
			}
			if err := clipboard.WriteAll(sel); err != nil {
				m.status = "clipboard unavailable; selector: " + sel
			} else {
				m.status = "copied: -l " + sel
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

func (m model) View() string {
	total := m.set.Total()
	stats := m.stats()
	header := titleStyle.Render(fmt.Sprintf("%d resources · %d keys", total, len(stats)))
	if m.vary {
		header += headerStyle.Render("  (distinctive only — v to toggle)")
	}

	var rendered []string
	for i := range m.cols {
		border := plainBorder
		if i == m.focus {
			border = focusedBorder
		}
		title := fmt.Sprintf(" %s (%d)", columnTitles[i], len(m.cols[i].VisibleItems()))
		col := headerStyle.Render(title) + "\n" + m.cols[i].View()
		rendered = append(rendered, border.Render(col))
	}
	columns := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)

	sel := m.selector()
	footerLeft := ""
	if sel != "" {
		footerLeft = helpStyle.Render("selector: ") + sel
	}
	status := ""
	if m.status != "" {
		status = "  " + statusStyle.Render(m.status)
	}
	help := helpStyle.Render("↑↓/jk move · ←→/hl/tab drill · esc back · / filter · v vary · c copy selector · q quit")
	return header + "\n" + columns + "\n" + footerLeft + status + "\n" + help
}
