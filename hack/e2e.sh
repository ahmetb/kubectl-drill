#!/usr/bin/env bash
# e2e harness: exercises the built binary against static testdata.
# Usage: hack/e2e.sh   (builds ./kubectl-labels first)
set -u

cd "$(dirname "$0")/.."
BIN=./kubectl-labels
FAILURES=0

go build -o "$BIN" . || { echo "BUILD FAILED"; exit 1; }

check() { # name, expected-substring, command...
  local name="$1" want="$2"; shift 2
  local out
  out=$("$@" 2>&1)
  if [[ "$out" == *"$want"* ]]; then
    echo "ok   $name"
  else
    echo "FAIL $name: expected substring '$want'"
    echo "--- output ---"; echo "$out" | head -15; echo "--------------"
    FAILURES=$((FAILURES+1))
  fi
}

check_fails() { # name, command...
  local name="$1"; shift
  if "$@" >/dev/null 2>&1; then
    echo "FAIL $name: expected non-zero exit"
    FAILURES=$((FAILURES+1))
  else
    echo "ok   $name"
  fi
}

F="-f testdata/nodes.json"

check "keys summary header"   "4 nodes · 10 distinct keys · 9 distinctive" $BIN $F
check "identity detection"    "kubernetes.io/hostname  (identity)"         $BIN $F
check "uniform hidden"        "1 uniform key hidden"                      $BIN $F
check "all shows uniform"     "node-init.example.com/ready"                $BIN $F --all
check "group prefix"          "feature.node.kubernetes.io/ (3 keys)"       $BIN $F --group-prefix
check "sort by name"          "example.com/pool"                           $BIN $F --sort-by=name
check "values distribution"   "present on 3/4 nodes · 2 distinct values"   $BIN $F example.com/pool
check "values missing"        "missing on 1: node-3"                       $BIN $F example.com/pool
check "absent key"            "no nodes carry this label key"              $BIN $F does.not/exist
check "single resource list"  "kubernetes.io/arch=arm64"                   $BIN -f testdata/nodes.json nodes/node-2
check "vary vs peers"         "kubernetes.io/arch=arm64"                   $BIN $F nodes/node-2 --vary
check "vary hides uniform"    "node-2"                                     $BIN $F nodes/node-2 --vary
check "json output"           '"resources": 4'                             $BIN $F -o json
check "yaml output"           "cardinality:"                               $BIN $F -o yaml
check "keys alias"            "10 distinct keys"                           $BIN keys $F
check "values alias"          "2 distinct values"                          $BIN values $F example.com/pool
check "list alias"            "kubernetes.io/hostname=node-0"              $BIN list $F
check "no tips when piped"    "distinctive"                                $BIN $F
check "stdin input"           "4 nodes · 10 distinct keys"                 bash -c "cat testdata/nodes.json | $BIN -f -"
check "selector in file mode" "2 nodes · 10 distinct keys"                 $BIN $F -l kubernetes.io/arch=amd64
check "bad selector"          "invalid selector"                           $BIN $F -l '==bogus=='
check "multi-doc yaml"        "2 pods"                                     $BIN -f testdata/pods.yaml

# negative cases
check_fails "no args errors"        $BIN
check_fails "too many args"         $BIN $F extra1 extra2 extra3

# --vary output must NOT contain the uniform key
out=$($BIN $F nodes/node-2 --vary 2>&1)
if [[ "$out" == *"node-init.example.com/ready"* ]]; then
  echo "FAIL vary excludes uniform key"; FAILURES=$((FAILURES+1))
else
  echo "ok   vary excludes uniform key"
fi

# tip must never appear when piped
out=$($BIN $F 2>&1)
if [[ "$out" == *"tip:"* ]]; then
  echo "FAIL tip shown when piped"; FAILURES=$((FAILURES+1))
else
  echo "ok   tip suppressed when piped"
fi

echo
if [[ $FAILURES -gt 0 ]]; then
  echo "$FAILURES FAILURES"; exit 1
fi
echo "ALL PASS"
