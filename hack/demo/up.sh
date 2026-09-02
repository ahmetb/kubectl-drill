#!/usr/bin/env bash
# Copyright 2026 Baseten Labs, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Spin up the demo environment: a kind cluster populated with fake,
# label-heavy nodes. Uses a dedicated kubeconfig so your current
# context is never touched.
#
#   hack/demo/up.sh    # create cluster + nodes
#   hack/demo/down.sh  # delete everything
set -euo pipefail
cd "$(dirname "$0")/../.."

CLUSTER=labels-demo
KCFG=hack/demo/kubeconfig

if ! kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
  echo "creating kind cluster $CLUSTER..."
  kind create cluster --name "$CLUSTER" --kubeconfig "$KCFG"
else
  kind export kubeconfig --name "$CLUSTER" --kubeconfig "$KCFG"
fi

echo "applying fake nodes..."
hack/demo/gen-nodes.py | KUBECONFIG="$KCFG" kubectl apply -f - >/dev/null

# mark fake nodes Ready so "kubectl get nodes" looks real in the demo
hack/demo/ready.sh

# blend the kind control-plane node into the fleet: give it the common
# label set (as an ARM node on the newest rollout) so coverage patterns
# stay meaningful; it keeps its node-role.kubernetes.io/control-plane label
KUBECONFIG="$KCFG" kubectl label --overwrite node "$CLUSTER-control-plane" \
  kubernetes.io/os=linux \
  topology.kubernetes.io/region=eu-central-1 \
  topology.kubernetes.io/zone=eu-central-1a \
  node-init.example.com/ready=true \
  node-init.example.com/last-run-at=2026-08-30T22-45-00Z \
  node-init.example.com/digest=e77c1a90bd23 \
  node-init.example.com/version=0.10.0 \
  feature.node.kubernetes.io/cpu-model.vendor_id=ARM \
  feature.node.kubernetes.io/kernel-version.major=6 \
  feature.node.kubernetes.io/kernel-version.minor=10 \
  feature.node.kubernetes.io/system-os_release.ID=ubuntu \
  feature.node.kubernetes.io/system-os_release.VERSION_ID=24.04 \
  feature.node.kubernetes.io/storage-nonrotationaldisk=true \
  feature.node.kubernetes.io/cpu-cpuid.ASIMD=true \
  feature.node.kubernetes.io/cpu-cpuid.ASIMDDP=true \
  feature.node.kubernetes.io/cpu-cpuid.AES=true \
  feature.node.kubernetes.io/cpu-cpuid.SHA1=true \
  feature.node.kubernetes.io/cpu-cpuid.CRC32=true >/dev/null

echo
echo "demo cluster ready. use it with:"
echo "  export KUBECONFIG=$PWD/$KCFG"
echo "  kubectl labels nodes"
