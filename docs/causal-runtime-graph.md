# Causal Runtime Graph

The Workload ABI Causal Runtime Graph converts normalized deep runtime evidence into a deterministic graph of **who did what to which runtime dependency**.

It exists to answer a question that flat diffs cannot answer well:

> Which new execution path caused this operational dependency to appear?

The graph is derived from a verified `wabi.dev/v1alpha3` snapshot. It is intentionally versioned separately as `wabi.graph/v1alpha1` so graph semantics can evolve without rewriting the snapshot evidence contract.

## Pipeline

```text
verified snapshot
      |
      v
normalized RuntimeEvent set
      |
      v
Causal Runtime Graph
      |
      +---- nodes: workload / process / file / endpoint / domain / syscall
      |
      +---- edges: spawn / exec / read / write / connect / listen / ...
      |
      v
graph fingerprint
      |
      +---- compare with another graph
      |
      v
edge diff + causal explanations
```

## Node identity

Node identifiers are deterministic semantic identities.

Examples:

```text
workload:root
process:/usr/bin/app
process:/usr/bin/helper
file:/var/run/secrets/token
endpoint:api.vendor.com:443
domain:telemetry.example.com
syscall:io_uring_setup
```

The current graph deliberately does **not** use PID as process identity. PIDs are runtime-instance diagnostics and are not stable across equivalent experiments.

That means `process:/usr/bin/helper` represents the normalized executable identity observed by the evidence provider, not one particular kernel process instance.

## Edge identity

An edge is identified by:

```text
from + relation + to
```

Examples:

```text
process:/usr/bin/app
  --spawn-->
process:/usr/bin/helper

process:/usr/bin/helper
  --read-->
file:/var/run/secrets/token

process:/usr/bin/helper
  --connect:outbound-->
endpoint:api.vendor.com:443
```

Provider metadata such as Falco rule names, Tracee event metadata, PID, timestamps, and sensor identity does not participate in graph identity.

## Graph fingerprint

Every graph contains two hashes with different meanings:

- `snapshot_fingerprint` binds the graph to the verified Operational ABI evidence it was derived from;
- `fingerprint` identifies the deterministic graph artifact itself.

The graph fingerprint covers:

- graph schema version;
- source snapshot fingerprint;
- normalized nodes;
- normalized edges.

It deliberately excludes display metadata such as the image reference.

`wabi graph-diff` verifies both input graph fingerprints before comparing them. A modified edge with an unchanged fingerprint fails closed.

## Causal explanations

A raw graph diff may contain several new edges that are really one operational change.

For example:

```text
/usr/bin/app --spawn--> /usr/bin/helper
/usr/bin/helper --read--> /var/run/secrets/token
/usr/bin/helper --connect:outbound--> api.vendor.com:443
```

Workload ABI groups that chain into one explanation:

```text
New process /usr/bin/helper is spawned by /usr/bin/app and reads
/var/run/secrets/token and connects to api.vendor.com:443.
```

This is intentionally evidence-backed. The explanation is assembled only from graph edges that were actually observed and newly introduced in the candidate graph.

## CLI

Build graphs from verified enriched snapshots:

```bash
wabi graph --output baseline.graph.json baseline.deep.json
wabi graph --output candidate.graph.json candidate.deep.json
```

Compare them:

```bash
wabi graph-diff baseline.graph.json candidate.graph.json
```

Machine-readable output:

```bash
wabi graph-diff \
  --format json \
  baseline.graph.json candidate.graph.json > graph-diff.json
```

CI gate:

```bash
wabi graph-diff \
  --fail-on-change \
  baseline.graph.json candidate.graph.json
```

`--fail-on-change` returns exit code `3` after emitting the diff when any causal edge changes.

## Why the graph is derived instead of embedded in snapshots

A snapshot is evidence. A graph is an interpretation of relationships between evidence facts.

Keeping them separate preserves three useful properties:

1. old snapshots can be reinterpreted with a newer graph engine without recollecting runtime evidence;
2. graph semantics can evolve without changing the evidence schema;
3. independent implementations can consume the public RuntimeEvent contract and produce their own graph-compatible tooling.

## Current inference boundary

`v1alpha1` intentionally avoids claims the evidence cannot support reliably.

The graph can state:

- a normalized process was observed as spawned/executed by another process;
- a process read, wrote, opened, renamed, or removed a file target;
- a process connected, listened, accepted, sent, received, or resolved a network target;
- a process used a normalized syscall.

The graph does **not** currently claim:

- that a file read caused a later network connection merely because both happened close in time;
- that two executions of the same executable path are the same kernel process instance;
- that a network destination is malicious or trusted;
- that an observed dependency is breaking without target or policy context.

Those boundaries are deliberate. Causal explanations are structural explanations over observed relations, not speculative root-cause inference.

## Next depth

The next recorder layer can improve graph precision by emitting stable process-instance correlation, cgroup/container attribution, DNS resolution edges, and richer lifecycle events. It must still normalize into the public evidence boundary before graph construction.
