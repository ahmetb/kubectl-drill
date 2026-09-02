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
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TestProgramEndToEnd drives the real bubbletea program loop (not just
// model.Update) through a drill-down and quits.
func TestProgramEndToEnd(t *testing.T) {
	m := newModel(testSet())
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "PREFIXES")
	}, teatest.WithCheckInterval(50*time.Millisecond), teatest.WithDuration(5*time.Second))

	// click the first KEYS row to focus it and select; the footer should
	// then show the implied label selector
	tm.Send(tea.MouseMsg{X: 35, Y: 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "selector:")
	}, teatest.WithCheckInterval(50*time.Millisecond), teatest.WithDuration(5*time.Second))

	// drill into values, toggle vary, then quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRight})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
	out := tm.FinalOutput(t)
	b, err := readAll(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "keys") {
		t.Errorf("final output looks wrong:\n%s", b)
	}
}

func readAll(tm interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var out []byte
	buf := make([]byte, 1<<16)
	for {
		n, err := tm.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			return out, nil // EOF expected after quit
		}
	}
}
