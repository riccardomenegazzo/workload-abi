# Payment API demo

This deliberately small example proves the core Workload ABI idea with two API-compatible container releases.

`v1` writes only under `/tmp`, listens on 8080, has a small memory footprint, and shuts down quickly. `v2` keeps the same HTTP API but introduces operational changes: it writes under `/var/lib/payment`, listens on 9090, spawns a shell child, allocates substantially more memory, and delays graceful shutdown.

Build both images:

```bash
make demo-build
```

Compare them under the exact same scenario and evaluate the candidate against the supplied Compose target:

```bash
./bin/wabi compare \
  wabi-demo/payment-api:v1 \
  wabi-demo/payment-api:v2 \
  --scenario examples/payment-api/scenario.json \
  --target examples/payment-api/compose.yaml \
  --service payment-api \
  --graphs-dir out/graphs \
  --json out/result.json \
  --fail-on never
```

The expected result is `BREAKING`: the candidate's observed runtime behavior conflicts with the target's read-only filesystem, memory limit, and stop grace period.
