# sandbox-exec wrapper + Python example (macOS)

Tool for running potentially-untrusted binaries on macOS:
* no network access
* writes confined to the current directory (and a per-run temp dir)
* reads allowed in the current directory, system/runtime paths, and `PATH` entries
* parent directory listings are blocked (`ls ..` fails)
* reads under `/Users` are denied except for the current directory and any
  `PATH` entries under `/Users`

## Files

- `example.py`: Python program that creates `sandbox_data`, reads/writes a file, and
  attempts prohibited access outside the directory.
- `sandbox-wrap.sh`: Generic wrapper that generates a temporary sandbox profile
  and runs any command with access limited to the current directory.

## Run

```sh
chmod +x sandbox-wrap.sh
PYTHONDONTWRITEBYTECODE=1 PYTHONNOUSERSITE=1 ./sandbox-wrap.sh \
  /Library/Developer/CommandLineTools/Library/Frameworks/Python3.framework/Versions/3.9/bin/python3 -S example.py
```

## Usage synopsis

```text
sandbox-wrap.sh <command> [args...]
```

## Test

```sh
chmod +x test-sandbox.sh
./test-sandbox.sh
```

Note: the test script creates a temporary directory under `/Users/$USER` and
cleans it up on exit.

## Expected output (example)

```text
base_dir=/Users/foo/sandbox/sandbox-exec-fun
data_dir=/Users/foo/sandbox/sandbox-exec-fun/sandbox_data
create data directory
write file inside data directory
read file inside data directory
content='hello from sandbox'
attempt write outside data directory (should fail)
expected failure: PermissionError: [Errno 1] Operation not permitted: '/Users/foo/sandbox/outside.txt'
attempt read outside data directory (should fail)
expected failure: FileNotFoundError: [Errno 2] No such file or directory: '/Users/foo/sandbox/outside.txt'
attempt network access (should fail)
expected failure: gaierror: [Errno 8] nodename nor servname provided, or not known
attempt localhost network access (should fail)
expected failure: PermissionError: [Errno 1] Operation not permitted
remove data directory
```

## Notes

- `sandbox-wrap.sh` allows unrestricted reads/writes inside the current directory.
- The profile grants read-only access to system/runtime paths (e.g. `/System`,
  `/Library`, and `/System/Volumes/Data/Library`) plus `mach-lookup` so tools can
  start; writes remain confined to the current directory.
- Network access is denied by default because no `network*` rules are allowed.
- Minimal `file-read-data` and `file-read-metadata` rules for `/` are included
  so tools like `ls` and `realpath` can operate without broader access.
- `file-read-metadata` on `/var` and `file-ioctl` on `/dev` are allowed so
  interactive shells can use job control without accessing real data outside
  the current directory.
- Parent directories of the current working directory (excluding `/`) allow
  metadata-only reads but deny `file-read-data`, which blocks `ls ..` while
  still allowing traversal.
- Reads and executable mappings under `/Users` (and `/System/Volumes/Data/Users`)
  are denied except for the current directory and any `PATH` entries that live
  under `/Users`.
- Directories in the current `PATH` are allowed for read+execute so tools there
  can run without manually editing the profile.
- If a `PATH` entry equals the user home directory (e.g. `/Users/name`), it is
  ignored to avoid granting read access to the entire home tree.
- The wrapper creates a per-run temporary directory under `${TMPDIR:-/tmp}`,
  sets `TMPDIR`, `TMP`, and `TEMP`, and removes it on exit. This lets tools use
  temp files without accessing other users' temp files.
- The wrapper also creates a per-run `HOME` directory under `${TMPDIR:-/tmp}`
  to keep tools from reading your real home directory.

## FAQ and considerations

**Is `sandbox-exec` deprecated?**  
Yes. The manpage labels it DEPRECATED and suggests using App Sandbox for apps.

**Why use it anyway?**  
It is still a practical, lightweight way to sandbox arbitrary CLI commands
without turning them into a signed, sandboxed app bundle.

**What is this good for?**  
Running untrusted code so it cannot write outside a working directory or access
the network, while still letting it read system/runtime files needed to start.

**Is there an App Sandbox replacement for arbitrary commands?**  
Not directly. App Sandbox applies to signed app bundles, not ad-hoc shell
commands. The closest alternative is a small helper that calls `sandbox_init()`
with a profile and then `exec`s the target command.
