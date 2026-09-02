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

// Package render prints label statistics as human-friendly tables.
package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"golang.org/x/term"
	"sigs.k8s.io/yaml"

	"github.com/ahmetb/kubectl-labels/internal/analysis"
)

var (
	bold   = color.New(color.Bold)
	gray   = color.New(color.FgHiBlack)
	yellow = color.New(color.FgYellow)
	green  = color.New(color.FgGreen)
	cyan   = color.New(color.FgCyan)
)

const (
	maxSamples      = 3
	maxSampleLen    = 48
	maxSamplesWidth = 56 // total width of the joined samples column
	barWidth        = 24
	maxValueRows    = 50 // value rows shown before truncating (—all lifts the cap)
)

// Options controls rendering of all views.
type Options struct {
	Vary        bool   // distinctive keys only, no footers (script-friendly)
	All         bool   // include uniform keys
	SortBy      string // "", "name", "coverage", "cardinality"
	GroupPrefix bool   // group keys by DNS prefix
	Output      string // "", "json", "yaml"
	SetLabel    string // plural label for the resource set, e.g. "nodes"
	Tip         string // browse command to suggest; "" disables the tip
}

// CountNoun returns the singular form of the plural set noun when n is 1
// ("nodes" -> "node"), so headers never read "1 nodes".
func CountNoun(n int, plural string) string {
	if n == 1 && strings.HasSuffix(plural, "s") {
		return strings.TrimSuffix(plural, "s")
	}
	return plural
}

// KeySummary prints the pivot-by-key view.
func KeySummary(w io.Writer, set analysis.Set, opts Options) error {
	total := set.Total()
	stats := set.KeyStats()
	analysis.SortKeyStats(stats, opts.SortBy)

	if opts.Output == "json" || opts.Output == "yaml" {
		return marshal(w, opts.Output, map[string]any{
			"resources": total,
			"keys":      stats,
		})
	}

	distinctive := analysis.Distinctive(stats, total)
	uniformCount := len(stats) - len(distinctive)

	fmt.Fprintf(w, "%s · %s · %s\n",
		bold.Sprintf("%d %s", total, CountNoun(total, opts.SetLabel)),
		gray.Sprintf("%d distinct keys", len(stats)),
		gray.Sprintf("%d distinctive", len(distinctive)))

	if len(stats) == 0 {
		fmt.Fprintln(w, gray.Sprintf("none of the %d %s carry any labels", total, CountNoun(total, opts.SetLabel)))
		return nil
	}

	show := distinctive
	if opts.All {
		show = stats
	}
	if len(show) == 0 {
		fmt.Fprintln(w, gray.Sprint("all labels are identical across the set (--all to show them)"))
		return nil
	}

	// Size the samples column to the terminal so rows never wrap; fall back
	// to a fixed cap when not a TTY (pipes, tests).
	keyWidth := 0
	for _, k := range show {
		n := len([]rune(k.Key))
		if k.Identity(total) {
			n += len("  (identity)")
		}
		if n > keyWidth {
			keyWidth = n
		}
	}
	samplesWidth := maxSamplesWidth
	if tw := termWidth(w); tw > 0 {
		// key + coverage + values columns, separators, breathing room
		if b := tw - keyWidth - 24; b < samplesWidth {
			samplesWidth = max(b, 12)
		}
	}

	table, buf := newTable([]string{"KEY", "COVERAGE", "VALUES", "SAMPLE VALUES"})
	appendKey := func(k analysis.KeyStat, indent bool) {
		key := k.Key
		if indent {
			key = "  " + analysis.ShortKey(k.Key)
		}
		if k.Identity(total) {
			key += gray.Sprint("  (identity)")
		}
		cov := green.Sprintf("%d/%d", k.Coverage, total)
		if k.Coverage != total {
			cov = yellow.Sprintf("%d/%d", k.Coverage, total)
		}
		if k.Uniform(total) && opts.All {
			key = gray.Sprint(key)
		}
		table.Append([]string{key, cov, fmt.Sprint(k.Cardinality), samples(k, samplesWidth)})
	}

	if opts.GroupPrefix {
		for _, g := range analysis.GroupByPrefix(show) {
			if g.Prefix != "" {
				label := fmt.Sprintf("%d keys", len(g.Keys))
				if len(g.Keys) == 1 {
					label = "1 key"
				}
				table.Append([]string{gray.Sprintf("%s/ (%s)", g.Prefix, label), "", "", ""})
			}
			for _, k := range g.Keys {
				appendKey(k, g.Prefix != "")
			}
		}
	} else {
		for _, k := range show {
			appendKey(k, false)
		}
	}
	flushTable(w, table, buf)

	if !opts.All && !opts.Vary && uniformCount > 0 {
		noun := "keys"
		if uniformCount == 1 {
			noun = "key"
		}
		fmt.Fprintf(w, "%s\n", gray.Sprintf("… %d uniform %s hidden · use --all to show", uniformCount, noun))
	}
	if opts.Tip != "" && total >= 2 && len(distinctive) >= 2 {
		fmt.Fprintf(w, "%s\n", gray.Sprintf("tip: explore interactively with `%s`", opts.Tip))
	}
	return nil
}

// ValueDistribution prints the pivot-by-value view for a single key.
func ValueDistribution(w io.Writer, set analysis.Set, key string, opts Options) error {
	vs := set.ValueStats(key, true)

	if opts.Output == "json" || opts.Output == "yaml" {
		return marshal(w, opts.Output, vs)
	}

	total := set.Total()
	fmt.Fprintf(w, "%s · present on %s · %s\n",
		bold.Sprint(key),
		gray.Sprintf("%d/%d %s", vs.Present, total, opts.SetLabel),
		gray.Sprintf("%d distinct values", len(vs.Values)))

	if vs.Present == 0 {
		fmt.Fprintln(w, yellow.Sprintf("no %s carry this label key", opts.SetLabel))
		return nil
	}

	max := 0
	for _, v := range vs.Values {
		if v.Count > max {
			max = v.Count
		}
	}
	table, buf := newTable([]string{"VALUE", "COUNT", "DISTRIBUTION"})
	rows := vs.Values
	if !opts.All && len(rows) > maxValueRows {
		rows = rows[:maxValueRows]
	}
	for _, v := range rows {
		n := v.Count * barWidth / max
		if n == 0 {
			n = 1
		}
		table.Append([]string{
			v.Value,
			bold.Sprintf("%d", v.Count),
			cyan.Sprint(strings.Repeat("█", n)),
		})
	}
	flushTable(w, table, buf)

	if rest := len(vs.Values) - len(rows); rest > 0 {
		fmt.Fprintf(w, "%s\n", gray.Sprintf("… %d more values hidden · use --all to show", rest))
	}

	if len(vs.Missing) > 0 {
		names := resourceNames(vs.Missing, 5)
		fmt.Fprintf(w, "%s\n", gray.Sprintf("missing on %d: %s", len(vs.Missing), names))
	}
	return nil
}

// ResourceList prints the pivot-by-resource view: one label per line,
// grouped under each resource. view holds the resources to display; stats
// is the set used to compute which labels are distinctive (the two differ
// for "TYPE/NAME --vary", where a single resource is compared against its
// peers).
func ResourceList(w io.Writer, view, stats analysis.Set, opts Options) error {
	resources := append([]analysis.Resource(nil), view.Resources...)
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Namespace != resources[j].Namespace {
			return resources[i].Namespace < resources[j].Namespace
		}
		return resources[i].Name < resources[j].Name
	})

	if opts.Output == "json" || opts.Output == "yaml" {
		return marshal(w, opts.Output, resources)
	}

	var distinctiveKeys map[string]bool
	if opts.Vary {
		distinctiveKeys = make(map[string]bool)
		for _, k := range analysis.Distinctive(stats.KeyStats(), stats.Total()) {
			distinctiveKeys[k.Key] = true
		}
	}

	for i, r := range resources {
		if i > 0 {
			fmt.Fprintln(w)
		}
		header := bold.Sprint(r.Kind) + " " + bold.Sprint(r.String())
		fmt.Fprintln(w, header)

		keys := make([]string, 0, len(r.Labels))
		for k := range r.Labels {
			if distinctiveKeys != nil && !distinctiveKeys[k] {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			fmt.Fprintln(w, gray.Sprint("  (no labels)"))
			continue
		}
		for _, k := range keys {
			fmt.Fprintf(w, "  %s=%s\n", gray.Sprint(k), r.Labels[k])
		}
	}
	return nil
}

// newTable builds a table that renders into an internal buffer; call
// flushTable to write it out. Rows are right-trimmed on flush so padded
// cells never emit trailing whitespace (which wraps ugly in terminals
// narrower than the table).
func newTable(header []string) (*tablewriter.Table, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	table := tablewriter.NewWriter(buf)
	table.SetHeader(header)
	table.SetBorder(false)
	table.SetHeaderLine(false)
	table.SetColumnSeparator("")
	table.SetAutoWrapText(false)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	return table, buf
}

func flushTable(w io.Writer, table *tablewriter.Table, buf *bytes.Buffer) {
	table.Render()
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}
}

func samples(k analysis.KeyStat, width int) string {
	parts := k.Samples(maxSamples)
	for i, p := range parts {
		if p == "" {
			p = `""`
		}
		parts[i] = truncate(p, maxSampleLen)
	}
	s := strings.Join(parts, ", ")
	if rest := k.Cardinality - len(parts); rest > 0 {
		s += fmt.Sprintf(", +%d more", rest)
	}
	return truncate(s, width)
}

// termWidth returns the width of the terminal backing w, or 0 if w is not
// a terminal.
func termWidth(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok {
		return 0
	}
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	return width
}

func resourceNames(rs []analysis.Resource, limit int) string {
	var names []string
	for i, r := range rs {
		if i >= limit {
			names = append(names, fmt.Sprintf("+%d more", len(rs)-limit))
			break
		}
		names = append(names, r.String())
	}
	return strings.Join(names, ", ")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func marshal(w io.Writer, format string, v any) error {
	var (
		b   []byte
		err error
	)
	if format == "yaml" {
		b, err = yaml.Marshal(v)
	} else {
		b, err = json.MarshalIndent(v, "", "  ")
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}
