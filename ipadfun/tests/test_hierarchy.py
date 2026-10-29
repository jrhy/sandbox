from pathlib import Path
import unittest

from ipad_cli.observation import observe, public, markdown, resolve, group_path
from ipad_cli.transport import Error

SOURCE = (Path(__file__).parent / "fixtures/calendar-groups.xml").read_text()


class HierarchyTests(unittest.TestCase):
    def setUp(self):
        self.observation = observe(SOURCE, "test-device", "test-session")
        self.elements = self.observation["elements"]
        self.groups = self.observation["groups"]

    def test_named_regions_and_anonymous_semantic_containers(self):
        month = next(g for g in self.groups if g["name"] == "kViewMonthCalendarA11yID")
        day = next(g for g in self.groups if g["name"] == "kViewType1DayA11yID")
        scroll = next(g for g in self.groups if g["role"] == "ScrollView")
        self.assertEqual(scroll["parent_id"], month["id"])
        self.assertEqual(month["parent_id"], day["parent_id"])
        self.assertEqual(scroll["label"], "ScrollView")
        self.assertEqual(len(self.groups), 5)
        self.assertNotIn("Window", [g["role"] for g in self.groups])
        self.assertNotIn("Empty region", [g["name"] for g in self.groups])
        events = [e for e in self.elements if e["label"].startswith("Example event")]
        self.assertEqual(len(events), 2)
        self.assertTrue(all(e["group_id"] == day["id"] for e in events))
        day_buttons = [e for e in self.elements if e["label"] == "Monday, June 7"]
        self.assertEqual(
            [e["group_id"] for e in day_buttons], [scroll["id"], day["id"]]
        )
        self.assertIn("kViewType1DayA11yID", events[0]["context"])

    def test_grouped_markdown_order_and_every_reference_once(self):
        output = markdown(self.observation)
        for e in self.elements:
            self.assertEqual(output.count(f"[{e['ref']}]"), 1)
        self.assertLess(
            output.index("kViewMonthCalendarA11yID"),
            output.index("kViewType1DayA11yID"),
        )
        self.assertLess(
            output.index("Example event A"), output.index("Example event B")
        )
        self.assertIn("  - **kViewType1DayA11yID** — Other\n    - [", output)
        self.assertIn("All-day events", output)
        self.assertNotIn("Empty region", output)
        self.assertNotIn(
            " (Calendar", output
        )  # Hierarchy replaces repeated breadcrumb suffixes.
        navigation = next(g for g in self.groups if g["name"] == "Navigation")
        self.assertIn(f"**[{navigation['ref']}] Navigation**", output)

    def test_json_membership_points_to_existing_groups_and_elements(self):
        output = public(self.observation)
        groups = {g["id"]: g for g in output["groups"]}
        refs = {e["ref"] for e in output["elements"]}
        for g in groups.values():
            self.assertTrue(g["parent_id"] is None or g["parent_id"] in groups)
            if "ref" in g:
                self.assertIn(g["ref"], refs)
        for e in output["elements"]:
            self.assertIn(e["group_id"], groups)
            self.assertNotIn("attributes", e)
            self.assertFalse(any(k.startswith("_") for k in e))
        self.assertNotIn("session", output)
        self.assertNotIn("fingerprint", output)

    def test_same_named_sibling_groups_remain_distinct(self):
        source = """<AppiumAUT>
          <XCUIElementTypeOther type="XCUIElementTypeOther" name="Section">
            <XCUIElementTypeButton type="XCUIElementTypeButton" label="First"/>
          </XCUIElementTypeOther>
          <XCUIElementTypeOther type="XCUIElementTypeOther" name="Section">
            <XCUIElementTypeButton type="XCUIElementTypeButton" label="Second"/>
          </XCUIElementTypeOther></AppiumAUT>"""
        observation = observe(source, "a", "session")
        self.assertEqual(len(observation["groups"]), 2)
        self.assertNotEqual(
            observation["elements"][0]["group_id"],
            observation["elements"][1]["group_id"],
        )
        self.assertEqual(markdown(observation).count("**Section**"), 2)

    def test_hidden_container_with_visible_child_still_groups(self):
        source = """<AppiumAUT>
          <XCUIElementTypeCollectionView type="XCUIElementTypeCollectionView" visible="false">
            <XCUIElementTypeCell type="XCUIElementTypeCell" label="Book" visible="true"/>
          </XCUIElementTypeCollectionView></AppiumAUT>"""
        observation = observe(source, "a", "session")
        self.assertEqual(len(observation["elements"]), 1)
        self.assertEqual(observation["groups"][0]["label"], "CollectionView")
        self.assertEqual(
            observation["elements"][0]["group_id"], observation["groups"][0]["id"]
        )
        self.assertIn("**CollectionView**", markdown(observation))

    def test_group_labels_remain_bound_to_snapshot_and_arent_tap_refs(self):
        target = next(
            e for e in self.elements if e["label"].startswith("Example event A")
        )
        self.assertEqual(
            resolve(
                self.observation, target["ref"], "test-device", "test-session", SOURCE
            ),
            target,
        )
        with self.assertRaisesRegex(Error, "Screen changed"):
            resolve(
                self.observation,
                target["ref"],
                "test-device",
                "test-session",
                SOURCE.replace("kViewType1DayA11yID", "Different region"),
            )
        with self.assertRaisesRegex(Error, "Unknown or stale"):
            resolve(
                self.observation,
                target["group_id"],
                "test-device",
                "test-session",
                SOURCE,
            )

    def test_old_observation_rendering_and_bad_group_edges(self):
        import copy

        old = copy.deepcopy(self.observation)
        del old["groups"]
        for e in old["elements"]:
            del e["group_id"]
        self.assertIn(" (Calendar", markdown(old))
        with self.assertRaisesRegex(Error, "Invalid observation group"):
            group_path("missing", {})
        with self.assertRaisesRegex(Error, "Invalid observation group"):
            group_path("g1", {"g1": {"id": "g1", "parent_id": "g1"}})


if __name__ == "__main__":
    unittest.main()
