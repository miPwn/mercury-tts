# Deployment Workflow

This repository now supports two explicit deploy paths:

- Falcon host runtime deployment for `/usr/local/bin/halo` and `/usr/local/bin/hal`
- Falcon K3s deployment for `tts/hal-tts`

## Falcon Host Runtime

Run from the Falcon checkout when you are ready to install the live script target:

```bash
./scripts/deploy-falcon-runtime.sh --runtime halo
./scripts/deploy-falcon-runtime.sh --runtime hal
```

The script installs the checked-out runtime to the matching live path and verifies the result with `command -v`, `sha256sum`, and `ls -l`.

## Falcon K3s Runtime

Run from the Falcon checkout when you want to rebuild and redeploy `hal-tts`:

```bash
./deploy-hal-tts.sh
```

Useful overrides:

```bash
HAL_TTS_IMAGE_TAG=gh-12345 ./deploy-hal-tts.sh
HAL_TTS_NAMESPACE=tts ./deploy-hal-tts.sh
```

The script rebuilds `streaming_safe_daemon`, builds the Docker image on Falcon, imports it into the local K3s container runtime, reapplies the manifest, pins the deployment to the fresh image tag, and waits for rollout.

## GitHub Actions Deploy Workflows

Manual workflows are available in:

- `.github/workflows/deploy-host.yml`
- `.github/workflows/deploy-k3s.yml`

Required GitHub secrets:

- `FALCON_SSH_HOST`
- `FALCON_SSH_USER`
- `FALCON_SSH_PRIVATE_KEY`

Optional GitHub variable:

- `FALCON_SSH_PORT`

Those workflows sync the checked-out repository contents to the Falcon repo path you provide and then run the same checked-in deploy scripts there.

## Operational Notes

- For host deploys, installation is only complete when `/usr/local/bin/halo` or `/usr/local/bin/hal` has been updated.
- For K3s deploys, a successful image build is not sufficient; the deployment rollout must complete in `tts`.
- If you want clean-slate CI deploys, point the workflow at a dedicated Falcon checkout rather than an actively edited repo clone.
