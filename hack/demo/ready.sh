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

# Mark the fake demo nodes Ready. kind's node lifecycle controller flips
# kubelet-less nodes to NotReady after ~40s, so this is re-run (hidden)
# at the start of demo.tape as well.
set -euo pipefail
cd "$(dirname "$0")/../.."

KCFG=hack/demo/kubeconfig
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
for n in $(KUBECONFIG="$KCFG" kubectl get nodes -o jsonpath='{.items[*].metadata.name}'); do
  KUBECONFIG="$KCFG" kubectl patch node "$n" --type=json --subresource status \
    -p="[{\"op\":\"add\",\"path\":\"/status/conditions\",\"value\":[{\"type\":\"Ready\",\"status\":\"True\",\"reason\":\"KubeletReady\",\"lastHeartbeatTime\":\"$NOW\",\"lastTransitionTime\":\"$NOW\"}]}]" \
    >/dev/null 2>&1 || true
done
