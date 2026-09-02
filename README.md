# kubectl labels

[![ci](https://github.com/ahmetb/kubectl-labels/actions/workflows/ci.yml/badge.svg)](https://github.com/ahmetb/kubectl-labels/actions/workflows/ci.yml)
[![krew](https://img.shields.io/badge/krew-labels-blue)](https://krew.sigs.k8s.io/)

A kubectl plugin to explore and pivot Kubernetes resource labels — built for
resources that carry *many* labels, where `kubectl get --show-labels` becomes
an unreadable wall of text.

It answers three questions about any set of resources:

- **Which label keys exist?** With what coverage and cardinality?
- **How is a key distributed?** Which values exist, on how many resources?
- **What makes this resource different?** Which of its labels actually vary
  across the set?

## Why

Node-feature-discovery, Karpenter, and operators can put 50–100+ labels on a
single object. `kubectl get nodes --show-labels` prints them as one
comma-joined blob per line — you can't see which keys exist, which values
matter, or which nodes differ. `kubectl labels` turns that blob into a pivot
table you can drill into.

## Usage

Query resources with the classic `kubectl get` selectors, then pivot:

```console
# Which label keys exist on nodes, and how much do they vary?
kubectl labels nodes

# How is a label distributed across the set?
kubectl labels nodes topology.kubernetes.io/zone

# Labels of a single resource, one per line
kubectl labels nodes/node-1

# What makes THIS node different from its peers?
kubectl labels nodes/node-1 --vary

# What differs among these pods?
kubectl labels pods -n prod -l app=web --vary

# Explore interactively (TUI)
kubectl labels browse nodes
```

Selecting resources works exactly like `kubectl get`: `TYPE`, `TYPE/NAME`,
`-l/--selector`, `-n/--namespace`, `-A/--all-namespaces`, and `-f/--filename`
(manifests are read fully offline; `-` reads stdin).

### Key summary

```console
$ kubectl labels nodes
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

Labels identical on every resource are hidden by default (`--all` shows them;
`--vary` prints only distinctive ones, with no footers, for scripting). Keys
unique per resource are tagged `(identity)` — they can't group anything.
`--sort-by=name|coverage|cardinality` and `--group-prefix` control the view.

### Value distribution

```console
$ kubectl labels nodes example.com/pool
example.com/pool · present on 3/4 nodes · 2 distinct values

VALUE  COUNT  DISTRIBUTION
cpu    2      ████████████████████████
arm    1      ████████████
missing on 1: node-3
```

### Interactive browser

`kubectl labels browse <type>` (or `-i` on any query) opens a Miller-columns
TUI: drill **prefix → key → value → resources**, filter with `/`, toggle
distinctive-only with `v`, and copy the implied `-l key=value` selector with
`c` to reuse in `kubectl get`.

## Output formats

Add `-o json` or `-o yaml` to any view for scripting:

```console
$ kubectl labels nodes -o json | jq '.keys[] | select(.cardinality > 10)'
```

## Tips

On interactive terminals the summary view suggests the equivalent `browse`
command. Set `KUBECTL_LABELS_NO_TIPS=1` to disable.

## Installation

Install using [Krew](https://krew.sigs.k8s.io/):

```shell
kubectl krew install labels
```

Or download the binary from the **Releases** page and move it somewhere on
your `PATH` as `kubectl-labels`.

## Development

```shell
go test ./...    # unit + integration tests (no cluster needed)
hack/e2e.sh      # end-to-end CLI harness against testdata/
vhs demo.tape    # record img/demo.gif (requires vhs)
```

## License

Copyright 2026 Baseten Labs, Inc.

This project is distributed under [Apache 2.0 License](./LICENSE).
