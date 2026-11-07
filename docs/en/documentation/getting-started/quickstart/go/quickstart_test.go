// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

var goldenKeywords = []string{"Hilton Basel", "Hyatt Regency", "book"}

func isTransientLLMError(output string) bool {
	lower := strings.ToLower(output)
	for _, marker := range []string{
		"error 503",
		"status: unavailable",
		"experiencing high demand",
		"error 429",
		"resource_exhausted",
		"overloaded",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func maxSampleAttempts() int {
	if raw := os.Getenv("SAMPLE_TEST_MAX_ATTEMPTS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 1 {
			return n
		}
	}
	return 3
}

func TestQuickstartSample(t *testing.T) {
	framework := os.Getenv("ORCH_NAME")
	if framework == "" {
		t.Skip("Skipping test: ORCH_NAME environment variable is not set.")
	}

	t.Logf("--- Testing: %s ---", framework)

	if framework == "openAI" {
		if os.Getenv("OPENAI_API_KEY") == "" {
			t.Skip("Skipping test: OPENAI_API_KEY environment variable is not set for openAI framework.")
		}
	} else {
		if os.Getenv("GOOGLE_API_KEY") == "" {
			t.Skipf("Skipping test for %s: GOOGLE_API_KEY environment variable is not set.", framework)
		}
	}

	sampleDir := filepath.Join(".", framework)
	if _, err := os.Stat(sampleDir); os.IsNotExist(err) {
		t.Fatalf("Test setup failed: directory for framework '%s' not found.", framework)
	}

	maxAttempts := maxSampleAttempts()
	backoff := 5 * time.Second
	var stdout, stderr bytes.Buffer
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		stdout.Reset()
		stderr.Reset()
		cmd := exec.Command("go", "run", ".")
		cmd.Dir = sampleDir
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err = cmd.Run()
		if err == nil {
			break
		}
		if attempt < maxAttempts && isTransientLLMError(stderr.String()+"\n"+stdout.String()) {
			t.Logf("Transient upstream LLM error on attempt %d/%d; retrying in %s...\n%s", attempt, maxAttempts, backoff, stderr.String())
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		break
	}
	actualOutput := stdout.String()

	if err != nil {
		t.Fatalf("Script execution failed with error: %v\n--- STDERR ---\n%s", err, stderr.String())
	}
	if len(actualOutput) == 0 {
		t.Fatal("Script ran successfully but produced no output.")
	}

	var missingKeywords []string
	outputLower := strings.ToLower(actualOutput)

	for _, keyword := range goldenKeywords {
		kw := strings.TrimSpace(keyword)
		if kw != "" && !strings.Contains(outputLower, strings.ToLower(kw)) {
			missingKeywords = append(missingKeywords, kw)
		}
	}

	if len(missingKeywords) > 0 {
		t.Fatalf("FAIL: The following keywords were missing from the output: [%s]", strings.Join(missingKeywords, ", "))
	}
}
