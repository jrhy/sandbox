`settings.xml` is synthetic and deliberately includes duplicate labels, hidden UI,
static text, and a disabled switch.

`fonts-real-sanitized.xml` is the Fonts navigation bar subtree captured from a real
physical iPad Settings session on October 28, 2026 (iPadOS 26.6.1, XCUITest 12.18.1).
Only that subtree is retained. Attributes are allowlisted to the parser's UI fields;
text is allowlisted to public navigation labels. The internal back-button identifier
is replaced with `Sanitized`. No device identifiers, account content, installed-app
list, screenshot, or operational logs are retained here. Original capture stays under
ignored `.local/evidence/`. This small fixture establishes the real name-vs-label
shape and duplicate title text; it is not evidence of complete app coverage.

`bluefire-library-real-sanitized.xml` retains the two visible cell nodes from a real
Bluefire Reader library capture on October 28, 2026. All nonempty name/label/value
content is replaced with `Example Book` / `Example Author`; no library titles or
personal contents are retained. It preserves the unnamed cell's absent attributes
and the cells' distinct geometry. The collection ancestor is preserved, including
its false visibility flag despite its explicitly visible cells. Cell children are
omitted. The later capture's layout differs from the initial report; tests use its
captured geometry.

`calendar-groups.xml` is synthetic. It models the named month/day containers observed
in Google Calendar, with anonymous wrappers, a nested scroll view, static text, and
an empty region. Dates and entries are invented. No actual calendar entries, account
identity, or original calendar XML are committed.
