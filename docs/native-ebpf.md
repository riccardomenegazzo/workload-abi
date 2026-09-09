# Native Linux eBPF provider

`wabi-native` is the optional native Linux evidence provider for Workload ABI.

It captures selected runtime behavior directly from Linux eBPF tracepoints and emits the same public `RuntimeEvent` contract used by Falco, Tracee, and generic third-party sensors.

The native provider does **not** contain a second comparison or compatibility engine.

```text
Linux kernel
    |
    v
native eBPF tracepoints
    |
    v
wabi-native
    |
    v
RuntimeEvent[]
    |
    v
wabi enrich --format generic
    |
    v
verified v1alpha3 snapshot
    |
    +--> semantic diff
    +--> target solving
    +--> policy
    +--> causal runtime graph
```

## Why it is a separate binary

The core `wabi` CLI remains portable and does not need native eBPF privileges or Linux-only runtime assumptions.

`wabi-native` is shipped separately so users can opt into kernel observation only where it is appropriate. This also keeps the public boundary simple: the provider produces normalized `RuntimeEvent` JSON, and the existing `wabi enrich` pipeline consumes it.

## Current evidence surface

The v0.6 provider captures three native Linux behaviors:

| Probe | Tracepoint | RuntimeEvent |
|---|---|---|
| process execution | `syscalls/sys_enter_execve` | `category=process`, `operation=exec` |
| file open | `syscalls/sys_enter_openat` | `category=file`, `operation=open` |
| outbound socket connect | `syscalls/sys_enter_connect` | `category=network`, `operation=connect`, `direction=outbound` |

For process and file events, the provider records executable/process context where available. For network events it decodes IPv4 and IPv6 socket destinations.

PID and provider name are diagnostic metadata and do not define RuntimeEvent semantic identity.

## Kernel portability model

The provider uses `github.com/cilium/ebpf` and constructs tracepoint programs directly in Go assembly.

It deliberately does not hard-code tracepoint argument offsets. Instead, it reads the active kernel's tracefs `format` metadata and discovers fields such as `filename` and `uservaddr` at runtime.

This avoids generated kernel-specific offsets while preserving a small, auditable native collector.

The current implementation is tracepoint-based rather than a broad CO-RE object bundle. Future native coverage can evolve behind the same `RuntimeEvent` boundary without changing compatibility semantics.

## Requirements

- Linux;
- a kernel with eBPF and the required syscall tracepoints enabled;
- tracefs available at `/sys/kernel/tracing` or `/sys/kernel/debug/tracing`;
- sufficient privilege to load and attach eBPF programs and create the perf event map.

On many systems this means running the provider as root. Capability-only deployments depend on the host kernel, LSM configuration, and distribution policy and are intentionally not assumed by the project.

If tracefs is not mounted, an administrator can mount it before starting the provider:

```bash
sudo mount -t tracefs tracefs /sys/kernel/tracing
```

`wabi-native` does not mount host filesystems itself.

## Record native evidence

Build on Linux:

```bash
make build-native
```

Record a bounded observation window:

```bash
sudo ./bin/wabi-native record \
  --duration 10s \
  --max-events 10000 \
  --output native-events.json \
  --stats-output native-stats.json
```

`native-events.json` is a JSON array of public `RuntimeEvent` objects.

Example shape:

```json
[
  {
    "source": "native-ebpf",
    "category": "network",
    "operation": "connect",
    "process": "curl",
    "target": "203.0.113.10:443",
    "protocol": "tcp",
    "direction": "outbound",
    "pid": 4242
  }
]
```

## Enrich a snapshot

Because the output already uses the provider-neutral schema, no native-only adapter is required:

```bash
wabi enrich \
  --snapshot snapshot.json \
  --events native-events.json \
  --format generic \
  --output snapshot.native.json
```

The resulting snapshot is fingerprinted and can be used with every existing offline feature:

```bash
wabi compare-snapshots baseline.native.json candidate.native.json
wabi graph snapshot.native.json --output graph.json
```

This is the architectural invariant of v0.6:

> Native collection changes how evidence is observed, not what Workload ABI means.

## Bounded collection and diagnostics

`--duration` bounds the observation window.

`--max-events` bounds the number of unique semantic events retained in memory.

The stats artifact reports:

- number of captured normalized events;
- perf-buffer lost sample count;
- actual collection duration;
- per-probe availability and attach/load errors.

Example:

```json
{
  "captured": 37,
  "lost_samples": 0,
  "duration": 4000000000,
  "probes": [
    {"name":"process-exec","available":true},
    {"name":"file-openat","available":true},
    {"name":"network-connect","available":true}
  ]
}
```

Lost samples are diagnostics rather than evidence facts, but they matter when deciding whether a capture is sufficiently complete for a production gate.

## CI proof

The repository has a dedicated `Native eBPF` GitHub Actions workflow that runs on an Ubuntu host and proves the complete native path:

1. unit tests the decoder and native package;
2. builds `wabi-native` and the core `wabi` CLI;
3. mounts/checks tracefs with host privilege;
4. loads and attaches all three eBPF programs;
5. generates real process, file, and network activity;
6. asserts all three RuntimeEvent categories were observed;
7. feeds the native event artifact through `wabi enrich`;
8. verifies the resulting v1alpha3 snapshot and operational fingerprint.

The workflow also cross-builds the Linux ARM64 provider so release packaging does not rely on an untested architecture.

## Security boundary

Native eBPF observation is privileged host instrumentation. Treat `wabi-native` differently from parsing an offline Falco or Tracee artifact.

Do not run the native provider on a sensitive production host merely to inspect untrusted workloads unless the privilege and telemetry implications are acceptable for that environment.

The provider is read-only with respect to workload behavior: it observes tracepoints and does not intentionally modify workload processes, files, networking, or kernel policy. Privileged eBPF access is nevertheless security-sensitive.

See [`../SECURITY.md`](../SECURITY.md).

## Deliberate v0.6 scope

v0.6 establishes a working native provider boundary and live-kernel proof. It does not claim complete runtime coverage.

Still intentionally deferred:

- process fork/clone/exit lifecycle;
- file read/write/rename/unlink semantics;
- TCP accept/listen;
- DNS query/resolution evidence;
- cgroup/container identity attribution;
- capability and broader syscall requirement evidence;
- cross-provider equivalence corpus for the same live workload.

Those capabilities can be added incrementally without changing the public `RuntimeEvent` model or introducing a native-only compatibility path.
