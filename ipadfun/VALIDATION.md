# Validation record — October 28, 2026

This is the first architectural proof of concept, not a claim that all hardware and lifecycle
acceptance criteria are complete.

## Verified on this Mac

- Appium 3.8.0, XCUITest 12.18.1, Xcode 27.1; one physical iPad Air (third generation),
  iPadOS 26.6.1. Device identity and signing details are in ignored local configuration.
- Initial inspection found an existing Appium/Inspector session on port 4723. It was
  left alone. After the interruption that server was no longer listening.
- The CLI started its own server on port 4725, with WDA forwarding 8101, MJPEG 9101,
  and a separate DerivedData directory, then created its own Settings session.
- Direct accessibility observation and PNG screenshot retrieval succeeded. A real
  General page initially produced 61 entries; removal of repeated cell/button/title
  and chevron representations reduced this to 21 entries. The full tree remains part
  of the stale-state fingerprint.
- A reference from that observation tapped the Fonts button. The fresh JSON result
  reported both action acceptance and an exact Fonts navigation-bar check passing.
- A fresh reference navigated back to General, with its navigation-bar check passing.
  Both JSON output and native Markdown output were retrieved through the CLI.
- A pre-navigation reference was rejected. After disconnect/reconnect, a reference
  from the prior session was rejected. These probes sent no tap after rejection.
- An additional direct, harmless Fonts navigation simulated an outside UI change
  while preserving the saved General observation. A subsequent CLI action rejected
  its reference with `Screen changed` before sending a mutation.
- The first immediate reconnect exposed a real shutdown race: disconnect signaled
  Appium but returned before its port was free. Disconnect now waits for the recorded
  process identity to disappear, with a bounded timeout that retains ownership state.
  Port probes allow address reuse to avoid treating TIME_WAIT as a live listener.
  A second immediate disconnect/reconnect passed after this fix.
- Real device connection details reported `wired` during successful screen/control
  checks, with an iPad present in the USB registry. These are USB results.

Original evidence (observations, screenshot, action responses, rejection responses,
and Appium log) stays under ignored `.local/`. Only the small allowlisted Fonts
navigation subtree is retained as a [real fixture](tests/fixtures/fonts-real-sanitized.xml).

## Deterministic checks

18 tests passed using `python3 -m unittest discover -s tests -v`. `ruff check` passed;
Python sources were formatted with `ruff format`.

Coverage includes selection precedence/ambiguity, cross-device and old-session refs,
changed hierarchy, old observations, duplicate labels and native-match ambiguity,
static/disabled elements, real navigation XML, compact rendering, invalid XML,
per-device lock/state isolation, private state permissions, attached server ownership,
selected-device disconnect, shutdown waiting/timeout, and non-retried mutation failures.
These are software checks, not physical two-device or Wi-Fi proof.

## Outstanding acceptance work

- The Bluefire Reader book-opening loop is now verified over USB; see the update below.
- Initial Wi-Fi investigation (superseded by the successful direct-tunnel test below):
  devicectl discovery reported `localNetwork`,
  but Appium logged a missing RemoteXPC tunnel registry and failed WDA forwarding
  attempts. A device authorization prompt was observed by the user. Startup eventually
  succeeded, but subsequent successful control was wired. No wireless claim follows.
- Sleep recovery and network interruption recovery remain unverified. No root tunnel
  helper was started; the successful wireless path below uses Apple's existing tunnel.
- Only one physical device was observed. Simultaneous physical two-device sessions
  and disconnecting one while the other remains usable remain outstanding.
- `type`, app activation/listing, Home, and swipe have verified API mappings and CLI
  implementations but have not been exercised on hardware during this slice.
- WDA overlay cleanup, Personal Team renewal/expiration, and uncertain attached-server
  startup recovery still need hardware exercises. Coordinate tapping is deferred.

## Repeat the USB smoke check

Use the enrolled local profile (`./ipad --home .local ...`) or create your own as
explained in [README.md](README.md). Run `screen`, locate a harmless Settings navigation
button, and `tap REF --expect PAGE`. Check the JSON `accepted` and
`navigation_verified` fields independently. Take a screenshot; use old references
after navigation and reconnect and confirm nonzero exit without another action.
Do not automate account, security, or network setting changes for validation.

## Bluefire targeting correction

A user-reported unnamed library cell matched both itself and a named book cell.
The cause was verified in the saved observation and locator code: empty attributes
were dropped, broadening the unnamed cell query to all visible/enabled cells.
Locators now explicitly constrain empty fields to native nil/empty and include the
snapshot rectangle. Geometry is used to locate an element ID, not to issue a
coordinate tap. Zero/multiple-match rejection and both hierarchy checks remain.
JSON exposes bounds; Markdown identifies unnamed and duplicate-label elements with
geometry. A sanitized real two-cell fixture and four regression tests were added.
All 22 deterministic tests, lint, and formatting checks passed.

Live end-to-end verification of this correction is outstanding: a later source
capture showed a hidden collection with visible cell children; native queries
returned no cells, and screenshot/lock-state requests failed with a WDA proxy
`socket hang up`. No book taps were sent during this investigation. Restore a
responsive session and take a fresh observation before testing the corrected tap.


## Bluefire USB follow-up

After the user reconnected USB, native searches returned both library cells.
The screenshot confirmed that the first, unnamed cell is the music book before
Treasure Island. Rendering had an additional bug: it propagated a collection's
`visible=false` to cells that XCTest explicitly marked visible. The screenshot
confirmed that those cells really were on screen. Explicit per-element visibility
now wins; only absent visibility flags inherit from their container.

A fresh observation exposed the unnamed cell, and `ipad tap REF --json` successfully
opened it through an exact native element match. The returned hierarchy changed to
`BluefireReader.Reader`; a fresh screenshot showed the intended music textbook.
This validates the empty-field/geometry locator fix and the observation/action loop
in Bluefire Reader over USB. The title was read visually, not supplied by Bluefire's
accessibility label. Personal book content and screenshots remain local.

The real sanitized fixture now retains the collection's false visibility flag and
its two visible child cells. All 23 deterministic tests, lint, and formatting checks
passed. Wireless testing is next and requires confirmed USB disconnection.


## Wi-Fi-only observation/action — verified

The user confirmed USB was physically disconnected and the iPad was unlocked.
`devicectl device info details` reported `transportType: localNetwork` and an active
CoreDevice tunnel. The existing USB-forwarded Appium session failed with a WDA proxy
socket error, so it was not presented as a usable wireless session.

A direct `/status` probe of WDA through that device's CoreDevice IPv6 tunnel succeeded.
The CLI now supports `connect --wifi`, resolving the tunnel per configured UDID and
checking hardware identity, network transport, connected state, and the WDA signing
bundle. It attaches using the documented `appium:webDriverAgentUrl` capability.

The first attachment failed because an explicitly requested `mjpegServerPort` requires
additional forwarding. Installed source (`commands/helpers/mjpeg.ts`) confirms this is
fatal only when explicitly requested. Wi-Fi mode now omits that optional capability;
HTTP screenshots remain available. A new CLI-owned session then connected successfully.

With USB still disconnected, the CLI retrieved accessibility JSON and a PNG screenshot,
tapped the referenced Settings Battery button, and returned a fresh observation with
`accepted: true` and `navigation_verified: true` for Battery. This establishes actual
wireless control on this setup. No privileged Appium tunnel helper was started.

The session reused WDA already running from the USB bootstrap. Wireless cold launch,
network interruption recovery, sleep recovery, streaming, and device-service APIs are
not covered by this result. The network session remains available for continued use.
The tunnel address, device identity, screenshots, and logs are retained only in local
configuration/evidence. Two new deterministic tests cover tunnel selection/refusal and
USB-vs-Wi-Fi capability/session handling; all 25 tests passed with lint/format checks.


## Wi-Fi app inventory and Clock follow-up

The user reported that `connect --wifi` succeeded but `apps` reported the UDID missing.
Installed XCUITest source confirms that `mobile: listApps` uses InstallationProxyClient,
which falls back to legacy usbmux when its RemoteXPC service facade is unavailable.
The direct WDA/CoreDevice tunnel session does not configure that separate service path.
This error therefore does not imply a lost WDA UI connection.

`devicectl device info apps --device UDID --include-all-apps` successfully listed 95
apps over the current network connection. `ipad apps` now uses that read-only provider
on both transports, retaining a stable bundle-ID map with name/version/build only.
It no longer calls Appium's installation-proxy service or requires an Appium session.
The repaired CLI command returned the same app count and correctly identified Clock.
The original installation-service error was verified from its literal installed-source
error path; no transport/session restart was needed to fix inventory.

Clock was opened with its inventoried bundle ID. Its tab buttons appear as accessible
buttons plus otherwise identical non-accessible twins at the same bounds. Observations
now omit those redundant twins; exact native predicates include the accessible flag.
The full snapshot fingerprint still retains every node. A referenced Stopwatch tap
succeeded, and the fresh JSON showed its selected button value `1`, zero elapsed time,
and Lap/Start controls. No stopwatch, timer, or alarm was started or edited.

`--expect Stopwatch` correctly failed its narrow navigation-bar assertion: Clock's
Stopwatch view has no NavigationBar. The UI change was verified from the fresh controls
instead; the CLI did not mistake action acceptance for that missing assertion.
All 29 deterministic tests passed, including inventory normalization/schema/error
handling, explicit device selection for CoreDevice, Appium-independent apps dispatch,
and accessible twin compaction/targeting. Lint and formatting checks passed.

The referenced return tap from Stopwatch to World Clock also succeeded, with
`navigation_verified: true` for its World Clock navigation bar. The session was
left on World Clock.


## Container hierarchy and grouped observations

The user requested sections for groups of tappable elements. The observation model now
retains named regions (including `Other` accessibility containers) and semantic container
roles with children. Anonymous layout-only wrappers are collapsed. JSON retains its flat
`elements` list and adds `groups`, whose parent IDs preserve the retained hierarchy;
each element has a nullable `group_id`. A group backed by a retained real element has
that element's reference. Markdown renders those groups as indented sections, printing
every real reference exactly once and retaining static text alongside controls.
Source-tree order is preserved, with no claim of VoiceOver order or inferred priority.

The saved Google Calendar capture parsed into nine groups with all 69 existing elements
retained. The month container is separate from the day container. The day container has
five direct members: its date header and four entries. The same counts and group
memberships were verified through a fresh live `ipad screen` over the current session.
Raw identifiers are retained verbatim; no app-specific friendly-name mapping was added.
No screen mutation was needed for verification. Personal data remains under `.local/`.

Seven additional deterministic tests cover named/anonymous semantic grouping, parent
and element membership consistency, source order, exactly-once reference rendering,
same-named sibling containers, hidden containers with visible children, static text,
empty-group pruning, snapshot changes to container names, non-actionable group IDs,
and old-observation rendering. All 36 tests passed; lint and formatting passed.
The calendar grouping fixture is synthetic and contains no actual entries.
