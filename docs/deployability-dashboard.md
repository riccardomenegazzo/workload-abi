# Environment Compatibility Matrix and Deployability Dashboard

Workload ABI can evaluate one verified baseline/candidate evidence pair against multiple production environments and persist the result as an independently versioned artifact.

The matrix answers a deployment question rather than a source-code question:

> Given what this release actually did under the recorded experiment, where can it run without a proven target conflict?

## Matrix artifact

Configuration uses `wabi.matrix-config/v1alpha1`:

```json
{
  "schema_version": "wabi.matrix-config/v1alpha1",
  "environments": [
    {
      "name": "developer-compose",
      "target": "compose.yaml",
      "target_kind": "compose",
      "service": "api"
    },
    {
      "name": "staging-kubernetes",
      "target": "deployment.json",
      "target_kind": "kubernetes",
      "container": "api",
      "network_policy": "egress.json"
    },
    {
      "name": "production-ecs",
      "target": "task-definition.json",
      "target_kind": "ecs",
      "container": "api"
    },
    {
      "name": "hardened-runtime",
      "seccomp_profile": "seccomp.json"
    }
  ]
}
```

Paths are resolved relative to the matrix configuration file so the configuration can be committed as a portable repository artifact.

Build and persist the matrix:

```bash
wabi matrix \
  --config environments.json \
  --output matrix.json \
  baseline.deep.json candidate.deep.json
```

Verify it later without rerunning the workloads:

```bash
wabi verify-matrix matrix.json
```

The persisted artifact uses `wabi.matrix/v1alpha1` and contains:

- baseline/candidate image identity;
- baseline/candidate Operational ABI fingerprints;
- scenario identity;
- per-environment target and policy identity;
- complete comparison changes per environment;
- summary counts;
- overall verdict;
- deterministic matrix fingerprint.

Changing a stored verdict, blocker, target identity, fingerprint binding, or other matrix content invalidates the matrix fingerprint.

## Deployability semantics

A matrix environment can be:

- `COMPATIBLE` — no relevant runtime difference was observed;
- `CHANGED` — the candidate changed, but no configured environment constraint proves it cannot run;
- `BREAKING` — at least one configured target/policy constraint proves a deployment conflict.

For deployment UX, both `COMPATIBLE` and `CHANGED` are **deployable**. `CHANGED` means review is appropriate; it is not automatically a deployment blocker. Only `BREAKING` is shown as **blocked**.

This distinction prevents a runtime difference from being confused with a target incompatibility.

## Dashboard

Serve a verified matrix locally:

```bash
wabi dashboard --matrix matrix.json
```

Default URL:

```text
http://127.0.0.1:8787
```

The dashboard includes:

- release baseline/candidate identity and Operational ABI fingerprints;
- matrix verdict and deployable/blocked counts;
- environment cards with target identities;
- blocker surfaces such as filesystem, runtime-network, syscall, privilege, lifecycle, or resource;
- expandable normalized evidence for every environment;
- blocker concentration by surface;
- artifact integrity bindings;
- client-side search and deployable/blocked filters;
- matrix JSON export.

### Causal graph correlation

If causal graphs are available, pass both:

```bash
wabi dashboard \
  --matrix matrix.json \
  --baseline-graph baseline.graph.json \
  --candidate-graph candidate.graph.json
```

The dashboard refuses graph artifacts unless their `snapshot_fingerprint` values match the exact baseline/candidate fingerprints stored in the matrix. The graph is therefore evidence-linked rather than decorative.

Causal explanations are then shown alongside environment blockers, for example:

```text
New process /usr/bin/helper is executed by /app,
reads /var/run/secrets/token and connects to api.vendor.com:443.
```

## Self-contained CI artifact

No server is required to share the dashboard:

```bash
wabi dashboard \
  --matrix matrix.json \
  --baseline-graph baseline.graph.json \
  --candidate-graph candidate.graph.json \
  --export dashboard.html
```

The output is one self-contained HTML file with embedded CSS, JavaScript, verified artifact data, and no CDN/npm dependency. It can be uploaded directly as a CI artifact or opened offline.

## Security boundary

Runtime evidence may reveal internal paths, process names, network destinations, policy names, and deployment topology.

For that reason:

- the dashboard binds to loopback by default;
- non-loopback binds are rejected unless `--allow-remote` is explicit;
- the server applies a restrictive Content Security Policy;
- the UI has no external network dependency;
- matrix and optional graph artifacts are fingerprint-verified before serving;
- graph artifacts must be bound to the same snapshots as the matrix.

Example explicit remote bind:

```bash
wabi dashboard \
  --matrix matrix.json \
  --listen 0.0.0.0:8787 \
  --allow-remote
```

Use that only on a network where exposing the runtime evidence is acceptable.

## Why the matrix is a derived artifact

The matrix does not modify snapshot schema `wabi.dev/v1alpha3`. Like the causal graph, it is a derived artifact with its own namespace.

That preserves three independent layers:

```text
observed evidence
      ↓
Operational ABI snapshot
      ↓
semantic comparison
      ↓
multiple environment proofs
      ↓
Environment Compatibility Matrix
      ↓
Deployability Dashboard
```

New target adapters can therefore be added without changing the meaning of previously published evidence artifacts.
