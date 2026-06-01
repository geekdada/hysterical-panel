# Releasing

`VERSION` is the canonical version for the whole app. `frontend/package.json`
must always match it, and release tags must use the `v<VERSION>` format.

Prepare a release locally:

```bash
scripts/release.sh 1.2.3
git push origin master
git push origin v1.2.3
```

Then manually publish a GitHub Release for `v1.2.3`.

The backend Docker image is built and pushed only after the GitHub Release is
published. Normal commits, pull requests, and tag pushes do not build or publish
the image.

Stable releases publish these GHCR tags:

```text
ghcr.io/geekdada/hysterical-panel-backend:1.2.3
ghcr.io/geekdada/hysterical-panel-backend:1.2
ghcr.io/geekdada/hysterical-panel-backend:1
ghcr.io/geekdada/hysterical-panel-backend:latest
```

Prereleases such as `v1.2.3-rc.1` publish only:

```text
ghcr.io/geekdada/hysterical-panel-backend:1.2.3-rc.1
```
