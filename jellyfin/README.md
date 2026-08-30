# Jellyfin on Bazzite — per-user, sudo-free

Jellyfin media server running as a **rootless Podman container** managed
by a `systemctl --user` unit. No sudo, no system packages, no rpm-ostree
layering — works on any Bazzite / Silverblue / Kinoite-style host, and
installs entirely inside your home directory.

## Files

- `setup.sh` — one-shot, idempotent installer (safe to re-run)
- `verify.sh` — end-to-end health check (service, API, login, index, streaming)

## What setup.sh does

1. Creates `~/.local/share/jellyfin/{config,cache}`
2. Pulls the **pinned** image (default `docker.io/jellyfin/jellyfin:10.11.11`,
   override with `JF_IMAGE`), creates the container and generates
   `~/.config/systemd/user/container-jellyfin.service` from its spec via
   `podman generate systemd --new`. Because the unit embeds the full
   `podman run` flags, systemd rebuilds the container on every start —
   if the container is ever removed, the next unit start restores it.
   It runs `docker.io/jellyfin/jellyfin` with:
   - config/cache from your home
   - your media dir mounted **read-only** at `/media` inside the container
     (resolved to `/var/mnt/...` — rootless podman cannot bind through the
     `/mnt → /var/mnt` symlink on Bazzite)
   - `--security-opt label=disable` so SELinux doesn't relabel the shared
     media drive; the `:ro` mount keeps writes out of it either way
3. Enables and starts the unit, waits for the HTTP API
4. Completes the first-run wizard over the API (Jellyfin 10.11 quirk:
   `GET /Startup/User` must precede the `POST`, or you get 404), creating
   the admin user
5. Enables remote access (LAN clients) via `POST /System/Configuration/network`
6. Creates a library from the media dir and triggers a scan
7. Enables `loginctl` linger so the server starts at **boot without login**
8. Prints a summary and runs `verify.sh`

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

# update to the pinned tag (bump the tag in setup.sh first, or override):
JF_IMAGE=docker.io/jellyfin/jellyfin:10.11.12 ./setup.sh   # recreates unit + container
# or, for an already-installed unit after pulling a new image:
podman pull docker.io/jellyfin/jellyfin:10.11.11 && systemctl --user restart container-jellyfin
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
rm ~/.config/systemd/user/container-jellyfin.service ~/.config/systemd/user/default.target.wants/container-jellyfin.service
podman rm -f jellyfin 2>/dev/null; podman rmi docker.io/jellyfin/jellyfin:10.11.11
rm -rf ~/.local/share/jellyfin
systemctl --user daemon-reload
```
