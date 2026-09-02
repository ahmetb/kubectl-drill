#!/usr/bin/env python3
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

"""Generate fake Node manifests with realistic label sets for the demo.

Deterministic: same output on every run, so the recorded demo is stable.
Prints a JSON List to stdout (pipe to kubectl apply -f -).
"""

import json

REGION = "eu-central-1"
ZONES = ["eu-central-1a", "eu-central-1b", "eu-central-1c"]

# node-init "rollout generations": digest -> version, oldest first
ROLLOUTS = [
    ("9f3a2c71d4ab", "0.9.1"),
    ("4b8e0d52c1f7", "0.9.2"),
    ("e77c1a90bd23", "0.10.0"),
]

INTEL_CPUID = ["ADX", "AESNI", "AVX", "AVX2", "AVX512F", "FMA3", "SHA", "VAES"]
ARM_CPUID = ["ASIMD", "ASIMDDP", "AES", "SHA1", "CRC32"]

# gpu product -> (memory MiB, count)
GPUS = {
    "NVIDIA-L4": ("23040", "1"),
    "NVIDIA-A10G": ("24576", "1"),
    "NVIDIA-T4": ("15360", "1"),
}


def nfd_labels(vendor, cpuid, kernel_minor):
    labels = {
        "feature.node.kubernetes.io/cpu-model.vendor_id": vendor,
        "feature.node.kubernetes.io/kernel-version.major": "6",
        "feature.node.kubernetes.io/kernel-version.minor": str(kernel_minor),
        "feature.node.kubernetes.io/system-os_release.ID": "ubuntu",
        "feature.node.kubernetes.io/system-os_release.VERSION_ID": "24.04",
        "feature.node.kubernetes.io/storage-nonrotationaldisk": "true",
    }
    for f in cpuid:
        labels[f"feature.node.kubernetes.io/cpu-cpuid.{f}"] = "true"
    return labels


def node(name, pool, itype, family, cpu, mem, zone, arch, vendor, cpuid,
         kernel_minor, rollout_idx, minute, capacity="on-demand", extra=None):
    digest, version = ROLLOUTS[rollout_idx]
    labels = {
        "kubernetes.io/hostname": name,
        "kubernetes.io/os": "linux",
        "kubernetes.io/arch": arch,
        "topology.kubernetes.io/region": REGION,
        "topology.kubernetes.io/zone": zone,
        "node.kubernetes.io/instance-type": itype,
        "karpenter.sh/nodepool": pool,
        "karpenter.sh/capacity-type": capacity,
        "karpenter.k8s.aws/instance-family": family,
        "karpenter.k8s.aws/instance-cpu": str(cpu),
        "karpenter.k8s.aws/instance-memory": str(mem),
        "node-init.example.com/ready": "true",
        "node-init.example.com/last-run-at": f"2026-08-30T22-{minute:02d}-00Z",
        "node-init.example.com/digest": digest,
        "node-init.example.com/version": version,
    }
    labels.update(nfd_labels(vendor, cpuid, kernel_minor))
    if extra:
        labels.update(extra)
    return {
        "apiVersion": "v1",
        "kind": "Node",
        "metadata": {"name": name, "labels": labels},
    }


def gpu_labels(product, driver):
    mem, count = GPUS[product]
    return {
        "nvidia.com/gpu.present": "true",
        "nvidia.com/gpu.product": product,
        "nvidia.com/gpu.count": count,
        "nvidia.com/gpu.memory": mem,
        "nvidia.com/gpu-driver-version": driver,
        "feature.node.kubernetes.io/pci-10de.present": "true",
    }


def main():
    nodes = []
    n = 0

    def hostname():
        nonlocal n
        n += 1
        return f"ip-10-24-{10 + n}.ec2.internal"

    # cpu pool: 10 amd64 (mixed Intel/AMD, mixed rollout generations, some spot)
    for i in range(10):
        nodes.append(node(
            hostname(), "cpu", "c7a.2xlarge" if i % 2 else "c6a.2xlarge",
            "c7a" if i % 2 else "c6a", 8, 16384,
            ZONES[i % 3], "amd64",
            "AuthenticAMD" if i % 2 else "GenuineIntel", INTEL_CPUID,
            kernel_minor=8 if i < 4 else 10,
            rollout_idx=min(i // 4, 2), minute=10 + i,
            capacity="spot" if i % 3 == 0 else "on-demand",
        ))
    # cpu pool: 2 arm64 nodes
    for i in range(2):
        nodes.append(node(
            hostname(), "cpu", "c7g.2xlarge", "c7g", 8, 16384,
            ZONES[i % 2], "arm64", "ARM", ARM_CPUID,
            kernel_minor=10, rollout_idx=2, minute=30 + i,
        ))
    # memory pool: 8 nodes with local nvme storage
    for i in range(8):
        nodes.append(node(
            hostname(), "memory", "r7a.2xlarge", "r7a", 8, 65536,
            ZONES[(i + 1) % 3], "amd64", "AuthenticAMD", INTEL_CPUID,
            kernel_minor=10, rollout_idx=2, minute=32 + i,
            extra={"example.com/storage-tier": "local-nvme"},
        ))
    # gpu pool: 6 nodes, mixed products; one on an older driver
    products = ["NVIDIA-L4"] * 3 + ["NVIDIA-A10G"] * 2 + ["NVIDIA-T4"]
    for i, product in enumerate(products):
        nodes.append(node(
            hostname(), "gpu", "g6.2xlarge", "g6", 8, 32768,
            ZONES[i % 3], "amd64", "GenuineIntel", INTEL_CPUID,
            kernel_minor=10, rollout_idx=2, minute=40 + i,
            capacity="spot" if i >= 4 else "on-demand",
            extra=gpu_labels(product, "535.86.10" if i == 5 else "535.104.05"),
        ))

    # one canary node in the cpu pool
    nodes[0]["metadata"]["labels"]["example.com/canary"] = "true"

    json.dump({"apiVersion": "v1", "kind": "List", "items": nodes},
              fp=__import__("sys").stdout, indent=2)


if __name__ == "__main__":
    main()
