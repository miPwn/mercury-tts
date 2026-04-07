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

The `halo` host deploy now installs a self-contained container app tree under `/opt/halo`, builds the `halo-runtime` image from that tree, starts it with Docker Compose, and updates `/usr/local/bin/halo` to a thin wrapper that executes commands inside the running container.

The installed `halo` app tree includes the build context and runtime assets needed by the container: the runtime script, Python helper modules, prompt templates, canon reference files, sensory/aware support packages, and the container entrypoint/wrapper scripts. The live runtime does not execute from the repo checkout.

The Compose service runs with host networking and host PID visibility so the existing local endpoint assumptions remain valid on Falcon, including `127.0.0.1` playback queue and XTTS fallback routes.

The `hal` deploy remains a lightweight self-contained script install under `/opt/hal` with `/usr/local/bin/hal` symlinked into place.

Verification for `halo` reports the command resolution path, compares the repo script hash to both the installed build-context copy and the script inside the running container, shows the compose service state, and prints both the app directory and the PATH symlink.

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
