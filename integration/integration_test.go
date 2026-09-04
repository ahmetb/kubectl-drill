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

// Package integration_test exercises the whole pipeline (collect -> render)
// against a static manifest, without needing a cluster.
package integration_test

import (
	"bytes"
	"strings"
	"testing"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/ahmetb/kubectl-drill/internal/analysis"
	"github.com/ahmetb/kubectl-drill/internal/collect"
	"github.com/ahmetb/kubectl-drill/internal/render"
)

func loadTestSet(t *testing.T) analysis.Set {
	t.Helper()
	cf := genericclioptions.NewConfigFlags(true)
	resources, err := collect.Query(cf, collect.Options{
		Filenames: []string{"../testdata/nodes.json"},
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(resources) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(resources))
	}
	return analysis.Set{Resources: resources}
}

func TestCollectFromFile(t *testing.T) {
	set := loadTestSet(t)
	for _, r := range set.Resources {
		if r.Kind != "Node" {
			t.Errorf("kind = %q, want Node", r.Kind)
		}
		if r.Name == "" {
			t.Error("empty name")
		}
	}
	// spot-check labels survived the pipeline
	var node0 *analysis.Resource
	for i := range set.Resources {
		if set.Resources[i].Name == "node-0" {
			node0 = &set.Resources[i]
		}
	}
	if node0 == nil {
		t.Fatal("node-0 not found")
	}
	if node0.Labels["example.com/pool"] != "cpu" {
		t.Errorf("node-0 pool = %q", node0.Labels["example.com/pool"])
	}
	if v, ok := node0.Labels["node-role.kubernetes.io/worker"]; !ok || v != "" {
		t.Errorf("empty-value label lost: %q %v", v, ok)
	}
}

func TestKeySummaryView(t *testing.T) {
	set := loadTestSet(t)
	var buf bytes.Buffer
	err := render.KeySummary(&buf, set, render.Options{SetLabel: "nodes"})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"4 nodes", "10 distinct keys", "9 distinctive",
		"kubernetes.io/hostname", "(identity)",
		"example.com/pool",
		"1 uniform key hidden",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
	// uniform key hidden by default
	if strings.Contains(out, "node-init.example.com/ready") {
		t.Errorf("uniform key should be hidden by default\n%s", out)
	}

	buf.Reset()
	if err := render.KeySummary(&buf, set, render.Options{SetLabel: "nodes", All: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "node-init.example.com/ready") {
		t.Errorf("--all should show uniform keys\n%s", buf.String())
	}
}

func TestValueDistributionView(t *testing.T) {
	set := loadTestSet(t)
	var buf bytes.Buffer
	if err := render.ValueDistribution(&buf, set, "example.com/pool", render.Options{SetLabel: "nodes"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"present on 3/4", "2 distinct values", "cpu", "arm", "missing on 1: node-3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestResourceListVaryComparesAgainstPeers(t *testing.T) {
	set := loadTestSet(t)
	// simulate "nodes/node-2 --vary": display node-2, stats from all 4
	var view analysis.Set
	for _, r := range set.Resources {
		if r.Name == "node-2" {
			view.Resources = append(view.Resources, r)
		}
	}
	var buf bytes.Buffer
	if err := render.ResourceList(&buf, view, set, render.Options{Vary: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// arch=arm64 distinguishes node-2; the uniform key must be hidden
	if !strings.Contains(out, "kubernetes.io/arch=arm64") {
		t.Errorf("expected distinctive label in output\n%s", out)
	}
	if strings.Contains(out, "node-init.example.com/ready") {
		t.Errorf("uniform label must be hidden with --vary\n%s", out)
	}
}

func TestJSONOutput(t *testing.T) {
	set := loadTestSet(t)
	var buf bytes.Buffer
	if err := render.KeySummary(&buf, set, render.Options{Output: "json"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"resources": 4`) || !strings.Contains(out, `"key": "kubernetes.io/hostname"`) {
		t.Errorf("unexpected json output\n%s", out)
	}
}

func TestAnnotationsPipeline(t *testing.T) {
	resources, err := collect.Query(genericclioptions.NewConfigFlags(true), collect.Options{
		Filenames: []string{"../testdata/nodes.json"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resources {
		if r.Annotations["example.com/owner"] != "platform" {
			t.Errorf("%s lost its annotations: %+v", r.Name, r.Annotations)
		}
	}
	set := analysis.Set{Resources: resources, Field: analysis.FieldAnnotations}

	var buf bytes.Buffer
	if err := render.KeySummary(&buf, set, render.Options{SetLabel: "nodes"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"4 nodes", "3 distinct keys", "2 distinctive",
		"example.com/cost-center", "example.com/spot",
		"1 uniform key hidden",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "kubernetes.io/hostname") {
		t.Errorf("labels must not leak into the annotations view\n%s", out)
	}

	buf.Reset()
	if err := render.ValueDistribution(&buf, set, "example.com/cost-center", render.Options{SetLabel: "nodes"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"present on 3/4 nodes", "cc-0", "missing on 1: node-3"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("output missing %q\n%s", want, buf.String())
		}
	}
}
