"""Local private configuration and serialized per-device state."""

from contextlib import contextmanager
import fcntl
import json
import os
from pathlib import Path
import re
from urllib.parse import urlparse
from .transport import Error


def read(path, default=None):
    if not path.exists():
        return default
    try:
        return json.loads(path.read_text())
    except (ValueError, OSError) as exc:
        raise Error(f"Cannot read {path}: {exc}") from exc


def write(path, value):
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    temporary = path.with_suffix(".tmp")
    with open(
        temporary, "w", opener=lambda p, flags: os.open(p, flags, 0o600)
    ) as stream:
        json.dump(value, stream, indent=2)
        stream.write("\n")
    temporary.replace(path)


def validate(config):
    profiles = config.get("devices", {})
    resources = {}
    for name, profile in profiles.items():
        if name == "_configuration" or not re.fullmatch(r"[a-zA-Z0-9_-]+", name):
            raise Error(f"Invalid profile name: {name}")
        url = urlparse(profile["endpoint"])
        if (
            url.scheme != "http"
            or url.hostname not in {"127.0.0.1", "localhost"}
            or not url.port
            or url.path not in {"", "/"}
        ):
            raise Error(
                "This proof of concept requires an explicit local HTTP endpoint port"
            )
        for kind, value in [
            ("udid", profile["udid"]),
            ("port", url.port),
            ("port", profile["wda_port"]),
            ("port", profile["mjpeg_port"]),
        ]:
            if kind == "port" and (type(value) is not int or not 1 <= value <= 65535):
                raise Error(f"Invalid port for {name}")
            key = (kind, value)
            if key in resources:
                raise Error(
                    f"Profiles must not share {kind} {value} ({resources[key]}, {name})"
                )
            resources[key] = name
    return profiles


def select(config, explicit=None, environ=None):
    profiles = validate(config)
    environ = os.environ if environ is None else environ
    name = explicit or environ.get("IPAD_DEVICE") or config.get("default")
    if name is None and len(profiles) == 1:
        name = next(iter(profiles))
    if name not in profiles:
        raise Error(
            "Select a configured device with --device, IPAD_DEVICE, or a default"
        )
    return name, profiles[name]


class Store:
    def __init__(self, root):
        self.root = Path(root).expanduser().resolve()
        self.config_path = self.root / "config.json"

    @contextmanager
    def lock(self, name):
        directory = self.root / "devices" / name
        directory.mkdir(parents=True, exist_ok=True, mode=0o700)
        with (directory / "lock").open("a") as stream:
            try:
                fcntl.flock(stream, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError as exc:
                raise Error(f"Device {name} is busy; no command was sent") from exc
            yield directory
