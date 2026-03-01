# Gastownfun

`gastownfun` gives you a persistent Alpine Linux environment through Podman on macOS.

## What Persists

- The image baseline is built from `gastownfun/Containerfile`, so Podman caches the expensive package and tool install layers between builds.
- The named container `gastownfun-alpine` has its own writable layer, so ad hoc changes you make inside the container stay available until you recreate or remove that container.
- Files written to `/data` inside the container appear on the host in `gastownfun/data/`.
- `/data/root` is the persistent home for the root account.
- `/data/user` is the persistent home for the default non-root `user` account.

## Isolation

- The container only gets one host bind mount: `gastownfun/data/` at `/data`.
- No other part of your home directory is mounted into the container.
- The container still has its own internal root filesystem, but that is container-local state, not direct access to the host.

## Usage

From this directory:

```sh
./gastownfun/ensure-container.sh
./gastownfun/enter.sh
```

`./gastownfun/ensure-container.sh` builds the local image from `gastownfun/Containerfile`, creates the container when needed, starts it, and performs the runtime `/data` setup.

`./gastownfun/enter.sh` creates the container the first time, starts it if needed, and drops you into a login shell as `user`.

To enter as root directly:

```sh
./gastownfun/enter.sh root
```

Inside the container:

```sh
whoami
pwd
sudo -i
doas sh
cd /data
```

The image baseline installs `git`, `sqlite3`, `tmux`, `dolt`, `bd` (Beads), `gt` (Gastown), and the OpenAI Codex CLI, plus the runtime packages needed to support them.

That includes real `procps-ng` and `lsof` binaries instead of the BusyBox applets, because Gastown's Dolt status checks rely on GNU-style `ps` and `lsof`.

The checked-in [`gastownfun/data-golden/`](/Users/jeffr/sandbox/gastownfun/data-golden) directory is the safe reset baseline for reusable, non-personal user dotfiles. It intentionally includes only curated config like shell, tmux, git, and Codex settings, and excludes live state such as `gt/`, caches, histories, and other generated runtime data.

For new Gas Town HQs, `gt` currently defaults the town agent to `claude`. To use Codex, run this once inside each new town:

```sh
gt config default-agent codex
```

The container also keeps `~/.codex/config.toml` seeded with:

```toml
project_doc_fallback_filenames = ["CLAUDE.md"]
```

The tmux baseline uses vi-style copy and scrollback keys via `mode-keys vi`.

If you experiment inside the container and then decide a change should become permanent, add it to `gastownfun/Containerfile`, run `./gastownfun/ensure-container.sh`, and then use `--recreate` when you are ready to replace the current writable layer with the new baseline.

## Recreate

If you want to throw away the container writable layer and recreate from the current `Containerfile` baseline without deleting the persistent homes in `/data`, run:

```sh
./gastownfun/ensure-container.sh --recreate
```

If you remove the container manually:

```sh
podman rm -f gastownfun-alpine
```

That also resets the container writable layer. Files in `gastownfun/data/`, including `root` and `user`, remain on the host.
