# Deployment Contract

This repository contains `deployment-contract.json` to make deployment-facing changes explicit and reviewable.

## Purpose

The contract is intended to stop repo-local edits from drifting away from the real runtime topology.

It records:

- active runtime targets
- environment ownership boundaries
- cross-repo integration points
- change guardrails that require contract updates when deployment-facing files change

## Current Runtime Targets

- Falcon host runtime: `/usr/local/bin/halo`
- Falcon host runtime: `/usr/local/bin/hal`
- Falcon K3s deployment path: `tts/hal-tts`
- Archos primary XTTS dependency: `http://192.168.1.165:5003/api/tts`
- Windows client wrapper path: `tools/halo.ps1`

## Operator Workflow

When you change host runtime behavior:

1. Update the source files.
2. Update `deployment-contract.json` if the live target, ownership boundary, or integration contract changed.
3. Run `python3 scripts/validate_deployment_contract.py`.
4. If the change touches Falcon runtime deployment, confirm the corresponding Falcon-side update path exists.
5. If the change touches Windows wrappers or port-forward tooling, confirm the wrapper and Falcon runtime assumptions still match.

When you change K3s deployment assets:

1. Update the manifest or image inputs.
2. Update `deployment-contract.json` if the cluster target, environment role, or required companion files changed.
3. Re-run the validator.

## Guardrail Intent

The validator is deliberately narrow.

It does not claim a deployment is correct in production.
It enforces that deployment-relevant changes must also update the reviewed contract file, which makes Falcon, Archos, K3s, and Windows assumptions visible in code review.

## Release Bundles

This repo now includes `scripts/build_release_bundle.py`.

Use it to produce deterministic source bundles for the runtime targets recorded in the contract:

```bash
python3 scripts/build_release_bundle.py
```

CI also runs deployment-tool tests and publishes bundle artifacts through `.github/workflows/release-bundles.yml`.

## Related Interfaces

- Playback queue HTTP submission to the queue service
- Latency state consumed by `halo-mon`
- Dotmatrix trigger writes consumed by `hal-display`

Those interfaces are part of the contract because a change here is rarely local to this repo alone.
