# ipad-cli

A small local CLI for physical iPads through Appium/XCUITest. Python 3.11+,
no Python runtime dependencies, no model/API key, and no Inspector dependency.
This is the first proof of concept; see [VALIDATION.md](VALIDATION.md) for actual evidence.

## Install and enroll

Run `./ipad` directly from this checkout, or install with `pip install -e .` in a virtualenv.
Use one shared Appium **3.8.0** and XCUITest **12.18.1** installation. Managed startup
checks both versions; attaching to a server requires its operator to verify the driver version.
If installing from scratch, the equivalent npm/Appium commands are:

```sh
npm install -g appium@3.8.0
appium driver install xcuitest@12.18.1
appium driver list --installed
xcodebuild -version
xcrun devicectl list devices
```

Use Xcode with the device's OS support installed. Pair/trust the Mac over USB,
enable Developer Mode and UI Automation on the iPad, and trust the developer
certificate when prompted. Configure an Apple Development identity in the macOS
Keychain and its team in Xcode. Profiles reference that identity; the CLI never
exports or duplicates private keys. Each device needs valid WDA provisioning.

```sh
./ipad configure sandman \
  --udid YOUR_DEVICE_UDID --platform-version YOUR_OS_VERSION \
  --team-id YOUR_TEAM --wda-bundle-id YOUR_PROVISIONED_BUNDLE_ID --default
./ipad devices
./ipad connect
./ipad screen
./ipad screenshot
```

Configuration and runtime data default to `~/.local/share/ipad-cli`. Set `IPAD_HOME`
or use `--home .local` for a private, gitignored checkout-local workspace. Enrollment
never writes device identities into source code. Generated state uses private file
permissions. Screenshots, observations, and logs may contain personal information;
keep them local and review any fixture before committing it.

## Observe and act

```sh
export IPAD_DEVICE=sandman
./ipad open com.apple.Preferences
./ipad screen --json
./ipad tap s0123456789abcdef:3 --expect Battery
./ipad type s0123456789abcdef:7 'search text'
./ipad swipe up
./ipad home
./ipad apps
./ipad status
./ipad invalidate
./ipad disconnect
```

Copy real references from the latest observation; the examples above are placeholders.
Actions return a fresh observation. JSON and Markdown share one model, including labels,
roles, values, context, enabled/selected state, and whether an element is considered
potentially actionable. A role is a heuristic, not a guarantee an app supports an action.
Static text is retained and cannot be tapped through references. Explicit visibility
flags on individual elements take precedence over their containers; Bluefire reports
a hidden collection whose book cells are visible. `type` appends through
WebDriver's native element-value command; it does not clear the field first.

`tap --expect LABEL` checks for an exact visible NavigationBar label in the resulting
snapshot and exits unsuccessfully if absent. Acceptance by Appium is reported separately
from that check. A matching navigation bar is modest evidence, not a general assertion
that the intended business outcome occurred. Other actions report acceptance only.

## Screen hierarchy

Markdown groups elements under indented sections reflecting their container hierarchy.
It uses app-supplied labels/names verbatim, including technical accessibility identifiers.
An unnamed semantic container falls back to its role, such as `ScrollView`. Anonymous
`Other` and `Window` layout wrappers are collapsed; empty sections are omitted. Static
text stays with its controls. Elements remain in source-tree order; grouping does not
claim reading priority or reorder events ahead of navigation.

JSON keeps the flat `elements` list and adds a normalized `groups` list:

```json
{
  "groups": [
    {"id": "g1", "parent_id": null, "role": "Application", "name": "Calendar", "label": "Calendar"},
    {"id": "g2", "parent_id": "g1", "role": "Other", "name": "kViewType1DayA11yID", "label": "kViewType1DayA11yID"}
  ],
  "elements": [
    {"ref": "sexample:1", "label": "Example event", "role": "Button", "group_id": "g2"}
  ]
}
```

This is a schema excerpt; actual elements retain values, states, bounds, and textual
`context`. `group_id` points to the closest retained container and may be null. Follow
`parent_id` to find ancestor regions, including when controls sit inside a nested table
or scroll view. Separate containers with identical names have distinct IDs. IDs are
local to one observation and are not tap references or persistent selectors.

When a group corresponds to a retained element, it also has a `ref` pointing to that
entry in `elements`. Markdown renders that reference once in the section heading.
Its normal role determines whether it can be tapped; a NavigationBar heading is still
context, while a Cell may be actionable. Named layout containers get headings without
invented element references. The full raw hierarchy remains in stale-state detection,
including collapsed wrappers and duplicate controls.

In the verified Google Calendar Day screen, `kViewMonthCalendarA11yID` contains the
month picker/grid while `kViewType1DayA11yID` contains the selected-day header and its
four entries. An agent can select that region through its name and follow its group
memberships without parsing Markdown or relying on screen coordinates. These names
come from that app version; they are not universal calendar roles. We do not infer
an event type, a friendly section name, or importance from an internal identifier.

## Installed apps

`ipad apps` uses Apple's `devicectl` inventory for the selected UDID on both USB and
Wi-Fi. It includes system, third-party, and other installed apps reported by CoreDevice,
without requiring an Appium session. Output is a sorted JSON object keyed by bundle ID:

```json
{
  "com.apple.mobiletimer": {"name": "Clock", "version": "1.1", "build": "1.0"}
}
```

This replaces the original raw Appium `mobile: listApps` output, which included many
platform-specific properties and defaulted to user apps. The smaller schema is now
the same across transports; installation/container paths are omitted. Appium's listing
command uses a separate installation-proxy device service. Attaching WDA over CoreDevice
Wi-Fi does not configure that service's discovery, so it can report a missing device
while screen/tap commands still work. UI observation and actions continue to use Appium.

## Reference and concurrency contract

References contain a random observation token plus a one-based element number. Internal
state binds them to the selected profile and exact Appium session. A new `screen` replaces
the previous observation, even if nothing changed. Reconnect, disconnect, explicit
`invalidate`, and attempted mutations invalidate old references. Failed/uncertain
mutations are never automatically repeated. If the mutation succeeds but reading state
fails, the error says so; run `screen` to recover.

Before a referenced action the CLI compares the current XML hierarchy fingerprint
(attributes and structure, including bounds) to the saved snapshot. It finds the target
with an exact native predicate, rejects zero or multiple matches, and compares the
hierarchy again before acting. There is no label-only, XPath, or coordinate fallback.
Empty name/label/value fields are explicit constraints (matching native nil or an empty
string). Native predicates also include the observed rectangle, so duplicate labels
at different positions can be distinguished within the current snapshot. This selects
an Appium element, not a coordinate tap. Native queries also preserve the explicit
`accessible` flag. Identical non-accessible button twins are omitted from observations
when an otherwise identical accessible target exists; both remain in the stale-state
fingerprint. If those constraints still match multiple
elements, the action is refused. JSON includes `bounds`; Markdown shows geometry for
unlabeled elements and repeated labels. An unlabeled cell stays unlabeled rather than
being assigned a guessed book title or purpose.

This does **not** make physical UI operations atomic: the screen can change between
validation and the action; changes that return to an identical hierarchy are not detected;
canvas/image changes may be invisible to accessibility. Conversely clocks, animations,
status values, or offscreen hierarchy changes can cause conservative stale-state rejection.
Take another observation rather than forcing an old reference. Screenshots provide a
visual fallback; coordinate tapping is intentionally deferred until the native loop works.

All commands for one device use a nonblocking file lock. A concurrent command receives
`device busy` without sending a request. Different devices have independent locks.
Only the latest observation is retained, so clients sharing a device must exchange the
latest references. Use one common IPAD_HOME for all clients. Separate homes or external
Inspector clients cannot be coordinated by these locks; do not run them against the same
device concurrently. Do not hand-edit profile transport/signing details during a session.

## Device isolation and server ownership

Selection: `--device NAME` → `IPAD_DEVICE` → configured default → sole profile.
Multiple profiles without a selection fail. Global flags precede the command:

```sh
./ipad --device kitchen screen
```

Configure each additional device with unique Appium, WDA-forwarding, and MJPEG ports:
`--port 4726 --wda-port 8102 --mjpeg-port 9102`. Duplicate UDIDs and any port collisions
are rejected. Each profile gets its own `devices/NAME/` directory, DerivedData, session
state, observation, screenshots, and Appium log.

Managed `connect` starts its own loopback-only Appium process after checking for occupied
ports. Check `lsof -i :PORT` first when diagnosing conflicts. `disconnect` deletes only the
session created by this CLI and signals only a server whose recorded PID, start time,
and command still match. State survives session startup failure for cleanup. If deletion
fails with an uncertain transport error, state is retained; inspect before retrying.
Appium's idle session timeout is one hour; the local command timeout is 45 seconds and
session creation timeout is 240 seconds. A client-side timeout does not cancel server work.
If startup times out on an attached server, inspect its log for a possibly created session
before attempting a new connection.

`configure ... --attach --port PORT` uses an existing dedicated server but still creates
its own session. It never adopts another client's session or stops the attached process.
Ensure the device is free first. Appium's `/status` readiness does not prove a device has
no active session. This prototype does not manage unrelated Inspector sessions.

Deleting a session does not guarantee WDA exits or the automation overlay disappears.
The CLI leaves WDA/provisioning installed for reuse; it does not uninstall apps or remove
certificates. If the overlay persists, the prior manual recovery was holding both iPad
volume buttons. Revalidate this with the current OS before relying on it operationally.

## Wi-Fi and recovery

Wi-Fi observation, screenshot capture, and referenced Settings navigation have been
verified with USB disconnected on the enrolled iPad. This uses a running WDA instance
bootstrapped over USB and Apple's existing CoreDevice network tunnel. Wireless cold
startup, sleep recovery, and network interruption recovery are not yet verified.

Pair/trust the device using Xcode first, and establish a normal USB session so WDA is
built, signed, and running. Keep the device unlocked. Then:

```sh
./ipad --home .local disconnect
# Unplug USB, leave iPad unlocked and reachable over Wi-Fi.
./ipad --home .local connect --wifi
./ipad --home .local screen
./ipad --home .local screenshot
```

`connect --wifi` asks `xcrun devicectl device info details` for the configured UDID's
current tunnel. It checks the returned hardware UDID, requires `localNetwork` transport
and a connected tunnel, and probes WDA at that tunnel's IPv6 address using the profile's
`wda_port`. It checks the WDA signing bundle against the profile before creating the
Appium session with `appium:webDriverAgentUrl`. Tunnel addresses are resolved on each
connection, not committed or guessed. This mode requires USB disconnected so the
transport check cannot silently pass on a wired connection. A normal `connect` uses
the usual Appium startup flow. Both modes initially open Settings.

This working path required no separate privileged Appium tunnel helper. Xcode/CoreDevice
pairing and its existing connected network tunnel are prerequisites. The CLI does not
install or supervise that system tunnel or a WDA launch daemon. If WDA is not reachable,
reconnect USB and run a normal session to bootstrap it again. `wda_port` must be the port
where that WDA instance listens; on the verified setup Appium's USB bootstrap used the
same local/remote port. This attachment does not take ownership of the WDA process.

In Wi-Fi mode the explicit MJPEG streaming capability is omitted: the driver otherwise
requires its separate port-forwarding machinery. HTTP screenshots still work. MJPEG
streaming/recording and other Appium device-service APIs are not certified on this path.
The CLI's app inventory uses CoreDevice separately, as described above. Driver warnings about unavailable optional MJPEG forwarding are expected.
Appium server, profile locks, session ownership, observations, and screenshots remain
isolated per device; physical two-device operation is still unverified.

An alternative driver-managed RemoteXPC route is documented upstream:

```sh
sudo appium driver run xcuitest tunnel-creation
```

It creates tunnel interfaces and a registry, requires administrator authentication, and
must remain running. It was not needed or started in this validation. If using it, follow
the installed version's guide and use the same Appium installation/home. Do not run the
ordinary CLI as root. The shared helper is not owned or stopped by this CLI.

After sleep or network loss, do not retry a possibly accepted mutation. Re-establish
transport, read `status`, invalidate references, then take a new `screen`; disconnect
and `connect --wifi` if the session is dead. A client-side timeout does not cancel server
work. Unlock a passcode-protected iPad manually. If cleanup fails, inspect ownership
state and the local Appium log rather than killing unrelated processes. Test these
recovery paths on real hardware before relying on them unattended.

Code 65 is a generic Xcode failure. Read the device's local Appium log and distinguish
certificate trust, expired provisioning, build failures, and missing transport. Free
Personal Team provisioning needs periodic renewal: renew signing/provisioning in Xcode,
then reconnect so WDA can be rebuilt. Do not change account/security/network settings as
a control demonstration.

## Development

```sh
python3 -m unittest discover -s tests -v
ruff check ipad_cli tests
ruff format --check ipad_cli tests
```

Tests use synthetic XML and fake transport; they make no network calls and are not hardware
proof. The code separates [transport](ipad_cli/transport.py), [state](ipad_cli/state.py),
[observation/rendering](ipad_cli/observation.py), and [CLI](ipad_cli/cli.py).
`doctor` currently reports prerequisite paths and expected versions; it does not certify
signing, connectivity, or wireless readiness.

API contracts were checked against the installed XCUITest 12.18.1 source:
`lib/desired-caps.ts`, `lib/execute-method-map.ts`, `lib/commands/{source,find,element}.ts`,
and Appium's `@appium/base-driver/lib/basedriver/commands/timeout.ts`.
Primary references:

- [WebDriver endpoints](https://appium.io/docs/en/latest/reference/api/webdriver/)
- [XCUITest capabilities](https://appium.github.io/appium-xcuitest-driver/latest/reference/capabilities/)
- [Native execute methods](https://appium.github.io/appium-xcuitest-driver/latest/reference/execute-methods/)
- [RemoteXPC tunnels](https://appium.github.io/appium-xcuitest-driver/latest/guides/remotexpc-tunnels-real-devices/)

- [Attach to running WDA](https://appium.github.io/appium-xcuitest-driver/latest/guides/attach-to-running-wda/)
