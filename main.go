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

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/utils/ptr"

	"github.com/ahmetb/kubectl-drill/internal/analysis"
	"github.com/ahmetb/kubectl-drill/internal/collect"
	"github.com/ahmetb/kubectl-drill/internal/render"
	"github.com/ahmetb/kubectl-drill/internal/tui"
)

var (
	allNamespaces bool
	selectorOpt   string
	filenameOpts  = &resource.FilenameOptions{}

	listFlag     bool
	varyFlag     bool
	allFlag      bool
	sortBy       string
	groupPrefix  bool
	outputFormat string
	noColorFlag  bool
	boringFlag   bool
	forceStatic  bool // hidden view aliases print tables, never the TUI
)

// Build information, overridden by goreleaser ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	configFlags := genericclioptions.NewConfigFlags(true)

	root := &cobra.Command{
		Use:   "drill [labels|annotations] TYPE[/NAME] [KEY]",
		Short: "Drill into Kubernetes resource labels and annotations",
		Long: `kubectl drill queries a set of resources (by type, name, label
selector, namespace, or file) and pivots their labels or annotations:
which keys exist, their cardinality and coverage, how values are
distributed, and what each resource carries.

The default view is the interactive drill-down (prefix > key > value >
resource). Pass --boring for the static, scriptable tables.

With no KEY it summarizes keys across the set. With KEY it shows the
value distribution. With TYPE/NAME it lists one resource's pairs.`,
		Example: `  # Drill into node labels (the default is interactive)
  kubectl drill nodes
  kubectl drill labels nodes

  # Static, scriptable key summary
  kubectl drill labels nodes --boring

  # Drill into annotations instead
  kubectl drill annotations pods

  # How is a label distributed across nodes?
  kubectl drill labels nodes --boring topology.kubernetes.io/zone

  # What makes THIS node different from its peers?
  kubectl drill labels nodes/node-1 --boring --vary

  # What differs among these pods?
  kubectl drill labels pods -n prod -l app=web --boring --vary`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(3),
		Version:       version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(configFlags, args)
		},
	}
	root.SetVersionTemplate(versionString())

	staticRun := func(cmd *cobra.Command, args []string) error {
		forceStatic = true
		return run(configFlags, args)
	}
	keysAlias := &cobra.Command{
		Use:          "keys TYPE[/NAME]",
		Hidden:       true,
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE:         staticRun,
	}
	valuesAlias := &cobra.Command{
		Use:          "values TYPE[/NAME] KEY",
		Hidden:       true,
		SilenceUsage: true,
		Args:         cobra.RangeArgs(1, 2),
		RunE:         staticRun,
	}
	listAlias := &cobra.Command{
		Use:          "list TYPE[/NAME]",
		Hidden:       true,
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			listFlag = true
			return staticRun(cmd, args)
		},
	}
	root.AddCommand(keysAlias, valuesAlias, listAlias, &cobra.Command{
		Use:          "version",
		Short:        "Print the kubectl-drill version",
		SilenceUsage: true,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(versionString())
		},
	})

	pf := root.PersistentFlags()
	pf.BoolVarP(&allNamespaces, "all-namespaces", "A", false, "If present, list the requested object(s) across all namespaces")
	pf.StringVarP(&selectorOpt, "selector", "l", "", "Selector (label query) to filter on, supports '=', '==', '!=', 'in', 'notin'")
	pf.StringSliceVarP(&filenameOpts.Filenames, "filename", "f", nil, "Filename, directory, or URL to files identifying the resources")
	pf.BoolVar(&filenameOpts.Recursive, "recursive", false, "Process the directory used in -f, --filename recursively")
	pf.StringVarP(&filenameOpts.Kustomize, "kustomize", "k", "", "Process a kustomization directory")
	pf.BoolVar(&listFlag, "list", false, "List key/value pairs per resource instead of summarizing keys")
	pf.BoolVar(&varyFlag, "vary", false, "Show only keys that differ across the set (no footers; script-friendly)")
	pf.BoolVar(&allFlag, "all", false, "Show all keys, including those identical on every resource")
	pf.StringVar(&sortBy, "sort-by", "", "Sort keys by: name, coverage, cardinality (default)")
	pf.BoolVar(&groupPrefix, "group-prefix", false, "Group keys by their DNS prefix")
	pf.StringVarP(&outputFormat, "output", "o", "", "Output format: json, yaml")
	pf.BoolVar(&noColorFlag, "no-color", false, "Disable colored output")
	pf.BoolVar(&boringFlag, "boring", false, "Print static tables instead of the interactive drill-down (implied when piping or using -o)")

	configFlags.AddFlags(pf)
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// versionString renders the --version output; commit and date are only
// shown when set by a real build (not "dev" checkouts).
func versionString() string {
	s := "kubectl-drill " + version
	if commit != "none" && commit != "" {
		s += " (" + commit
		if date != "unknown" && date != "" {
			s += ", built " + date
		}
		s += ")"
	}
	return s + "\n"
}

func run(configFlags *genericclioptions.ConfigFlags, args []string) error {
	// The first positional selects which metadata map to drill into;
	// "labels" is the default and can be omitted.
	field := analysis.FieldLabels
	if len(args) > 0 && (args[0] == "labels" || args[0] == "annotations") {
		if args[0] == "annotations" {
			field = analysis.FieldAnnotations
		}
		args = args[1:]
	}
	if len(args) == 0 && len(filenameOpts.Filenames) == 0 && filenameOpts.Kustomize == "" {
		return fmt.Errorf("specify a resource type, e.g. `kubectl drill labels nodes` (see --help)")
	}
	var resourceArg, key string
	if len(args) > 0 {
		resourceArg = args[0]
	}
	if len(args) > 1 {
		key = args[1]
	}
	if len(args) > 2 {
		return fmt.Errorf("unexpected argument %q (at most TYPE[/NAME] and KEY)", args[2])
	}
	if noColorFlag {
		color.NoColor = true
	}

	switch outputFormat {
	case "", "json", "yaml":
	default:
		return fmt.Errorf("unsupported output format %q (only json, yaml)", outputFormat)
	}
	switch sortBy {
	case "", "name", "coverage", "cardinality":
	default:
		return fmt.Errorf("unsupported --sort-by %q (only name, coverage, cardinality)", sortBy)
	}

	fileMode := len(filenameOpts.Filenames) > 0 || filenameOpts.Kustomize != ""
	if fileMode && len(args) > 1 {
		return fmt.Errorf("with -f/--filename, specify at most one positional argument (KEY or KIND/NAME)")
	}

	query := collect.Options{
		AllNamespaces: allNamespaces,
		Selector:      selectorOpt,
		Filenames:     filenameOpts.Filenames,
		Recursive:     filenameOpts.Recursive,
		Kustomize:     filenameOpts.Kustomize,
	}

	// "TYPE/NAME --vary" means: what makes THIS resource different from its
	// peers? That requires querying the whole type for comparison, then
	// displaying only the named resource.
	singleResource := strings.Contains(resourceArg, "/")
	varyAgainstPeers := varyFlag && singleResource && key == "" && !fileMode

	switch {
	case fileMode:
		// The builder rejects resource arguments combined with file input;
		// positional args are interpreted after loading instead.
	case varyAgainstPeers:
		query.Args = []string{strings.SplitN(resourceArg, "/", 2)[0]}
	case len(args) > 0:
		query.Args = args[:1]
	}

	resources, err := collect.Query(configFlags, query)
	if err != nil {
		return err
	}
	if len(resources) == 0 {
		fmt.Fprintln(os.Stderr, "No resources found")
		return nil
	}
	set := analysis.Set{Resources: resources, Field: field}

	// In file mode the positional arg is ours: KIND/NAME selects one object
	// from the file, anything else is treated as a key.
	fileResource := ""
	if fileMode && resourceArg != "" && key == "" {
		if _, ok := findResource(set, resourceArg); ok {
			fileResource = resourceArg
		} else {
			key = resourceArg
		}
		resourceArg = ""
	}

	// The interactive drill-down is the default view: it needs a terminal
	// and an open-ended question. --boring, -o, pipes, and view-specific
	// asks (KEY, --list, --vary) all get the static tables instead.
	interactive := !boringFlag && !forceStatic && outputFormat == "" &&
		key == "" && !listFlag && !varyFlag &&
		term.IsTerminal(int(os.Stdout.Fd()))

	if interactive {
		if fileResource != "" {
			if r, ok := findResource(set, fileResource); ok {
				set = analysis.Set{Resources: []analysis.Resource{*r}, Field: field}
			}
		}
		return tui.Run(set)
	}

	if fileResource != "" {
		if r, ok := findResource(set, fileResource); ok {
			opts := render.Options{Vary: varyFlag, Output: outputFormat, SetLabel: setLabel(fileResource, set)}
			return render.ResourceList(os.Stdout, analysis.Set{Resources: []analysis.Resource{*r}, Field: field}, set, opts)
		}
	}

	opts := render.Options{
		Vary:        varyFlag,
		All:         allFlag,
		SortBy:      sortBy,
		GroupPrefix: groupPrefix,
		Output:      outputFormat,
		SetLabel:    setLabel(resourceArg, set),
	}

	switch {
	case listFlag || (singleResource && key == ""):
		view := set
		if varyAgainstPeers {
			name := strings.SplitN(resourceArg, "/", 2)[1]
			view = analysis.Set{Field: field}
			for _, r := range set.Resources {
				if r.Name == name {
					view.Resources = append(view.Resources, r)
				}
			}
			if len(view.Resources) == 0 {
				return fmt.Errorf("resource %q not found among %s", name, opts.SetLabel)
			}
		}
		return render.ResourceList(os.Stdout, view, set, opts)
	case key != "":
		return render.ValueDistribution(os.Stdout, set, key, opts)
	default:
		opts.Tip = tuiTip(configFlags, field, resourceArg)
		return render.KeySummary(os.Stdout, set, opts)
	}
}

// findResource matches a KIND/NAME arg against the set, tolerating case and
// the plural "s" users habitually type ("nodes/node-1" matches kind Node).
func findResource(set analysis.Set, arg string) (*analysis.Resource, bool) {
	parts := strings.SplitN(arg, "/", 2)
	if len(parts) != 2 {
		return nil, false
	}
	kind, name := strings.ToLower(parts[0]), parts[1]
	for i := range set.Resources {
		r := &set.Resources[i]
		k := strings.ToLower(r.Kind)
		if (k == kind || k+"s" == kind) && r.Name == name {
			return r, true
		}
	}
	return nil, false
}

// setLabel names the resource set for summary lines, preferring the type
// as the user typed it ("nodes", "pods").
func setLabel(resourceArg string, set analysis.Set) string {
	if resourceArg != "" && !strings.Contains(resourceArg, "/") {
		return strings.ToLower(resourceArg)
	}
	kind := ""
	for _, r := range set.Resources {
		if kind == "" {
			kind = r.Kind
		} else if kind != r.Kind {
			return "resources"
		}
	}
	if kind == "" {
		return "resources"
	}
	kind = strings.ToLower(kind)
	if !strings.HasSuffix(kind, "s") {
		kind += "s"
	}
	return kind
}

// tuiTip suggests the equivalent interactive command, only on interactive
// terminals and never when output is machine-readable.
func tuiTip(configFlags *genericclioptions.ConfigFlags, field analysis.Field, resourceArg string) string {
	if os.Getenv("KUBECTL_DRILL_NO_TIPS") != "" || outputFormat != "" || varyFlag {
		return ""
	}
	if !term.IsTerminal(int(os.Stdout.Fd())) || resourceArg == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("kubectl drill " + field.String() + " " + resourceArg)
	if allNamespaces {
		b.WriteString(" -A")
	} else if ns := ptr.Deref(configFlags.Namespace, ""); ns != "" {
		b.WriteString(" -n " + ns)
	}
	if selectorOpt != "" {
		fmt.Fprintf(&b, " -l %q", selectorOpt)
	}
	return b.String()
}
