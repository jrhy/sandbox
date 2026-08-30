# Jellyfin on Bazzite — per-user, sudo-free

Jellyfin media server running as a **rootless Podman container** defined by a
**Quadlet** file and managed by a `systemctl --user` unit. No sudo, no system
packages, no rpm-ostree layering — works on any Bazzite / Silverblue /
Kinoite-style host, and installs entirely inside your home directory.

## Files

- `setup.sh` — one-shot, idempotent installer (safe to re-run)
- `verify.sh` — end-to-end health check (service, API, login, index, streaming)

## What setup.sh does

1. Creates `~/.local/share/jellyfin/{config,cache}`
2. Pulls the **pinned** image (default `docker.io/jellyfin/jellyfin:10.11.11`,
   override with `JF_IMAGE`) and writes a **Quadlet** file,
   `~/.config/containers/systemd/container-jellyfin.container`, which
   systemd's podman user generator turns into `container-jellyfin.service`.
   The generated unit rebuilds the container on every start — if the
   container is ever removed, the next unit start restores it. (Re-running
   setup.sh also migrates an older generate-systemd install automatically.)
   The container runs `docker.io/jellyfin/jellyfin` with:
   - config/cache from your home
   - your media dir mounted **read-only** at `/media` inside the container
     (resolved to `/var/mnt/...` — rootless podman cannot bind through the
     `/mnt → /var/mnt` symlink on Bazzite)
   - `SecurityLabelDisable=true` so SELinux doesn't relabel the shared
     media drive; the `:ro` mount keeps writes out of it either way
3. Enables and starts the unit, waits for the HTTP API
4. Completes the first-run wizard over the API (Jellyfin 10.11 quirk:
   `GET /Startup/User` must precede the `POST`, or you get 404), creating
   the admin user
5. Enables remote access (LAN clients) via `POST /System/Configuration/network`
6. Creates a library from the media dir and triggers a scan
7. Enables `loginctl` linger so the server starts at **boot without login**
8. Adds `AutoUpdate=registry` to the container and enables the daily
   `podman-auto-update.timer` (inert while the image tag is pinned —
   see *Upgrading and downgrading* below)
9. Prints a summary and runs `verify.sh`

Admin credentials are stored in `~/.local/share/jellyfin/ADMIN_CREDENTIALS`
(mode 600). Pass `JF_ADMIN_PASSWORD=...` to set your own instead.

## Usage

```sh
./setup.sh                          # defaults: /var/mnt/hdd/media, port 8096
JF_MEDIA_DIR=/var/mnt/hdd/media JF_PORT=8096 ./setup.sh
./verify.sh                         # run the health check any time
```

Web UI: `http://<host-ip>:8096` — add server by IP in clients
(auto-discovery UDP 7359 isn't proxied by rootless podman).

## Day-to-day

```sh
systemctl --user status container-jellyfin
systemctl --user restart container-jellyfin
journalctl --user -u container-jellyfin -f
podman logs -f jellyfin

# update: bump the tag in setup.sh (or the .container file), then:
podman pull docker.io/jellyfin/jellyfin:10.11.11 && systemctl --user restart container-jellyfin
# (or re-run ./setup.sh with JF_IMAGE=... to rewrite the Quadlet file)

## Upgrading and downgrading

Updates touch two layers, each with its own safety net:

- **Image**: the daily `podman-auto-update.timer` runs `podman auto-update`,
  which checks the registry digest of the tag in `Image=`. With a pinned
  release tag that digest doesn't change, so it's a no-op; if you ever
  switch `Image=` to a mutable tag like `:latest`, updates happen
  automatically. `podman auto-update` rolls back to the previous image
  automatically if the unit fails to come up after the restart (detected
  via sd_notify) — but it cannot detect a functionally-broken server.
- **Config/data**: every service start (including right before an upgrade
  restart, whether manual or auto) snapshots `config/` to
  `~/.local/share/jellyfin/config-snapshots/` (reflink copy, last 6 kept).
  Each snapshot has a sidecar `config-<ts>.image` recording the image
  that was RUNNING when it was taken (captured from the container after
  each successful start — during an upgrade restart the `.container`
  file already names the NEW image, so it can't be trusted for this).

### Rollback — worked example

```sh
S=~/.local/share/jellyfin

# 1. list snapshots and the image each was taken under
ls -1t "$S"/config-snapshots/
for f in "$S"/config-snapshots/*.image; do
    echo "$(basename "$f") -> $(cat "$f")"; done

# 2. stop the server
systemctl --user stop container-jellyfin.service

# 3. restore config from the snapshot taken under the good version
rm -rf "$S/config"
cp -a "$S/config-snapshots/config-YYYYMMDD-HHMMSS" "$S/config"

# 4. re-pin Image= in the Quadlet file to the tag from that snapshot's
#    .image sidecar (older tag, or a digest: ...jellyfin@sha256:...)
$EDITOR ~/.config/containers/systemd/container-jellyfin.container

# 5. reload + start (ExecStartPre takes a fresh snapshot first)
systemctl --user daemon-reload
systemctl --user start container-jellyfin.service

# 6. verify
./verify.sh
```

Old images are kept locally by podman (auto-update never deletes them),
so re-pinning is instant and offline. Pair the config restore with the
re-pin: restarting a NEW image on an OLD config just re-runs the
database migrations you were escaping.
```

## Notes / caveats

- If LAN clients can't connect but `verify.sh` passes locally, the distro
  firewall is blocking port 8096 — allow it in the desktop Firewall settings.
- Media is read-only to Jellyfin; downloaded metadata/artwork lives in
  `~/.local/share/jellyfin/config`.
- The unit is self-healing: `podman rm -f jellyfin` and the service
  rebuilds the container on the next start.
- Full removal:

```sh
systemctl --user disable --now container-jellyfin
rm ~/.config/containers/systemd/container-jellyfin.container
podman rm -f jellyfin 2>/dev/null; podman rmi docker.io/jellyfin/jellyfin:10.11.11
rm -rf ~/.local/share/jellyfin
systemctl --user daemon-reload
```
