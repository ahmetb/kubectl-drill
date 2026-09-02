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

// Package analysis computes label statistics over a set of resources.
// It is deliberately free of any Kubernetes client dependencies so it
// can be unit-tested with plain data.
package analysis

import (
	"sort"
	"strings"
)

// Resource is a single Kubernetes object reduced to what this tool needs.
type Resource struct {
	Kind      string
	Namespace string
	Name      string
	Labels    map[string]string
}

// String returns "namespace/name" for namespaced resources, else "name".
func (r Resource) String() string {
	if r.Namespace != "" {
		return r.Namespace + "/" + r.Name
	}
	return r.Name
}

// Set is the queried collection of resources being analyzed.
type Set struct {
	Resources []Resource
}

// Total returns the number of resources in the set.
func (s Set) Total() int { return len(s.Resources) }

// ValueCount pairs a label value with how many resources carry it.
type ValueCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// KeyStat describes one label key across the whole set.
type KeyStat struct {
	Key         string       `json:"key"`
	Coverage    int          `json:"coverage"`    // resources carrying this key
	Cardinality int          `json:"cardinality"` // distinct values
	Values      []ValueCount `json:"values"`      // sorted: count desc, value asc
}

// Uniform reports whether every resource in the set carries this key with
// the same value, i.e. the key distinguishes nothing.
func (k KeyStat) Uniform(total int) bool {
	return total > 0 && k.Coverage == total && k.Cardinality == 1
}

// Distinctive reports whether the key tells resources apart: either its
// value varies, or it is absent from some resources.
func (k KeyStat) Distinctive(total int) bool {
	return !k.Uniform(total)
}

// Identity reports whether every resource has this key with a unique value
// (e.g. kubernetes.io/hostname), making it useless for grouping.
func (k KeyStat) Identity(total int) bool {
	return total > 1 && k.Coverage == total && k.Cardinality == total
}

// Samples returns up to n example values, most frequent first.
func (k KeyStat) Samples(n int) []string {
	var out []string
	for i, v := range k.Values {
		if i >= n {
			break
		}
		out = append(out, v.Value)
	}
	return out
}

// Prefix returns the DNS-prefix portion of a label key
// ("feature.node.kubernetes.io/cpu-x" -> "feature.node.kubernetes.io"),
// or "" for unprefixed keys like "app".
func Prefix(key string) string {
	if i := strings.Index(key, "/"); i >= 0 {
		return key[:i]
	}
	return ""
}

// ShortKey returns the key with its prefix stripped ("cpu-x" above).
func ShortKey(key string) string {
	if i := strings.Index(key, "/"); i >= 0 {
		return key[i+1:]
	}
	return key
}

// KeyStats computes statistics for every label key in the set.
func (s Set) KeyStats() []KeyStat {
	type valCounts map[string]int
	byKey := make(map[string]valCounts)
	coverage := make(map[string]int)
	for _, r := range s.Resources {
		for k, v := range r.Labels {
			if byKey[k] == nil {
				byKey[k] = make(valCounts)
			}
			byKey[k][v]++
			coverage[k]++
		}
	}
	out := make([]KeyStat, 0, len(byKey))
	for k, vc := range byKey {
		vals := make([]ValueCount, 0, len(vc))
		for v, c := range vc {
			vals = append(vals, ValueCount{Value: v, Count: c})
		}
		sort.Slice(vals, func(i, j int) bool {
			if vals[i].Count != vals[j].Count {
				return vals[i].Count > vals[j].Count
			}
			return vals[i].Value < vals[j].Value
		})
		out = append(out, KeyStat{
			Key:         k,
			Coverage:    coverage[k],
			Cardinality: len(vals),
			Values:      vals,
		})
	}
	return out
}

// SortKeyStats sorts stats in place. sortBy is one of "", "name",
// "coverage", "cardinality". The default orders by cardinality desc,
// coverage desc, key asc.
func SortKeyStats(stats []KeyStat, sortBy string) {
	sort.Slice(stats, func(i, j int) bool {
		a, b := stats[i], stats[j]
		switch sortBy {
		case "name":
			return a.Key < b.Key
		case "coverage":
			if a.Coverage != b.Coverage {
				return a.Coverage > b.Coverage
			}
		default: // "cardinality"
			if a.Cardinality != b.Cardinality {
				return a.Cardinality > b.Cardinality
			}
			if a.Coverage != b.Coverage {
				return a.Coverage > b.Coverage
			}
		}
		return a.Key < b.Key
	})
}

// Distinctive filters stats to keys that distinguish resources in the set.
func Distinctive(stats []KeyStat, total int) []KeyStat {
	var out []KeyStat
	for _, k := range stats {
		if k.Distinctive(total) {
			out = append(out, k)
		}
	}
	return out
}

// Uniform filters stats to keys identical across the whole set.
func Uniform(stats []KeyStat, total int) []KeyStat {
	var out []KeyStat
	for _, k := range stats {
		if k.Uniform(total) {
			out = append(out, k)
		}
	}
	return out
}

// PrefixGroup is a set of label keys sharing a DNS prefix.
type PrefixGroup struct {
	Prefix string    `json:"prefix"` // "" means unprefixed keys
	Keys   []KeyStat `json:"keys"`
}

// GroupByPrefix buckets stats by label prefix. Groups are sorted by prefix
// with the unprefixed group last; keys within a group keep their order.
func GroupByPrefix(stats []KeyStat) []PrefixGroup {
	var order []string
	byPrefix := make(map[string][]KeyStat)
	for _, k := range stats {
		p := Prefix(k.Key)
		if _, seen := byPrefix[p]; !seen {
			order = append(order, p)
		}
		byPrefix[p] = append(byPrefix[p], k)
	}
	sort.Strings(order)
	// move "" (unprefixed) to the end
	for i, p := range order {
		if p == "" {
			order = append(order[:i], order[i+1:]...)
			order = append(order, p)
			break
		}
	}
	out := make([]PrefixGroup, 0, len(order))
	for _, p := range order {
		out = append(out, PrefixGroup{Prefix: p, Keys: byPrefix[p]})
	}
	return out
}

// ValueStat describes one value of a label key.
type ValueStat struct {
	Value     string     `json:"value"`
	Count     int        `json:"count"`
	Resources []Resource `json:"resources,omitempty"`
}

// ValueStats is the distribution of a single label key across the set.
type ValueStats struct {
	Key     string      `json:"key"`
	Present int         `json:"present"` // resources carrying the key
	Values  []ValueStat `json:"values"`  // sorted: count desc, value asc
	Missing []Resource  `json:"missing,omitempty"`
}

// ValueStats computes the value distribution for key. includeResources
// controls whether per-value resource lists are populated.
func (s Set) ValueStats(key string, includeResources bool) ValueStats {
	byValue := make(map[string][]Resource)
	var missing []Resource
	for _, r := range s.Resources {
		if v, ok := r.Labels[key]; ok {
			byValue[v] = append(byValue[v], r)
		} else {
			missing = append(missing, r)
		}
	}
	vs := ValueStats{Key: key, Present: s.Total() - len(missing), Missing: missing}
	for v, rs := range byValue {
		st := ValueStat{Value: v, Count: len(rs)}
		if includeResources {
			st.Resources = rs
		}
		vs.Values = append(vs.Values, st)
	}
	sort.Slice(vs.Values, func(i, j int) bool {
		if vs.Values[i].Count != vs.Values[j].Count {
			return vs.Values[i].Count > vs.Values[j].Count
		}
		return vs.Values[i].Value < vs.Values[j].Value
	})
	return vs
}
