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

package collect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ahmetb/kubectl-labels/internal/analysis"
)

func TestFromFilesJSON(t *testing.T) {
	rs, err := fromFiles(Options{Filenames: []string{"../../testdata/nodes.json"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 4 {
		t.Fatalf("got %d resources, want 4", len(rs))
	}
	if rs[0].Kind != "Node" || rs[0].Name == "" {
		t.Errorf("unexpected first resource: %+v", rs[0])
	}
}

func TestFromFilesMultiDocYAML(t *testing.T) {
	rs, err := fromFiles(Options{Filenames: []string{"../../testdata/pods.yaml"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("got %d pods, want 2", len(rs))
	}
	for _, r := range rs {
		if r.Kind != "Pod" {
			t.Errorf("kind = %q, want Pod", r.Kind)
		}
	}
}

func TestFromFilesStdin(t *testing.T) {
	f, err := os.Open("../../testdata/nodes.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	old := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = old }()

	rs, err := fromFiles(Options{Filenames: []string{"-"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 4 {
		t.Fatalf("got %d resources from stdin, want 4", len(rs))
	}
}

func TestFromFilesDirectoryNeedsRecursive(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.yaml"), []byte("kind: Node\napiVersion: v1\nmetadata:\n  name: node-x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := fromFiles(Options{Filenames: []string{dir}}); err == nil {
		t.Error("directory without --recursive should fail")
	}
	rs, err := fromFiles(Options{Filenames: []string{dir}, Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].Name != "node-x" {
		t.Errorf("unexpected recursive read: %+v", rs)
	}
}

func TestFromFilesListOfManifests(t *testing.T) {
	dir := t.TempDir()
	list := `apiVersion: v1
kind: List
items:
- apiVersion: v1
  kind: Node
  metadata:
    name: node-a
- apiVersion: v1
  kind: Node
  metadata:
    name: node-b
`
	if err := os.WriteFile(filepath.Join(dir, "list.yaml"), []byte(list), 0o644); err != nil {
		t.Fatal(err)
	}
	rs, err := fromFiles(Options{Filenames: []string{filepath.Join(dir, "list.yaml")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].Name != "node-a" || rs[1].Name != "node-b" {
		t.Errorf("List kind not expanded: %+v", rs)
	}
}

func TestFilterSelector(t *testing.T) {
	rs := []analysis.Resource{
		{Name: "a", Labels: map[string]string{"app": "web"}},
		{Name: "b", Labels: map[string]string{"app": "api"}},
		{Name: "c", Labels: map[string]string{"tier": "web"}},
	}
	got, err := filter(rs, Options{Selector: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("selector app matched %d, want 2: %+v", len(got), got)
	}
	if _, err := filter(rs, Options{Selector: "==bogus=="}); err == nil {
		t.Error("bad selector should error")
	}
}

func TestFilterNamespace(t *testing.T) {
	rs := []analysis.Resource{
		{Namespace: "a", Name: "x"},
		{Namespace: "b", Name: "x"},
		{Name: "cluster-scoped"},
	}
	got, err := filter(rs, Options{Namespace: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Namespace != "a" {
		t.Errorf("namespace filter: %+v", got)
	}
	// --all-namespaces lifts the namespace restriction
	got, err = filter(rs, Options{Namespace: "a", AllNamespaces: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("all-namespaces should keep everything: %+v", got)
	}
}

func TestIsManifestFile(t *testing.T) {
	for path, want := range map[string]bool{
		"a.json": true, "a.yaml": true, "a.YML": true,
		"a.txt": false, ".hidden.yaml": false, "a": false,
	} {
		if got := isManifestFile(path); got != want {
			t.Errorf("isManifestFile(%q) = %v, want %v", path, got, want)
		}
	}
}
