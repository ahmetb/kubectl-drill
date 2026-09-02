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

// Package collect queries Kubernetes resources using the same builder
// machinery as "kubectl get" and reduces them to their label sets.
//
// File input (-f) is handled without the builder so that it works fully
// offline: the resource builder resolves REST mappings via server
// discovery, which would make manifest analysis require a cluster.
package collect

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/utils/ptr"

	"github.com/ahmetb/kubectl-labels/internal/analysis"
)

// Options mirrors the classic kubectl get selectors.
type Options struct {
	Namespace     string
	AllNamespaces bool
	Selector      string
	Filenames     []string
	Recursive     bool
	Kustomize     string
	Args          []string // resource type, type/name, etc.
}

// Query fetches the matching resources and extracts their labels.
func Query(configFlags *genericclioptions.ConfigFlags, opts Options) ([]analysis.Resource, error) {
	if len(opts.Filenames) > 0 {
		return fromFiles(opts)
	}
	return fromServer(configFlags, opts)
}

// fromServer queries the API server via the resource builder.
func fromServer(configFlags *genericclioptions.ConfigFlags, opts Options) ([]analysis.Resource, error) {
	kubeconfigNamespace, _, err := configFlags.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return nil, fmt.Errorf("failed to determine namespace from kubeconfig: %w", err)
	}

	rb := resource.NewBuilder(configFlags)
	if namespace := ptr.Deref(configFlags.Namespace, ""); namespace != "" {
		rb.NamespaceParam(namespace)
	} else if kubeconfigNamespace != "" {
		rb.NamespaceParam(kubeconfigNamespace)
	}

	builder := rb.DefaultNamespace().
		AllNamespaces(opts.AllNamespaces).
		Unstructured().
		ResourceTypeOrNameArgs(true, opts.Args...).
		FilenameParam(false, &resource.FilenameOptions{Kustomize: opts.Kustomize}).
		LabelSelectorParam(opts.Selector).
		Flatten().
		ContinueOnError()

	// Latest() re-fetching only makes sense for live queries; -k builds
	// in-memory objects whose labels are what the user wants to inspect.
	if opts.Kustomize == "" {
		builder = builder.Latest()
	}

	var out []analysis.Resource
	err = builder.Do().
		Visit(func(info *resource.Info, err error) error {
			if err != nil {
				return err
			}
			accessor, err := meta.Accessor(info.Object)
			if err != nil {
				return fmt.Errorf("failed to read metadata of %s: %w", info.Name, err)
			}
			out = append(out, analysis.Resource{
				Kind:      info.Object.GetObjectKind().GroupVersionKind().Kind,
				Namespace: info.Namespace,
				Name:      info.Name,
				Labels:    accessor.GetLabels(),
			})
			return nil
		})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// fromFiles loads resources from local manifests (JSON/YAML, multi-doc,
// and List kinds), directories, or "-" for stdin. No cluster needed.
func fromFiles(opts Options) ([]analysis.Resource, error) {
	var out []analysis.Resource
	for _, path := range opts.Filenames {
		if path == "-" {
			rs, err := decode("stdin", os.Stdin)
			if err != nil {
				return nil, err
			}
			out = append(out, rs...)
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			if !opts.Recursive {
				return nil, fmt.Errorf("%s is a directory; pass --recursive to read manifests from directories", path)
			}
			err = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if fi.IsDir() || !isManifestFile(p) {
					return nil
				}
				rs, err := decodeFile(p)
				if err != nil {
					return err
				}
				out = append(out, rs...)
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		rs, err := decodeFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, rs...)
	}
	return filter(out, opts)
}

func isManifestFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") {
		return false
	}
	return ext == ".json" || ext == ".yaml" || ext == ".yml"
}

func decodeFile(path string) ([]analysis.Resource, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return decode(path, f)
}

// decode reads a stream of YAML or JSON documents, expanding List kinds.
func decode(source string, r io.Reader) ([]analysis.Resource, error) {
	var out []analysis.Resource
	dec := utilyaml.NewYAMLOrJSONDecoder(r, 4096)
	for {
		var raw map[string]any
		err := dec.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", source, err)
		}
		if len(raw) == 0 {
			continue
		}
		u := unstructured.Unstructured{Object: raw}
		if u.IsList() {
			err := u.EachListItem(func(obj runtime.Object) error {
				out = append(out, toResource(*obj.(*unstructured.Unstructured)))
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("failed to read list in %s: %w", source, err)
			}
			continue
		}
		out = append(out, toResource(u))
	}
	return out, nil
}

func toResource(u unstructured.Unstructured) analysis.Resource {
	return analysis.Resource{
		Kind:      u.GetKind(),
		Namespace: u.GetNamespace(),
		Name:      u.GetName(),
		Labels:    u.GetLabels(),
	}
}

// filter applies the namespace and label selectors to file-loaded resources.
func filter(rs []analysis.Resource, opts Options) ([]analysis.Resource, error) {
	var sel labels.Selector
	if opts.Selector != "" {
		var err error
		sel, err = labels.Parse(opts.Selector)
		if err != nil {
			return nil, fmt.Errorf("invalid selector: %w", err)
		}
	}
	out := rs[:0]
	for _, r := range rs {
		if opts.Namespace != "" && !opts.AllNamespaces && r.Namespace != opts.Namespace {
			continue
		}
		if sel != nil && !sel.Matches(labels.Set(r.Labels)) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}
