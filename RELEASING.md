# Releasing

`VERSION` is the canonical version for the whole app. `frontend/package.json`
must always match it, and release tags must use the `v<VERSION>` format.

Prepare a release locally:

```bash
scripts/release.sh 1.2.3
git push origin master
git push origin v1.2.3
```

Pushing the `v1.2.3` tag triggers the `Release` workflow: it validates the
version, builds and pushes the backend and frontend Docker images, then creates
a **draft** GitHub Release for `v1.2.3` whose body lists the published image
versions with ready-to-copy `docker pull` commands. Review the draft, add your
notes, and publish it from the Releases page.

Normal commits and pull requests do not build or publish images — only a pushed
`v*.*.*` tag does. Publishing the draft does not rebuild anything; the images
were already pushed when the tag landed.

Stable releases publish these GHCR tags:

```text
ghcr.io/geekdada/hysterical-panel-backend:1.2.3
ghcr.io/geekdada/hysterical-panel-backend:1.2
ghcr.io/geekdada/hysterical-panel-backend:1
ghcr.io/geekdada/hysterical-panel-backend:latest

ghcr.io/geekdada/hysterical-panel-frontend:1.2.3
ghcr.io/geekdada/hysterical-panel-frontend:1.2
ghcr.io/geekdada/hysterical-panel-frontend:1
ghcr.io/geekdada/hysterical-panel-frontend:latest
```

Prereleases such as `v1.2.3-rc.1` publish only:

```text
ghcr.io/geekdada/hysterical-panel-backend:1.2.3-rc.1
ghcr.io/geekdada/hysterical-panel-frontend:1.2.3-rc.1
```

The frontend image is built with an empty `VITE_API_BASE_URL` (same-origin API
calls). Put a reverse proxy in front of the UI and backend, or use the root
`docker-compose.yml` nginx setup for local full-stack runs.
