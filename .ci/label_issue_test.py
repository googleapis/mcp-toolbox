# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Unit tests for label_issue.py. Run: python3 -m unittest discover -s .ci -p 'label_issue_test.py'"""

import json
import os
import tempfile
import unittest
from unittest import mock

import label_issue

ALLOWED = {
    "type": ["type: bug", "type: cleanup", "type: feature request"],
    "priority": ["priority: p0", "priority: p1", "priority: p2", "priority: p3"],
    "product": ["product: postgres"],
}


def answer(type_, priority, product):
    return json.dumps({"type": type_, "priority": priority, "product": product})


class AllowedLabelsTest(unittest.TestCase):
    def test_repo_labels_file_has_all_groups(self):
        allowed = label_issue.allowed_labels()
        for group in label_issue.GROUPS:
            self.assertTrue(allowed[group])
            self.assertTrue(all(name.startswith(f"{group}: ") for name in allowed[group]))

    def test_reads_quoted_and_unquoted_names(self):
        allowed = self.parse("- name: 'type: bug'\n- name: \"priority: p0\"\n- name: product: x\n- name: duplicate\n")
        self.assertEqual(allowed, {"type": ["type: bug"], "priority": ["priority: p0"], "product": ["product: x"]})

    def test_exits_when_a_group_is_missing(self):
        with self.assertRaises(SystemExit):
            self.parse("- name: 'type: bug'\n- name: 'priority: p0'\n")

    def parse(self, text):
        with tempfile.NamedTemporaryFile("w", suffix=".yaml", delete=False) as f:
            f.write(text)
        self.addCleanup(os.unlink, f.name)
        with mock.patch.object(label_issue, "LABELS_FILE", f.name):
            return label_issue.allowed_labels()


class CheckedLabelsTest(unittest.TestCase):
    def test_returns_the_three_labels(self):
        text = answer("type: bug", "priority: p1", "product: postgres")
        self.assertEqual(
            label_issue.checked_labels(ALLOWED, text), ["type: bug", "priority: p1", "product: postgres"]
        )

    def test_none_means_no_product_label(self):
        text = answer("type: bug", "priority: p1", "none")
        self.assertEqual(label_issue.checked_labels(ALLOWED, text), ["type: bug", "priority: p1"])

    def test_rejects_anything_else(self):
        rejected = {
            "label not on the list": answer("tests: run", "priority: p1", "none"),
            "none for a group other than product": answer("none", "priority: p1", "none"),
            "value that is not a string": answer(["type: bug"], "priority: p1", "none"),
            "missing field": json.dumps({"type": "type: bug", "priority": "priority: p1"}),
            "extra field": json.dumps(
                {"type": "type: bug", "priority": "priority: p1", "product": "none", "status": "x"}
            ),
            "not an object": json.dumps(["type: bug"]),
            "not JSON": "type: bug",
        }
        for name, text in rejected.items():
            with self.subTest(name), self.assertRaises(ValueError):
                label_issue.checked_labels(ALLOWED, text)


class PlanTest(unittest.TestCase):
    def test_adds_everything_to_an_unlabelled_issue(self):
        proposed = ["priority: p1", "product: postgres", "type: bug"]
        add, remove, skipped = label_issue.plan([], proposed)
        self.assertEqual(add, ["type: bug", "priority: p1", "product: postgres"])
        self.assertEqual((remove, skipped), ([], []))

    def test_replaces_a_type_the_template_set(self):
        add, remove, skipped = label_issue.plan(
            ["type: feature request"], ["type: cleanup", "priority: p2"], {"type: feature request"}
        )
        self.assertEqual(add, ["type: cleanup", "priority: p2"])
        self.assertEqual(remove, ["type: feature request"])
        self.assertEqual(skipped, [])

    def test_priority_follows_the_replaced_type(self):
        add, remove, _ = label_issue.plan(
            ["type: bug"], ["type: feature request", "priority: p3"], {"type: bug"}
        )
        self.assertEqual(add, ["type: feature request", "priority: p3"])
        self.assertEqual(remove, ["type: bug"])

    def test_keeps_a_type_a_maintainer_set_and_skips_the_mismatched_priority(self):
        add, remove, skipped = label_issue.plan(
            ["type: feature request"], ["type: bug", "priority: p0", "product: postgres"]
        )
        self.assertEqual(add, ["product: postgres"])
        self.assertEqual(remove, [])
        self.assertEqual([label for label, _ in skipped], ["type: bug", "priority: p0"])

    def test_same_type_is_left_alone(self):
        add, remove, skipped = label_issue.plan(["type: bug"], ["type: bug", "priority: p1"], {"type: bug"})
        self.assertEqual((add, remove), (["priority: p1"], []))
        self.assertEqual([label for label, _ in skipped], ["type: bug"])

    def test_never_replaces_priority_or_product(self):
        add, remove, skipped = label_issue.plan(
            ["type: bug", "priority: p0", "product: postgres"],
            ["type: bug", "priority: p1", "product: postgres"],
            {"type: bug"},
        )
        self.assertEqual((add, remove), ([], []))
        self.assertEqual(len(skipped), 3)

    def test_bug_never_gets_p2_or_p3(self):
        for priority in ("priority: p2", "priority: p3"):
            with self.subTest(priority):
                add, _, skipped = label_issue.plan([], ["type: bug", priority])
                self.assertEqual(add, ["type: bug"])
                self.assertEqual([label for label, _ in skipped], [priority])


class TemplateTypesTest(unittest.TestCase):
    def test_only_types_the_author_applied(self):
        def labeled(name, actor):
            return {"event": "labeled", "label": {"name": name}, "actor": {"login": actor}}

        replies = {
            "issues/7": {"user": {"login": "reporter"}},
            "issues/7/events?per_page=100": [
                labeled("type: bug", "reporter"),
                labeled("type: docs", "maintainer"),
                labeled("product: postgres", "reporter"),
                {"event": "assigned", "actor": {"login": "maintainer"}},
                labeled("type: cleanup", "reporter"),
                labeled("type: cleanup", "maintainer"),
            ],
        }
        with mock.patch.object(label_issue, "github", lambda method, path: replies[path]):
            self.assertEqual(label_issue.template_types(7), {"type: bug"})


if __name__ == "__main__":
    unittest.main()
