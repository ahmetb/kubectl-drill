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

package analysis

import (
	"reflect"
	"testing"
)

func res(ns, name string, labels ...string) Resource {
	m := make(map[string]string)
	for i := 0; i < len(labels); i += 2 {
		m[labels[i]] = labels[i+1]
	}
	return Resource{Kind: "Pod", Namespace: ns, Name: name, Labels: m}
}

func TestResourceString(t *testing.T) {
	if got := res("prod", "web-1").String(); got != "prod/web-1" {
		t.Errorf("namespaced: got %q", got)
	}
	if got := res("", "node-1").String(); got != "node-1" {
		t.Errorf("cluster-scoped: got %q", got)
	}
}

func TestKeyStatsEmpty(t *testing.T) {
	s := Set{}
	if got := s.KeyStats(); len(got) != 0 {
		t.Errorf("expected no stats, got %d", len(got))
	}
}

func TestKeyStatsUnlabeledResources(t *testing.T) {
	s := Set{Resources: []Resource{res("", "a"), res("", "b")}}
	if got := s.KeyStats(); len(got) != 0 {
		t.Errorf("expected no stats for unlabeled resources, got %d", len(got))
	}
}

func TestKeyStatsBasic(t *testing.T) {
	s := Set{Resources: []Resource{
		res("", "a", "app", "web", "zone", "us1"),
		res("", "b", "app", "web", "zone", "us1"),
		res("", "c", "app", "api", "zone", "us2"),
		res("", "d", "app", "web"), // no zone key
	}}
	stats := s.KeyStats()
	if len(stats) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(stats))
	}
	byKey := map[string]KeyStat{}
	for _, k := range stats {
		byKey[k.Key] = k
	}

	app := byKey["app"]
	if app.Coverage != 4 || app.Cardinality != 2 {
		t.Errorf("app: coverage=%d cardinality=%d, want 4/2", app.Coverage, app.Cardinality)
	}
	// values sorted count desc, then value asc
	wantVals := []ValueCount{{Value: "web", Count: 3}, {Value: "api", Count: 1}}
	if !reflect.DeepEqual(app.Values, wantVals) {
		t.Errorf("app values = %+v, want %+v", app.Values, wantVals)
	}

	zone := byKey["zone"]
	if zone.Coverage != 3 || zone.Cardinality != 2 {
		t.Errorf("zone: coverage=%d cardinality=%d, want 3/2", zone.Coverage, zone.Cardinality)
	}
}

func TestKeyStatClassification(t *testing.T) {
	total := 4
	uniform := KeyStat{Key: "u", Coverage: 4, Cardinality: 1}
	partial := KeyStat{Key: "p", Coverage: 3, Cardinality: 1}
	varying := KeyStat{Key: "v", Coverage: 4, Cardinality: 2}
	identity := KeyStat{Key: "i", Coverage: 4, Cardinality: 4}

	if !uniform.Uniform(total) || uniform.Distinctive(total) {
		t.Error("uniform key misclassified")
	}
	if partial.Uniform(total) || !partial.Distinctive(total) {
		t.Error("partial-coverage key should be distinctive, not uniform")
	}
	if varying.Uniform(total) || !varying.Distinctive(total) {
		t.Error("varying key should be distinctive")
	}
	if !identity.Identity(total) {
		t.Error("identity key not detected")
	}
	if uniform.Identity(total) || partial.Identity(total) {
		t.Error("non-identity keys flagged as identity")
	}
	// identity is meaningless for a single-resource set
	if (KeyStat{Coverage: 1, Cardinality: 1}).Identity(1) {
		t.Error("single-resource set must not report identity")
	}
}

func TestSortKeyStats(t *testing.T) {
	stats := []KeyStat{
		{Key: "b", Coverage: 2, Cardinality: 1},
		{Key: "a", Coverage: 4, Cardinality: 3},
		{Key: "c", Coverage: 4, Cardinality: 2},
	}
	SortKeyStats(stats, "")
	if stats[0].Key != "a" || stats[1].Key != "c" || stats[2].Key != "b" {
		t.Errorf("default sort: got %s,%s,%s", stats[0].Key, stats[1].Key, stats[2].Key)
	}
	SortKeyStats(stats, "name")
	if stats[0].Key != "a" || stats[1].Key != "b" || stats[2].Key != "c" {
		t.Errorf("name sort: got %s,%s,%s", stats[0].Key, stats[1].Key, stats[2].Key)
	}
	SortKeyStats(stats, "coverage")
	if stats[0].Coverage != 4 || stats[2].Coverage != 2 {
		t.Errorf("coverage sort: got %d first, %d last", stats[0].Coverage, stats[2].Coverage)
	}
}

func TestPrefixAndShortKey(t *testing.T) {
	cases := []struct{ key, prefix, short string }{
		{"feature.node.kubernetes.io/cpu-x", "feature.node.kubernetes.io", "cpu-x"},
		{"app", "", "app"},
		{"kubernetes.io/hostname", "kubernetes.io", "hostname"},
	}
	for _, c := range cases {
		if got := Prefix(c.key); got != c.prefix {
			t.Errorf("Prefix(%q) = %q, want %q", c.key, got, c.prefix)
		}
		if got := ShortKey(c.key); got != c.short {
			t.Errorf("ShortKey(%q) = %q, want %q", c.key, got, c.short)
		}
	}
}

func TestGroupByPrefix(t *testing.T) {
	stats := []KeyStat{
		{Key: "app"},
		{Key: "b.io/x"},
		{Key: "a.io/y"},
		{Key: "a.io/z"},
	}
	groups := GroupByPrefix(stats)
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	// sorted by prefix, unprefixed last
	if groups[0].Prefix != "a.io" || groups[1].Prefix != "b.io" || groups[2].Prefix != "" {
		t.Errorf("group order: %q %q %q", groups[0].Prefix, groups[1].Prefix, groups[2].Prefix)
	}
	if len(groups[0].Keys) != 2 {
		t.Errorf("a.io should have 2 keys, got %d", len(groups[0].Keys))
	}
}

func TestValueStats(t *testing.T) {
	s := Set{Resources: []Resource{
		res("p", "a", "zone", "us1"),
		res("p", "b", "zone", "us1"),
		res("p", "c", "zone", "us2"),
		res("p", "d"),
	}}
	vs := s.ValueStats("zone", true)
	if vs.Present != 3 {
		t.Errorf("Present = %d, want 3", vs.Present)
	}
	if len(vs.Values) != 2 || vs.Values[0].Value != "us1" || vs.Values[0].Count != 2 {
		t.Errorf("unexpected values: %+v", vs.Values)
	}
	if len(vs.Values[0].Resources) != 2 {
		t.Errorf("expected resource names for us1")
	}
	if len(vs.Missing) != 1 || vs.Missing[0].Name != "d" {
		t.Errorf("missing = %+v, want [d]", vs.Missing)
	}

	vsNoRes := s.ValueStats("zone", false)
	if vsNoRes.Values[0].Resources != nil {
		t.Error("includeResources=false must not populate Resources")
	}

	absent := s.ValueStats("nonexistent", true)
	if absent.Present != 0 || len(absent.Values) != 0 || len(absent.Missing) != 4 {
		t.Errorf("absent key: %+v", absent)
	}
}

func TestDistinctiveAndUniformFilters(t *testing.T) {
	stats := []KeyStat{
		{Key: "u", Coverage: 3, Cardinality: 1},
		{Key: "v", Coverage: 3, Cardinality: 2},
		{Key: "p", Coverage: 1, Cardinality: 1},
	}
	if got := Distinctive(stats, 3); len(got) != 2 {
		t.Errorf("distinctive: got %d, want 2", len(got))
	}
	if got := Uniform(stats, 3); len(got) != 1 || got[0].Key != "u" {
		t.Errorf("uniform: got %+v", got)
	}
}
