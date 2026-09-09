# Deep runtime evidence

Workload ABI separates **execution** from **observation**.

Docker is currently the reference experiment runner. Deep runtime sensors are evidence providers: they can enrich a persisted Workload ABI snapshot without taking ownership of comparison, target solving, policy, or compatibility semantics.

This lets the project use real eBPF-derived evidence from Falco, Tracee, or another sensor while remaining vendor-neutral.

## Enrichment flow

```text
Docker experiment
      |
      v
snapshot.json -------------------+
                                 |
Falco / Tracee / custom sensor --+--> normalize --> v1alpha3 snapshot
                                                    |
                                                    v
                                               fingerprint
                                                    |
                                                    v
                                           semantic compatibility
```

Example:

```bash
wabi record --scenario scenario.json app:v2 > candidate.json

wabi enrich \
  --snapshot candidate.json \
  --events falco.jsonl \
  --format falco \
  --output candidate.deep.json
```

The output is a normal persisted snapshot and can be used by the existing pipeline:

```bash
wabi compare-snapshots baseline.deep.json candidate.deep.json
```

## Public event contract

The provider-neutral event schema is published at:

```text
schemas/v1alpha3/runtime-event.schema.json
```

Conceptually:

```json
{
  "source": "custom-ebpf",
  "category": "network",
  "operation": "connect",
  "process": "/usr/bin/app",
  "parent_process": "/sbin/init",
  "target": "api.example.com:443",
  "protocol": "tcp",
  "direction": "outbound",
  "pid": 1234,
  "rule": "optional provider diagnostic"
}
```

### Semantic identity

The Operational ABI identity of an event is based on:

- `category`;
- `operation`;
- `process`;
- `parent_process`;
- `target`;
- `protocol`;
- `direction`.

These fields are normalized, fingerprinted, and diffed.

The following are diagnostics and deliberately **do not** participate in semantic identity:

- `source`;
- `pid`;
- `rule`.

That distinction is essential. The same behavior observed by Falco with PID 42 and Tracee with PID 991 must represent the same Operational ABI fact.

## Generic provider

Any tool can emit one normalized JSON object per line and use:

```bash
wabi enrich \
  --snapshot snapshot.json \
  --events runtime-events.jsonl \
  --format generic
```

No Workload ABI Go dependency is required.

A JSON array of normalized runtime events is also accepted by the generic adapter.

## Falco adapter

Falco JSON output is expected as one JSON alert object per line with `output_fields`.

The adapter reads common syscall context including:

- `syscall.type` / `evt.type`;
- `proc.exepath` / `proc.name`;
- `proc.pexepath` / `proc.pname`;
- `proc.pid`;
- filesystem fields such as `fs.path.name`, `fd.name`, and common `evt.arg.*` paths;
- network fields such as remote address/port and L4 protocol;
- the Falco `rule` as diagnostic provenance.

Falco rules decide which fields are emitted. Better output context produces richer normalized events; missing optional context does not make Workload ABI Falco-specific.

## Tracee adapter

Tracee JSON is expected as one event object per line.

The adapter reads:

- `eventName` / `syscall`;
- `processName` and executable path when available;
- `processId` as diagnostic context;
- `args[]` for path/address/protocol targets.

## Event classification

Known operations are normalized into broad provider-neutral categories.

Examples:

| Raw event | Category | Operation | Direction |
|---|---|---|---|
| `execve` | process | exec | |
| `clone` | process | spawn | |
| `openat` | file | open | |
| `renameat2` | file | rename | |
| `connect` | network | connect | outbound |
| `accept4` | network | accept | inbound |
| `listen` | network | listen | inbound |

Unknown syscall names are preserved as `category=syscall` rather than discarded. This allows deeper syscall-ABI work later without requiring every kernel event to be preclassified today.

## Diff semantics

New provider-neutral process/file/network deep events default to `warning` because they represent an expanded observed operational surface. Unknown syscall evidence defaults to `info`.

Removal of an observed deep event defaults to `info`.

Policy can make a surface stricter. For example, an organization can deny any new `runtime-network` change without changing core Workload ABI semantics.

## Schema upgrade

`wabi enrich` upgrades the output snapshot to `wabi.dev/v1alpha3` and recomputes the operational fingerprint.

`v1alpha2` snapshots remain loadable and verifiable. The project does not mutate the published meaning of `v1alpha2`; deep events are only valid in `v1alpha3` or later.

## Security and privacy

Deep event streams can contain sensitive paths, commands, network destinations, and provider rule names. Review persisted evidence before publishing it.

Provider event files are treated as untrusted input. Workload ABI parses them as data and does not execute commands from the event stream.

## Future native eBPF provider

A native eBPF recorder may be added after this provider boundary is proven stable. It should emit the same normalized evidence contract rather than introducing a separate compatibility engine.

The purpose of v0.4 is therefore not to claim ownership of eBPF collection. It is to make **eBPF-derived behavior portable across sensors and meaningful to operational compatibility**.
