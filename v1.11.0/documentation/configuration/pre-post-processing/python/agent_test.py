# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import asyncio
import importlib
import os
from pathlib import Path

import pytest

ORCH_NAME = os.environ.get("ORCH_NAME")
module_path = f"python.{ORCH_NAME}.agent"
agent = importlib.import_module(module_path)

GOLDEN_KEYWORDS = [
    "AI:",
    "Loyalty Points",
    "POLICY CHECK: Intercepting 'update-hotel'",
]

# --- Execution Tests ---
class TestExecution:
    """Test framework execution and output validation."""

    @pytest.fixture(scope="function")
    def script_output(self, capsys):
        """Run the agent function and return its output."""
        asyncio.run(agent.main())
        return capsys.readouterr()

    def test_script_runs_without_errors(self, script_output):
        """Test that the script runs and produces no stderr."""
        assert script_output.err == "", f"Script produced stderr: {script_output.err}"

    def test_keywords_in_output(self, script_output):
        """Test that expected keywords are present in the script's output."""
        output = script_output.out
        print(f"\nAgent Output:\n{output}\n")
        missing_keywords = [kw for kw in GOLDEN_KEYWORDS if kw not in output]
        assert not missing_keywords, f"Missing keywords in output: {missing_keywords}"
