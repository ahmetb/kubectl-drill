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

package render

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/ahmetb/kubectl-labels/internal/analysis"
)

func setOf(n int) analysis.Set {
	var rs []analysis.Resource
	for i := 0; i < n; i++ {
		rs = append(rs, analysis.Resource{
			Kind: "Node", Name: fmt.Sprintf("node-%d", i),
			Labels: map[string]string{"kubernetes.io/hostname": fmt.Sprintf("node-%d", i)},
		})
	}
	return analysis.Set{Resources: rs}
}

func TestKeySummaryNoLabelsAtAll(t *testing.T) {
	set := analysis.Set{Resources: []analysis.Resource{
		{Kind: "Node", Name: "node-0"},
		{Kind: "Node", Name: "node-1"},
	}}
	var buf bytes.Buffer
	if err := KeySummary(&buf, set, Options{SetLabel: "nodes"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "none of the 2 nodes carry any labels") {
		t.Errorf("missing empty-set message\n%s", out)
	}
	if strings.Contains(out, "identical") {
		t.Errorf("must not claim labels are identical when there are none\n%s", out)
	}
}

func TestValueDistributionCapsRows(t *testing.T) {
	// 120 values exceeds maxValueRows; the default view truncates, --all shows
	set := setOf(maxValueRows + 70)
	var buf bytes.Buffer
	if err := ValueDistribution(&buf, set, "kubernetes.io/hostname", Options{SetLabel: "nodes"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, fmt.Sprintf("%d more values hidden", set.Total()-maxValueRows)) {
		t.Errorf("missing truncation footer\n%s", out[:200])
	}
	if strings.Count(out, "█") < 1 {
		t.Error("no distribution bars rendered")
	}

	buf.Reset()
	if err := ValueDistribution(&buf, set, "kubernetes.io/hostname", Options{SetLabel: "nodes", All: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "more values hidden") {
		t.Error("--all must not truncate\n")
	}
	if !strings.Contains(buf.String(), fmt.Sprintf("%d distinct values", set.Total())) {
		t.Errorf("--all should report the full value count\n%s", buf.String()[:200])
	}
}

func TestValueDistributionAbsentKey(t *testing.T) {
	set := setOf(3)
	var buf bytes.Buffer
	if err := ValueDistribution(&buf, set, "does.not/exist", Options{SetLabel: "nodes"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "no nodes carry this label key") {
		t.Errorf("unexpected output\n%s", buf.String())
	}
}

func TestResourceListSortedByNamespaceThenName(t *testing.T) {
	set := analysis.Set{Resources: []analysis.Resource{
		{Kind: "Pod", Namespace: "b", Name: "z", Labels: map[string]string{"app": "x"}},
		{Kind: "Pod", Namespace: "a", Name: "y", Labels: map[string]string{"app": "x"}},
		{Kind: "Pod", Namespace: "a", Name: "x", Labels: map[string]string{"app": "x"}},
	}}
	var buf bytes.Buffer
	if err := ResourceList(&buf, set, set, Options{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	xa := strings.Index(out, "a/x")
	ay := strings.Index(out, "a/y")
	bz := strings.Index(out, "b/z")
	if !(xa >= 0 && ay > xa && bz > ay) {
		t.Errorf("resources not sorted by namespace then name:\n%s", out)
	}
}

func TestResourceListNoLabels(t *testing.T) {
	set := analysis.Set{Resources: []analysis.Resource{{Kind: "Pod", Name: "p"}}}
	var buf bytes.Buffer
	if err := ResourceList(&buf, set, set, Options{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "(no labels)") {
		t.Errorf("unexpected output\n%s", buf.String())
	}
}

func TestCountNoun(t *testing.T) {
	for n, want := range map[int]string{0: "nodes", 1: "node", 2: "nodes", 4: "nodes"} {
		if got := CountNoun(n, "nodes"); got != want {
			t.Errorf("CountNoun(%d) = %q, want %q", n, got, want)
		}
	}
	if got := CountNoun(1, "resources"); got != "resource" {
		t.Errorf("CountNoun(1, resources) = %q", got)
	}
}

func TestTruncate(t *testing.T) {
	for in, want := range map[string]string{
		"short":                 "short",
		"":                      "",
		strings.Repeat("x", 48): strings.Repeat("x", 48),
		strings.Repeat("x", 49): strings.Repeat("x", 47) + "…",
		strings.Repeat("é", 60): strings.Repeat("é", 47) + "…", // rune-safe, not byte-safe
	} {
		if got := truncate(in, 48); got != want {
			t.Errorf("truncate(%q, 48) = %q, want %q", in, got, want)
		}
	}
}

func TestSamplesEmptyValueShown(t *testing.T) {
	k := analysis.KeyStat{
		Cardinality: 2,
		Values: []analysis.ValueCount{
			{Value: "", Count: 2},
			{Value: "x", Count: 1},
		},
	}
	if got := samples(k, maxSamples); !strings.Contains(got, `""`) {
		t.Errorf("empty value should render as \"\": %q", got)
	}
}
