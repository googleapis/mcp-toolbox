// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { describe, test, before, after } from "node:test";
import assert from "node:assert/strict";

import path from "path";
import { fileURLToPath } from "url";

const ORCH_NAME = process.env.ORCH_NAME;
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const orchDir = path.join(__dirname, ORCH_NAME);
const agentPath = path.join(orchDir, "agent.js");

const { main: runAgent } = await import(agentPath);

const GOLDEN_KEYWORDS = [
  "AI:",
  "Loyalty Points",
  "POLICY CHECK: Intercepting 'update-hotel'"
];

describe(`${ORCH_NAME} Pre/Post Processing Agent`, () => {
  let capturedOutput = [];
  let capturedErrors = [];
  let originalLog;
  let originalError;

  before(() => {
    originalLog = console.log;
    originalError = console.error;

    console.log = (...args) => {
      const msg = args.map(a => (typeof a === 'object' ? JSON.stringify(a, null, 2) : String(a))).join(' ');
      capturedOutput.push(msg);
    };
    
    console.error = (...args) => {
      const msg = args.map(a => (typeof a === 'object' ? JSON.stringify(a, null, 2) : String(a))).join(' ');
      capturedErrors.push(msg);
    };
  });

  after(() => {
    console.log = originalLog;
    console.error = originalError;
  });

  test("runs without errors and outputContainsRequiredKeywords", async () => {
    capturedOutput = [];
    capturedErrors = [];

    await runAgent();

    const actualErrors = capturedErrors.filter(err => !err.includes('Warning'));

    assert.equal(
        actualErrors.length, 
        0, 
        `Script produced stderr: ${actualErrors.join("\n")}`
    );

    const actualOutput = capturedOutput.join("\n");

    assert.ok(
      actualOutput.length > 0,
      "Assertion Failed: Script ran successfully but produced no output."
    );

    const missingKeywords = [];

    for (const keyword of GOLDEN_KEYWORDS) {
      if (!actualOutput.includes(keyword)) {
        missingKeywords.push(keyword);
      }
    }

    assert.ok(
      missingKeywords.length === 0,
      `Assertion Failed: The following keywords were missing from the output: [${missingKeywords.join(", ")}]`
    );
  });
});
