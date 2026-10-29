import argparse
import base64
import json
import ipaddress
import os
from pathlib import Path
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import time
from urllib.parse import urlparse

from .observation import markdown, observe, predicate, public, resolve
from .state import Store, read, select, validate, write
from .transport import Client, Error, SESSION_TIMEOUT_SECONDS

APPIUM_VERSION = "3.8.0"
XCUITEST_VERSION = "12.18.1"
SERVER_START_TIMEOUT_SECONDS = 20
SERVER_STOP_TIMEOUT_SECONDS = 10
SERVER_POLL_INTERVAL_SECONDS = 0.25
SERVER_PROBE_TIMEOUT_SECONDS = 1
NEW_COMMAND_TIMEOUT_SECONDS = 3600
DEVICE_INFO_TIMEOUT_SECONDS = 15
DEVICE_INFO_GRACE_SECONDS = 5
WDA_PROBE_TIMEOUT_SECONDS = 5
ELEMENT_KEY = "element-6066-11e4-a52e-4f735466cecf"


def parser():
    result = argparse.ArgumentParser(
        description="Observe and control local physical iPads through Appium"
    )
    result.add_argument("--device")
    result.add_argument(
        "--home", default=os.environ.get("IPAD_HOME", "~/.local/share/ipad-cli")
    )
    commands = result.add_subparsers(dest="command", required=True)
    commands.add_parser("devices")
    config = commands.add_parser("configure")
    config.add_argument("name")
    config.add_argument("--udid", required=True)
    config.add_argument("--platform-version", required=True)
    config.add_argument("--team-id", required=True)
    config.add_argument("--wda-bundle-id", required=True)
    config.add_argument("--port", type=int, default=4725)
    config.add_argument("--wda-port", type=int, default=8101)
    config.add_argument("--mjpeg-port", type=int, default=9101)
    config.add_argument(
        "--attach", action="store_true", help="Use an existing server; never stop it"
    )
    config.add_argument("--default", action="store_true")
    connection = commands.add_parser("connect")
    connection.add_argument(
        "--wifi",
        action="store_true",
        help="Attach to running WDA through this device's existing CoreDevice network tunnel",
    )
    commands.add_parser("status")
    commands.add_parser("disconnect")
    commands.add_parser("doctor")
    commands.add_parser("apps")
    commands.add_parser("screenshot")
    commands.add_parser("invalidate")
    for command in ("screen", "tap", "type", "open", "swipe", "home"):
        sub = commands.add_parser(command)
        sub.add_argument("--json", action="store_true")
        if command in {"tap", "type"}:
            sub.add_argument("reference")
        if command == "tap":
            sub.add_argument(
                "--expect", help="Verify exact visible NavigationBar label after tap"
            )
        if command == "type":
            sub.add_argument("text")
        if command == "open":
            sub.add_argument("bundle_id")
        if command == "swipe":
            sub.add_argument("direction", choices=["up", "down", "left", "right"])
    return result


def fingerprint_process(pid):
    result = subprocess.run(
        ["ps", "-p", str(pid), "-o", "lstart=", "-o", "command="],
        capture_output=True,
        text=True,
        check=False,
    )
    return result.stdout.strip()


def stop_owned_server(state):
    server = state.get("server")
    if (
        server
        and server["identity"]
        and fingerprint_process(server["pid"]) == server["identity"]
    ):
        os.kill(server["pid"], signal.SIGTERM)
        deadline = time.monotonic() + SERVER_STOP_TIMEOUT_SECONDS
        while fingerprint_process(server["pid"]) == server["identity"]:
            if time.monotonic() >= deadline:
                raise Error(
                    "Owned server did not exit; ownership state retained for inspection"
                )
            time.sleep(SERVER_POLL_INTERVAL_SECONDS)


def start_server(profile, directory):
    # Refuse occupied ports; never infer ownership from a listening port.
    for port in (
        urlparse(profile["endpoint"]).port,
        profile["wda_port"],
        profile["mjpeg_port"],
    ):
        with socket.socket() as probe:
            probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            try:
                probe.bind(("127.0.0.1", port))
            except OSError as exc:
                raise Error(
                    f"Port {port} is occupied; inspect lsof -i :{port}"
                ) from exc
    if not shutil.which("appium"):
        raise Error("appium is not installed")
    version = subprocess.check_output(["appium", "--version"], text=True).strip()
    if version != APPIUM_VERSION:
        raise Error(f"Expected Appium {APPIUM_VERSION}, found {version}")
    driver_home = Path(os.environ.get("APPIUM_HOME", "~/.appium")).expanduser()
    package = read(driver_home / "node_modules/appium-xcuitest-driver/package.json", {})
    if package.get("version") != XCUITEST_VERSION:
        raise Error(
            f"Expected shared XCUITest {XCUITEST_VERSION}; inspect appium driver list --installed"
        )
    log_path = directory / "appium.log"
    with log_path.open("ab") as log:
        process = subprocess.Popen(
            [
                "appium",
                "--address",
                "127.0.0.1",
                "--port",
                str(urlparse(profile["endpoint"]).port),
                "--log-timestamp",
            ],
            stdout=log,
            stderr=subprocess.STDOUT,
            stdin=subprocess.DEVNULL,
            start_new_session=True,
            env={**os.environ, "APPIUM_XCUITEST_PREFER_DEVICECTL": "1"},
        )
    server = {"pid": process.pid, "identity": fingerprint_process(process.pid)}
    deadline = time.monotonic() + SERVER_START_TIMEOUT_SECONDS
    while time.monotonic() < deadline:
        if process.poll() is not None:
            break
        try:
            Client(profile["endpoint"]).request(
                "GET", "/status", timeout_seconds=SERVER_PROBE_TIMEOUT_SECONDS
            )
            return server
        except Error:
            time.sleep(SERVER_POLL_INTERVAL_SECONDS)
    stop_owned_server({"server": server})
    raise Error(f"Appium did not start; inspect {log_path}")


def device_info(profile, resource, extra_args=()):
    """Read CoreDevice JSON for one explicit device; keep personal payloads local."""
    with tempfile.TemporaryDirectory(prefix="ipad-device-info-") as temporary:
        path = Path(temporary) / "device.json"
        result = subprocess.run(
            [
                "xcrun",
                "devicectl",
                "device",
                "info",
                resource,
                *extra_args,
                "--device",
                profile["udid"],
                "--timeout",
                str(DEVICE_INFO_TIMEOUT_SECONDS),
                "--json-output",
                str(path),
            ],
            capture_output=True,
            text=True,
            check=False,
            timeout=DEVICE_INFO_TIMEOUT_SECONDS + DEVICE_INFO_GRACE_SECONDS,
        )
        if result.returncode:
            raise Error(f"CoreDevice {resource} failed: {result.stderr.strip()}")
        payload = read(path, {})
        if "result" not in payload:
            raise Error(f"CoreDevice {resource} returned no result")
        return payload["result"]


def wifi_wda_url(profile):
    """Resolve only the configured UDID's Apple-managed tunnel, never a guessed IP."""
    info = device_info(profile, "details")
    properties = info.get("properties", {})
    hardware = properties.get("hardware", info.get("hardwareProperties", {}))
    if hardware.get("udid") != profile["udid"]:
        raise Error("CoreDevice returned a different device; refusing WDA attachment")
    connection = properties.get("connection", info.get("connectionProperties", {}))
    if connection.get("transportType") != "localNetwork":
        raise Error(
            "Wi-Fi mode requires USB disconnected and CoreDevice network transport"
        )
    if connection.get("state", connection.get("tunnelState")) != "connected":
        raise Error(
            "CoreDevice network tunnel is not connected; pair/connect in Xcode first"
        )
    try:
        address = ipaddress.IPv6Address(
            connection.get(
                "tunnelIPAddressString", connection.get("tunnelIPAddress", "")
            )
        )
    except (KeyError, ValueError) as exc:
        raise Error("CoreDevice did not provide a usable IPv6 tunnel address") from exc
    endpoint = f"http://[{address}]:{profile['wda_port']}"
    status = Client(endpoint).request(
        "GET", "/status", timeout_seconds=WDA_PROBE_TIMEOUT_SECONDS
    )
    if (
        status.get("build", {}).get("productBundleIdentifier")
        != profile["wda_bundle_id"]
    ):
        raise Error("Running WDA signing bundle does not match the selected profile")
    return endpoint


def installed_apps(profile):
    """Inventory uses CoreDevice; native UI control continues to use Appium/WDA."""
    info = device_info(profile, "apps", ("--include-all-apps",))
    if not isinstance(info.get("apps"), list):
        raise Error("CoreDevice app inventory is missing its apps array")
    apps = {}
    for app in info["apps"]:
        bundle_id = app.get("bundleIdentifier")
        if not isinstance(bundle_id, str) or not bundle_id:
            raise Error(
                "CoreDevice app inventory contains an app without a bundle identifier"
            )
        if bundle_id in apps:
            raise Error(f"CoreDevice returned duplicate app identifier: {bundle_id}")
        apps[bundle_id] = {
            "name": app.get("name", ""),
            "version": app.get("version", ""),
            "build": app.get("bundleVersion", ""),
        }
    return dict(sorted(apps.items()))


def connect(profile, directory, state_path, wifi=False):
    if read(state_path):
        raise Error(
            "Session state already exists; inspect status/disconnect before reconnecting"
        )
    wda_url = wifi_wda_url(profile) if wifi else None
    state = {
        "endpoint": profile["endpoint"],
        "udid": profile["udid"],
        "session": None,
        "wda_url": wda_url,
    }
    if not profile["attach"]:
        state["server"] = start_server(profile, directory)
    write(
        state_path, state
    )  # Preserve server ownership even when session creation fails.
    capabilities = {
        "platformName": "iOS",
        "appium:automationName": "XCUITest",
        "appium:udid": profile["udid"],
        "appium:platformVersion": profile["platform_version"],
        "appium:xcodeOrgId": profile["team_id"],
        "appium:xcodeSigningId": "Apple Development",
        "appium:updatedWDABundleId": profile["wda_bundle_id"],
        "appium:allowProvisioningDeviceRegistration": True,
        "appium:bundleId": "com.apple.Preferences",
        "appium:noReset": True,
        "appium:wdaLocalPort": profile["wda_port"],
        "appium:derivedDataPath": str(directory / "DerivedData"),
        "appium:newCommandTimeout": NEW_COMMAND_TIMEOUT_SECONDS,
    }
    if wda_url:
        capabilities["appium:webDriverAgentUrl"] = wda_url
        capabilities["appium:wdaRemotePort"] = profile["wda_port"]
    else:
        capabilities["appium:mjpegServerPort"] = profile["mjpeg_port"]
    value = Client(profile["endpoint"]).request(
        "POST",
        "/session",
        {"capabilities": {"alwaysMatch": capabilities, "firstMatch": [{}]}},
        timeout_seconds=SESSION_TIMEOUT_SECONDS,
    )
    state["session"] = value["sessionId"]
    write(state_path, state)
    print("Connected; run ipad screen")


def configure(args, store):
    with store.lock("_configuration"):
        config = read(store.config_path, {"devices": {}})
        if args.name in config["devices"]:
            raise Error("Profile already exists; edit config.json while disconnected")
        config["devices"][args.name] = {
            "udid": args.udid,
            "platform_version": args.platform_version,
            "team_id": args.team_id,
            "wda_bundle_id": args.wda_bundle_id,
            "endpoint": f"http://127.0.0.1:{args.port}",
            "wda_port": args.wda_port,
            "mjpeg_port": args.mjpeg_port,
            "attach": args.attach,
        }
        if args.default:
            config["default"] = args.name
        validate(config)
        write(store.config_path, config)
        print(f"Configured {args.name}: {store.config_path}")


def take_observation(client, name, observation_path):
    observation = observe(client.command("GET", "/source"), name, client.session)
    write(observation_path, observation)
    return observation


def action(args, client, name, observation_path):
    target = None
    if args.command in {"tap", "type"}:
        target = resolve(
            read(observation_path),
            args.reference,
            name,
            client.session,
            client.command("GET", "/source"),
        )
        if args.command == "type" and target["role"] not in {
            "TextField",
            "SecureTextField",
            "SearchField",
        }:
            raise Error("type requires a text/search field")
        matches = client.command(
            "POST",
            "/elements",
            {"using": "-ios predicate string", "value": predicate(target)},
        )
        if len(matches) != 1:
            raise Error(
                f"Exact native locator matched {len(matches)} elements; refusing ambiguous action"
            )
        element_id = matches[0][ELEMENT_KEY]
        # A second snapshot reduces lookup-time races. It cannot eliminate physical UI races.
        resolve(
            read(observation_path),
            args.reference,
            name,
            client.session,
            client.command("GET", "/source"),
        )
    observation_path.unlink(
        missing_ok=True
    )  # Invalidate before attempting any mutation.
    try:
        if args.command == "tap":
            client.command("POST", f"/element/{element_id}/click", {})
        elif args.command == "type":
            client.command("POST", f"/element/{element_id}/value", {"text": args.text})
        elif args.command == "open":
            client.execute("mobile: activateApp", {"bundleId": args.bundle_id})
        elif args.command == "home":
            client.execute("mobile: pressButton", {"name": "home"})
        elif args.command == "swipe":
            client.execute("mobile: swipe", {"direction": args.direction})
    except Error as exc:
        raise Error(
            f"{args.command} attempted; outcome unverified, references invalidated: {exc}"
        ) from exc
    result = {"operation": args.command, "accepted": True, "navigation_verified": False}
    if target:
        result["target"] = {
            "reference": args.reference,
            "label": target["label"],
            "role": target["role"],
        }
    try:
        observation = take_observation(client, name, observation_path)
    except Error as exc:
        raise Error(
            f"{args.command} accepted, but fresh observation failed: {exc}"
        ) from exc
    if args.command == "tap" and args.expect:
        result["expected_navigation_bar"] = args.expect
        result["navigation_verified"] = any(
            e["role"] == "NavigationBar" and e["label"] == args.expect
            for e in observation["elements"]
        )
    return observation, result


def run(args):
    os.umask(0o077)
    store = Store(args.home)
    if args.command == "configure":
        return configure(args, store)
    config = read(store.config_path, {"devices": {}})
    if args.command == "devices":
        for name, profile in validate(config).items():
            print(
                f"{name}\t{profile['endpoint']}\t{'attached' if profile['attach'] else 'managed'}"
            )
        return
    if args.command == "doctor":
        print(
            json.dumps(
                {
                    "python": sys.version.split()[0],
                    "appium": shutil.which("appium"),
                    "expected_appium": APPIUM_VERSION,
                    "expected_xcuitest": XCUITEST_VERSION,
                    "xcodebuild": shutil.which("xcodebuild"),
                    "config": str(store.config_path),
                    "hardware_verified": False,
                },
                indent=2,
            )
        )
        return
    name, profile = select(config, args.device)
    with store.lock(name) as directory:
        state_path = directory / "session.json"
        observation_path = directory / "observation.json"
        state = read(state_path, {})
        if state and (
            state["endpoint"] != profile["endpoint"] or state["udid"] != profile["udid"]
        ):
            raise Error(
                "Profile changed while session exists; restore profile before disconnecting"
            )
        if args.command == "connect":
            observation_path.unlink(missing_ok=True)
            return connect(profile, directory, state_path, wifi=args.wifi)
        client = Client(profile["endpoint"], state.get("session"))
        if args.command == "status":
            print(
                json.dumps(
                    {
                        "device": name,
                        "local_state": state,
                        "server": client.request("GET", "/status"),
                        "session_timeouts": client.command("GET", "/timeouts")
                        if client.session
                        else None,
                    },
                    indent=2,
                )
            )
            return
        if args.command == "invalidate":
            observation_path.unlink(missing_ok=True)
            print("References invalidated")
            return
        if args.command == "disconnect":
            observation_path.unlink(missing_ok=True)
            if client.session:
                try:
                    client.command("DELETE", "")
                except Error as exc:
                    if "invalid session id" not in str(exc):
                        raise  # Keep ownership state on uncertain failures.
            stop_owned_server(state)
            state_path.unlink(missing_ok=True)
            print("Disconnected owned session; attached servers are left running")
            return
        if args.command == "screenshot":
            path = directory / f"screenshot-{time.time_ns()}.png"
            path.write_bytes(
                base64.b64decode(client.command("GET", "/screenshot"), validate=True)
            )
            print(path)
            return
        if args.command == "apps":
            print(json.dumps(installed_apps(profile), indent=2))
            return
        if args.command == "screen":
            observation = take_observation(client, name, observation_path)
            result = None
        else:
            observation, result = action(args, client, name, observation_path)
        if args.json:
            output = public(observation)
            if result:
                output["action"] = result
            print(json.dumps(output, indent=2))
        else:
            if result:
                print(json.dumps(result) + "\n")
            print(markdown(observation))
        if (
            result
            and args.command == "tap"
            and args.expect
            and not result["navigation_verified"]
        ):
            raise Error(
                f"Tap accepted, but expected NavigationBar {args.expect!r} was not verified"
            )


def main():
    try:
        run(parser().parse_args())
    except (Error, OSError, ValueError, KeyError, subprocess.SubprocessError) as exc:
        print(f"ipad: {exc}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
