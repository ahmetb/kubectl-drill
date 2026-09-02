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

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ahmetb/kubectl-labels/internal/analysis"
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

func runeKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func special(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

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
	for _, want := range []string{"PREFIXES", "KEYS", "VALUES", "RESOURCES", "3 resources", "4 keys", "kubernetes.io/", "example.com/"} {
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
	if m.status != "select a key first" {
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
