# kubectl drill

[![ci](https://github.com/ahmetb/kubectl-drill/actions/workflows/ci.yml/badge.svg)](https://github.com/ahmetb/kubectl-drill/actions/workflows/ci.yml)
[![krew](https://img.shields.io/badge/krew-drill-blue)](https://krew.sigs.k8s.io/)

A kubectl plugin to drill into Kubernetes resource labels and annotations —
built for resources that carry *many* of them, where
`kubectl get --show-labels` becomes an unreadable wall of text.

It answers three questions about any set of resources:

- **Which keys exist?** With what coverage and cardinality?
- **How is a key distributed?** Which values exist, on how many resources?
- **What makes this resource different?** Which of its labels actually vary
  across the set?

## Why

Node-feature-discovery, Karpenter, and operators can put 50–100+ labels on a
single object. `kubectl get nodes --show-labels` prints them as one
comma-joined blob per line — you can't see which keys exist, which values
matter, or which nodes differ. `kubectl drill` turns that blob into a pivot
table you can drill into.

![demo](img/demo.gif)

## Usage

Jump straight into the interactive drill-down (the default) and explore
**prefix → key → value → resources**:

```console
# Labels (the default field)
kubectl drill nodes
kubectl drill labels nodes

# Annotations instead
kubectl drill annotations pods
```

Prefer static, scriptable tables? Add `--boring` (it's also implied when
you pipe the output or use `-o json`/`-o yaml`):

```console
# Which label keys exist on nodes, and how much do they vary?
kubectl drill labels nodes --boring

# How is a label distributed across the set?
kubectl drill labels nodes --boring topology.kubernetes.io/zone

# Labels of a single resource, one per line
kubectl drill labels nodes/node-1 --boring

# What makes THIS node different from its peers?
kubectl drill labels nodes/node-1 --boring --vary

# What differs among these pods?
kubectl drill labels pods -n prod -l app=web --boring --vary
```

Selecting resources works exactly like `kubectl get`: `TYPE`, `TYPE/NAME`,
`-l/--selector`, `-n/--namespace`, `-A/--all-namespaces`, and `-f/--filename`
(manifests are read fully offline; `-` reads stdin).

### Interactive drill-down

`kubectl drill <type>` (or `kubectl drill labels <type>`) opens a
Miller-columns TUI: drill **prefix → key → value → resources** (arrows,
`hjkl`, `tab`; `esc` steps back), filter any column with `/`, toggle
distinctive-only with `v`, and copy the implied `-l key=value` selector with
`c` to reuse in `kubectl get` (labels only — label selectors can't match
annotations).

It is fully mouse-driven, too:

- **Click** a row to focus its column and select it — the columns to its
  right rebuild instantly, so you can jump straight to a value in one click.
- **Click** a column's title to focus it, and the **scroll wheel** scrolls
  whichever column you hover.
- Click the **magnifier** ("") at the right edge of a column header to
  search within that column (same as typing `/` there).
- An applied column filter shows as a "" chip; click the red **``** button next to it to discard it instantly. `x` clears the focused column's
  filter, `X` clears every filter, and `esc` clears the filter before
  stepping back.

Icons use [Nerd Font](https://www.nerdfonts.com/) glyphs — a Nerd-patched
font (e.g. **Hack Nerd Font**) renders them as icons; other fonts may show
placeholder boxes for the icons while all text stays readable.

### Key summary

```console
$ kubectl drill labels nodes --boring
4 nodes · 10 distinct keys · 9 distinctive

KEY                                              COVERAGE  VALUES  SAMPLE VALUES
kubernetes.io/hostname  (identity)               4/4       4       node-0, node-1, node-2, +1 more
example.com/pool                                 3/4       2       cpu, arm
kubernetes.io/arch                               3/4       2       amd64, arm64
topology.kubernetes.io/zone                      3/4       2       zone-a, zone-b
kubernetes.io/os                                 3/4       1       linux
feature.node.kubernetes.io/cpu-cpuid.AESNI       2/4       1       true
feature.node.kubernetes.io/kernel-version.major  2/4       1       6
node-role.kubernetes.io/worker                   2/4       1       ""
feature.node.kubernetes.io/cpu-cpuid.ADX         1/4       1       true
… 1 uniform key hidden · use --all to show
```

Keys identical on every resource are hidden by default (`--all` shows them;
`--vary` prints only distinctive ones, with no footers, for scripting). Keys
unique per resource are tagged `(identity)` — they can't group anything.
`--sort-by=name|coverage|cardinality` and `--group-prefix` control the view.
Tables adapt to your terminal width; value tables cap at 50 rows (`--all`
shows the rest). Every view works on annotations too: swap `labels` for
`annotations`.

### Value distribution

```console
$ kubectl drill labels nodes --boring example.com/pool
example.com/pool · present on 3/4 nodes · 2 distinct values

VALUE  COUNT  DISTRIBUTION
cpu    2      ████████████████████████
arm    1      ████████████
missing on 1: node-3
```

## Output formats

Add `-o json` or `-o yaml` to any static view for scripting:

```console
$ kubectl drill labels nodes --boring -o json | jq '.keys[] | select(.cardinality > 10)'
```

## Tips

On interactive terminals the `--boring` summary suggests the equivalent
interactive command. Set `KUBECTL_DRILL_NO_TIPS=1` to disable.

## Installation

Install using [Krew](https://krew.sigs.k8s.io/):

```shell
kubectl krew install drill
```

Or download the binary from the **Releases** page and move it somewhere on
your `PATH` as `kubectl-drill`. Verify with `kubectl drill version`.

## Development

```shell
go test ./...     # unit + integration tests (no cluster needed)
hack/e2e.sh       # end-to-end CLI harness against testdata/

# record img/demo.gif (requires vhs, kind, docker):
hack/demo/up.sh   # kind cluster with fake label-heavy nodes
vhs demo.tape
hack/demo/down.sh
```

## License

Copyright 2026 Baseten Labs, Inc.

This project is distributed under [Apache 2.0 License](./LICENSE).
