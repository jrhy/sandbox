import argparse
from contextlib import redirect_stdout
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from ipad_cli.cli import (
    action,
    run,
    stop_owned_server,
    wifi_wda_url,
    connect,
    ELEMENT_KEY,
    installed_apps,
    device_info,
)
from ipad_cli.observation import (
    markdown,
    observe,
    predicate,
    public,
    resolve,
    tree_model,
)
from ipad_cli.state import Store, read, select, validate, write
from ipad_cli.transport import Client, Error

FIXTURE = (Path(__file__).parent / "fixtures/settings.xml").read_text()


def profile(port=4725, udid="test-a"):
    return {
        "endpoint": f"http://127.0.0.1:{port}",
        "udid": udid,
        "wda_port": port + 1000,
        "mjpeg_port": port + 2000,
        "attach": True,
    }


class FakeClient:
    session = "session-a"

    def __init__(self, count=1, fail_click=False, source=FIXTURE):
        self.calls = []
        self.count = count
        self.fail_click = fail_click
        self.source = source

    def command(self, method, path, body=None):
        self.calls.append((method, path, body))
        if path == "/source":
            return self.source
        if path == "/elements":
            return [{ELEMENT_KEY: str(i)} for i in range(self.count)]
        if self.fail_click:
            raise Error("timeout")


class ObservationTests(unittest.TestCase):
    def setUp(self):
        self.obs = observe(FIXTURE, "sandman", "session-a")
        self.target = next(e for e in self.obs["elements"] if e["label"] == "Battery")

    def test_unchanged_reference(self):
        self.assertEqual(
            resolve(self.obs, self.target["ref"], "sandman", "session-a", FIXTURE),
            self.target,
        )

    def test_stale_scope_and_change(self):
        cases = [
            ("kitchen", "session-a", FIXTURE),
            ("sandman", "session-b", FIXTURE),
            ("sandman", "session-a", FIXTURE.replace("74%", "73%")),
        ]
        for device, session, source in cases:
            with self.subTest(device=device, session=session, source=source[-80:]):
                with self.assertRaises(Error):
                    resolve(self.obs, self.target["ref"], device, session, source)
        new = observe(FIXTURE, "sandman", "session-a")
        with self.assertRaises(Error):
            resolve(new, self.target["ref"], "sandman", "session-a", FIXTURE)

    def test_static_disabled_and_hidden(self):
        labels = [e["label"] for e in self.obs["elements"]]
        self.assertNotIn("Hidden", labels)
        for label in ("Battery level", "Low Power Mode"):
            target = next(e for e in self.obs["elements"] if e["label"] == label)
            with self.assertRaises(Error):
                resolve(self.obs, target["ref"], "sandman", "session-a", FIXTURE)

    def test_duplicate_context_and_renderings(self):
        duplicates = [e for e in self.obs["elements"] if e["label"] == "More"]
        self.assertEqual(len(duplicates), 2)
        self.assertNotEqual(duplicates[0]["context"], duplicates[1]["context"])
        self.assertNotEqual(duplicates[0]["ref"], duplicates[1]["ref"])
        self.assertIn(self.target["ref"], markdown(self.obs))
        output = public(self.obs)
        self.assertNotIn("session", output)
        self.assertNotIn("attributes", output["elements"][0])
        self.assertEqual(output["elements"][1]["ref"], self.target["ref"])

    def test_predicate_escapes(self):
        self.target["attributes"]["label"] = 'A "quote" \\ path'
        self.assertIn('label == "A \\"quote\\" \\\\ path"', predicate(self.target))
        self.assertIn('type == "XCUIElementTypeCell"', predicate(self.target))

    def test_explicit_child_visibility_overrides_hidden_container(self):
        source = """<AppiumAUT><XCUIElementTypeCollectionView type="XCUIElementTypeCollectionView" visible="false">
          <XCUIElementTypeCell type="XCUIElementTypeCell" label="Visible book" visible="true"/>
          <XCUIElementTypeCell type="XCUIElementTypeCell" label="Hidden book" visible="false"/>
          <XCUIElementTypeCell type="XCUIElementTypeCell" label="Inherited hidden"/>
          </XCUIElementTypeCollectionView></AppiumAUT>"""
        nodes, _ = tree_model(source)
        self.assertEqual([e["label"] for e in nodes], ["Visible book"])

    def test_accessible_button_twin_is_compacted_and_targeted(self):
        source = """<AppiumAUT><XCUIElementTypeButton type="XCUIElementTypeButton" name="Stopwatch" label="Stopwatch" visible="true" enabled="true" accessible="true" x="562" y="36" width="116" height="36">
        <XCUIElementTypeButton type="XCUIElementTypeButton" name="Stopwatch" label="Stopwatch" visible="true" enabled="true" accessible="false" x="562" y="36" width="116" height="36"/>
        </XCUIElementTypeButton></AppiumAUT>"""
        nodes, digest = tree_model(source)
        self.assertEqual(len(nodes), 1)
        self.assertEqual(nodes[0]["attributes"]["accessible"], "true")
        self.assertIn("accessible == 1", predicate(nodes[0]))
        # A non-accessible button remains usable when it has no exact accessible twin.
        nodes, changed = tree_model(
            source.replace('accessible="true"', 'accessible="false"', 1)
        )
        self.assertEqual(len(nodes), 2)
        self.assertNotEqual(digest, changed)
        self.assertIn("accessible == 0", predicate(nodes[0]))

    def test_empty_attributes_constrain_native_locator(self):
        target = {
            "attributes": {
                "type": "XCUIElementTypeCell",
                "name": "",
                "label": "",
                "value": "",
                "x": "16",
                "y": "102",
                "width": "147",
                "height": "196",
            }
        }
        query = predicate(target)
        for key in ("name", "label", "value"):
            self.assertIn(f'({key} == nil OR {key} == "")', query)
        for key, value in (
            ("x", "16.0"),
            ("y", "102.0"),
            ("width", "147.0"),
            ("height", "196.0"),
        ):
            self.assertIn(f"rect.{key} == {value}", query)

    def test_same_label_at_different_positions_has_distinct_locator(self):
        first = next(e for e in self.obs["elements"] if e["label"] == "More")
        second = {
            **first,
            "attributes": {
                **first["attributes"],
                "x": "180",
                "y": "102",
                "width": "147",
                "height": "196",
            },
        }
        first = {**first, "attributes": {**second["attributes"], "x": "16"}}
        self.assertNotEqual(predicate(first), predicate(second))
        self.assertIn('name == "More"', predicate(first))
        self.assertIn('name == "More"', predicate(second))

    def test_unlabeled_and_duplicate_display_geometry(self):
        source = """<AppiumAUT><XCUIElementTypeCell type="XCUIElementTypeCell" x="16" y="102" width="147" height="196"/>
          <XCUIElementTypeButton type="XCUIElementTypeButton" label="More" x="180" y="102" width="40" height="40"/>
          <XCUIElementTypeButton type="XCUIElementTypeButton" label="More" x="240" y="102" width="40" height="40"/>
          </AppiumAUT>"""
        observation = observe(source, "a", "session")
        output = markdown(observation)
        self.assertIn("Unlabeled Cell", output)
        for position in ("(16, 102)", "(180, 102)", "(240, 102)"):
            self.assertIn(position, output)
        self.assertEqual(
            public(observation)["elements"][0]["bounds"],
            {"x": 16, "y": 102, "width": 147, "height": 196},
        )

    def test_real_unlabeled_library_cell(self):
        source = (
            Path(__file__).parent / "fixtures/bluefire-library-real-sanitized.xml"
        ).read_text()
        nodes, _ = tree_model(source)
        self.assertEqual(len(nodes), 2)
        self.assertEqual(nodes[0]["label"], "")
        self.assertNotEqual(predicate(nodes[0]), predicate(nodes[1]))
        self.assertIn('(name == nil OR name == "")', predicate(nodes[0]))
        self.assertIn('name == "Example Book\nExample Author"', predicate(nodes[1]))
        self.assertEqual(nodes[0]["bounds"]["x"], 16)
        self.assertEqual(nodes[1]["bounds"]["x"], 172)

    def test_real_navigation_fixture(self):
        source = (
            Path(__file__).parent / "fixtures/fonts-real-sanitized.xml"
        ).read_text()
        nodes, _ = tree_model(source)
        self.assertEqual(
            [(e["role"], e["label"]) for e in nodes],
            [("NavigationBar", "Fonts"), ("Button", "General")],
        )
        self.assertEqual(nodes[1]["context"], ["Fonts"])

    def test_compact_navigation_row(self):
        source = """<AppiumAUT><XCUIElementTypeCell type="XCUIElementTypeCell" label="Fonts" visible="true">
          <XCUIElementTypeButton type="XCUIElementTypeButton" label="Fonts" visible="true"/>
          <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" label="Fonts" visible="true"/>
          <XCUIElementTypeButton type="XCUIElementTypeButton" label="chevron" visible="true"/>
          <XCUIElementTypeStaticText type="XCUIElementTypeStaticText" label="Installed fonts" value="2" visible="true"/>
          </XCUIElementTypeCell></AppiumAUT>"""
        nodes, digest = tree_model(source)
        self.assertEqual(
            [(e["role"], e["label"]) for e in nodes],
            [("Button", "Fonts"), ("StaticText", "Installed fonts")],
        )
        # Compaction must not hide a changed hierarchy from stale-state detection.
        self.assertNotEqual(
            tree_model(source.replace('label="chevron"', 'label="Changed icon"'))[1],
            digest,
        )

    def test_bad_xml(self):
        with self.assertRaises(Error):
            tree_model("<bad")

    def test_action_ambiguity_and_failure(self):
        for count in (0, 2):
            with self.subTest(count=count), tempfile.TemporaryDirectory() as directory:
                path = Path(directory) / "observation.json"
                write(path, self.obs)
                client = FakeClient(count=count)
                args = argparse.Namespace(
                    command="tap", reference=self.target["ref"], expect=None
                )
                with self.assertRaisesRegex(Error, "ambiguous"):
                    action(args, client, "sandman", path)
                self.assertFalse(any("/click" in c[1] for c in client.calls))
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "observation.json"
            write(path, self.obs)
            client = FakeClient(fail_click=True)
            with self.assertRaisesRegex(Error, "outcome unverified"):
                action(args, client, "sandman", path)
            self.assertFalse(path.exists())
            self.assertEqual(sum("/click" in c[1] for c in client.calls), 1)

    def test_action_fresh_state_and_expectation(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "observation.json"
            write(path, self.obs)
            args = argparse.Namespace(
                command="tap", reference=self.target["ref"], expect="Settings"
            )
            after, result = action(args, FakeClient(), "sandman", path)
            self.assertTrue(result["accepted"])
            self.assertTrue(result["navigation_verified"])
            self.assertNotEqual(after["observation"], self.obs["observation"])
            self.assertEqual(read(path), after)


class StateTests(unittest.TestCase):
    def test_wifi_tunnel_requires_device_identity_network_and_matching_wda(self):
        from copy import deepcopy
        from types import SimpleNamespace

        base = {
            "result": {
                "properties": {
                    "hardware": {"udid": "test-a"},
                    "connection": {
                        "transportType": "localNetwork",
                        "state": "connected",
                        "tunnelIPAddressString": "fd00::1",
                    },
                }
            }
        }
        cases = [
            (None, None, None),
            ("hardware", "udid", "other-device"),
            ("connection", "transportType", "wired"),
            ("connection", "state", "disconnected"),
            ("connection", "tunnelIPAddressString", "invalid"),
        ]
        for section, key, value in cases:
            info = deepcopy(base)
            if section:
                info["result"]["properties"][section][key] = value
            p = {**profile(), "wda_bundle_id": "test.wda"}
            with (
                self.subTest(section=section, key=key),
                patch(
                    "ipad_cli.cli.subprocess.run",
                    return_value=SimpleNamespace(returncode=0, stderr=""),
                ),
                patch("ipad_cli.cli.read", return_value=info),
                patch.object(
                    Client,
                    "request",
                    return_value={"build": {"productBundleIdentifier": "test.wda"}},
                ) as request,
            ):
                if section is None:
                    self.assertEqual(wifi_wda_url(p), "http://[fd00::1]:5725")
                    request.assert_called_once()
                else:
                    with self.assertRaises(Error):
                        wifi_wda_url(p)
                    request.assert_not_called()
        with (
            patch(
                "ipad_cli.cli.subprocess.run",
                return_value=SimpleNamespace(returncode=0, stderr=""),
            ),
            patch("ipad_cli.cli.read", return_value=base),
            patch.object(
                Client,
                "request",
                return_value={"build": {"productBundleIdentifier": "wrong.wda"}},
            ),
        ):
            with self.assertRaisesRegex(Error, "signing bundle"):
                wifi_wda_url(p)

    def test_connect_wifi_capability_and_session_ownership(self):
        p = {
            **profile(),
            "platform_version": "26.6.1",
            "team_id": "test-team",
            "wda_bundle_id": "test.wda",
        }
        for wifi in (False, True):
            with self.subTest(wifi=wifi), tempfile.TemporaryDirectory() as temporary:
                d = Path(temporary)
                with (
                    patch(
                        "ipad_cli.cli.wifi_wda_url",
                        return_value="http://[fd00::1]:5725",
                    ) as resolve_url,
                    patch.object(
                        Client, "request", return_value={"sessionId": "owned-session"}
                    ) as request,
                    redirect_stdout(io.StringIO()),
                ):
                    connect(p, d, d / "session.json", wifi=wifi)
                    capabilities = request.call_args.args[2]["capabilities"][
                        "alwaysMatch"
                    ]
                    if wifi:
                        resolve_url.assert_called_once_with(p)
                        self.assertEqual(
                            capabilities["appium:webDriverAgentUrl"],
                            "http://[fd00::1]:5725",
                        )
                        self.assertEqual(
                            capabilities["appium:wdaRemotePort"], p["wda_port"]
                        )
                        self.assertNotIn("appium:mjpegServerPort", capabilities)
                    else:
                        resolve_url.assert_not_called()
                        self.assertNotIn("appium:webDriverAgentUrl", capabilities)
                        self.assertEqual(
                            capabilities["appium:mjpegServerPort"], p["mjpeg_port"]
                        )
                    state = read(d / "session.json")
                    self.assertEqual(state["session"], "owned-session")
                    self.assertNotIn(
                        "server", state
                    )  # Attached server remains unowned.
                    self.assertEqual(state["udid"], p["udid"])

    def test_coredevice_apps_normalized_without_personal_paths(self):
        payload = {
            "apps": [
                {
                    "bundleIdentifier": "test.z",
                    "name": "Example",
                    "version": "1.2",
                    "bundleVersion": "12",
                    "url": "file:///private/personal-path",
                },
                {"bundleIdentifier": "test.a", "name": "Clock"},
            ]
        }
        with patch("ipad_cli.cli.device_info", return_value=payload) as info:
            apps = installed_apps(profile())
            info.assert_called_once_with(profile(), "apps", ("--include-all-apps",))
        self.assertEqual(list(apps), ["test.a", "test.z"])
        self.assertEqual(
            apps["test.z"], {"name": "Example", "version": "1.2", "build": "12"}
        )
        self.assertEqual(apps["test.a"]["version"], "")
        for payload in (
            {},
            {"apps": [{}]},
            {"apps": [{"bundleIdentifier": "same"}, {"bundleIdentifier": "same"}]},
        ):
            with (
                self.subTest(payload=payload),
                patch("ipad_cli.cli.device_info", return_value=payload),
            ):
                with self.assertRaises(Error):
                    installed_apps(profile())

    def test_coredevice_explicit_udid_and_failure(self):
        from types import SimpleNamespace

        with (
            patch(
                "ipad_cli.cli.subprocess.run",
                return_value=SimpleNamespace(returncode=0, stderr=""),
            ) as command,
            patch("ipad_cli.cli.read", return_value={"result": {"apps": []}}),
        ):
            self.assertEqual(
                device_info(profile(), "apps", ("--include-all-apps",)), {"apps": []}
            )
            argv = command.call_args.args[0]
            self.assertEqual(argv[argv.index("--device") + 1], "test-a")
            self.assertIn("--include-all-apps", argv)
            self.assertEqual(argv[argv.index("--timeout") + 1], "15")
        with patch(
            "ipad_cli.cli.subprocess.run",
            return_value=SimpleNamespace(returncode=1, stderr="device unavailable"),
        ):
            with self.assertRaisesRegex(Error, "device unavailable"):
                device_info(profile(), "apps")
        with (
            patch(
                "ipad_cli.cli.subprocess.run",
                return_value=SimpleNamespace(returncode=0, stderr=""),
            ),
            patch("ipad_cli.cli.read", return_value={}),
        ):
            with self.assertRaisesRegex(Error, "no result"):
                device_info(profile(), "apps")

    def test_apps_dispatch_does_not_use_appium_device_service(self):
        with tempfile.TemporaryDirectory() as directory:
            store = Store(directory)
            write(store.config_path, {"devices": {"a": profile()}})
            args = argparse.Namespace(home=directory, device="a", command="apps")
            with (
                patch("ipad_cli.cli.installed_apps", return_value={}) as apps,
                patch.object(Client, "execute") as appium,
                redirect_stdout(io.StringIO()),
            ):
                run(args)
                apps.assert_called_once_with(profile())
                appium.assert_not_called()

    def test_selection_precedence(self):
        config = {
            "default": "a",
            "devices": {"a": profile(), "b": profile(4726, "test-b")},
        }
        for explicit, env, expected in [
            ("b", {"IPAD_DEVICE": "a"}, "b"),
            (None, {"IPAD_DEVICE": "b"}, "b"),
            (None, {}, "a"),
        ]:
            with self.subTest(explicit=explicit, env=env):
                self.assertEqual(select(config, explicit, env)[0], expected)
        del config["default"]
        with self.assertRaises(Error):
            select(config, environ={})
        with self.assertRaises(Error):
            select(config, explicit="missing", environ={})

    def test_collisions(self):
        for key in ("udid", "endpoint", "wda_port", "mjpeg_port"):
            a, b = profile(), profile(4726, "test-b")
            b[key] = a[key]
            with self.subTest(key=key), self.assertRaises(Error):
                validate({"devices": {"a": a, "b": b}})

    def test_lock_isolation_and_private_files(self):
        with tempfile.TemporaryDirectory() as directory:
            store = Store(directory)
            with store.lock("a") as a, store.lock("b") as b:
                with self.assertRaisesRegex(Error, "busy"):
                    with store.lock("a"):
                        pass
                write(a / "session.json", {"session": "a"})
                write(b / "session.json", {"session": "b"})
                (a / "session.json").unlink()
                self.assertEqual(read(b / "session.json"), {"session": "b"})
                self.assertEqual((b / "session.json").stat().st_mode & 0o777, 0o600)

    def test_server_ownership(self):
        with (
            patch("ipad_cli.cli.fingerprint_process", return_value="different"),
            patch("os.kill") as kill,
        ):
            stop_owned_server({"server": {"pid": 123, "identity": "ours"}})
            stop_owned_server({})
            kill.assert_not_called()
        with (
            patch("ipad_cli.cli.fingerprint_process", side_effect=["ours", ""]),
            patch("os.kill") as kill,
        ):
            stop_owned_server({"server": {"pid": 123, "identity": "ours"}})
            kill.assert_called_once()

    def test_server_stop_waits_for_exit(self):
        with (
            patch("ipad_cli.cli.fingerprint_process", side_effect=["ours", "ours", ""]),
            patch("os.kill") as kill,
            patch("ipad_cli.cli.time.sleep") as sleep,
        ):
            stop_owned_server({"server": {"pid": 123, "identity": "ours"}})
            kill.assert_called_once()
            sleep.assert_called_once()

    def test_server_stop_timeout_preserves_ownership(self):
        with (
            patch("ipad_cli.cli.fingerprint_process", return_value="ours"),
            patch("os.kill"),
            patch("ipad_cli.cli.time.monotonic", side_effect=[0, 20]),
        ):
            with self.assertRaisesRegex(Error, "ownership state retained"):
                stop_owned_server({"server": {"pid": 123, "identity": "ours"}})

    def test_disconnect_only_selected_session(self):
        with tempfile.TemporaryDirectory() as directory:
            store = Store(directory)
            write(
                store.config_path,
                {"devices": {"a": profile(), "b": profile(4726, "test-b")}},
            )
            for name in ("a", "b"):
                p = profile() if name == "a" else profile(4726, "test-b")
                write(
                    store.root / "devices" / name / "session.json",
                    {**p, "session": name},
                )
            args = argparse.Namespace(home=directory, device="a", command="disconnect")
            with (
                patch.object(Client, "command") as command,
                patch("os.kill") as kill,
                redirect_stdout(io.StringIO()),
            ):
                run(args)
                command.assert_called_once_with("DELETE", "")
                kill.assert_not_called()
            self.assertEqual(
                read(store.root / "devices/b/session.json")["session"], "b"
            )
            self.assertFalse((store.root / "devices/a/session.json").exists())

    def test_transport_never_retries(self):
        from urllib.error import URLError

        client = Client("http://127.0.0.1:4725", "ours")
        with patch.object(
            client.opener, "open", side_effect=URLError("offline")
        ) as request:
            with self.assertRaisesRegex(Error, "outcome may be unknown"):
                client.command("POST", "/element/1/click", {})
            request.assert_called_once()


if __name__ == "__main__":
    unittest.main()
