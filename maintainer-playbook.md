# Open Source Maintainer Playbook

## Overview

This playbook aligns on a consistent process for maintaining MCP Toolbox, ensuring quality, and providing a positive experience for contributors.

## Background

Our team maintains the MCP Toolbox family of open source repositories on GitHub:

* [MCP Toolbox](https://github.com/googleapis/mcp-toolbox)
* [MCP Toolbox \- Python SDK](https://github.com/googleapis/mcp-toolbox-sdk-python)
* [MCP Toolbox \- JS SDK](https://github.com/googleapis/mcp-toolbox-sdk-js)
* [MCP Toolbox \- Go SDK](https://github.com/googleapis/mcp-toolbox-sdk-go)

This playbook establishes our guidelines for maintaining our repositories, aiming to bring clarity, efficiency, and predictability to our open-source operations.

## The Issue Lifecycle: From Report to Release

This section outlines the end-to-end pipeline for how a bug or feature request moves from initial report to a final release. Each step links to a more detailed section later in this playbook.

1. [**Identify Issue**](#identify-issue): An issue is opened in one of our GitHub repositories by a community member or a team member.

2. [**Triage**](#bug-triage-workflow): A team member acknowledges the issue, applies appropriate labels for categorization and priority, and verifies the report.

3. [**Resolution**](#resolution): The issue is assigned and work begins.

4. [**Review & Merge**](#handling-pull-requests): The PR undergoes a thorough review by the team. Once approved and all checks pass, a maintainer merges it into the main branch.

5. [**Release**](#releases): The merged changes are bundled into the next versioned release and published.

## Issue Workflow

### Identify Issue

We track open issues and PRs across all repositories so maintainers have visibility into what needs attention. Each team should monitor incoming work against the team's response and closure targets (SLOs).

There are 3 primary types of work that need to be addressed, in prioritized order:

1. Out of SLO (past the target response/closure time)
2. Near to SLO (approaching the target)
3. Untriaged

While we want to immediately remediate anything that is out of SLO, ideally issues and PRs would not get to that point.

Everyone should check their open issues/PRs at least **once a week**.

Current SLO targets:

| Type | Priority | Metric | Objective |
| :---- | :---- | :---- | :---- |
| Feature Request | P0 | Response | 5 days |
| Process | P0 | Response | 5 days |
| Bug / Customer Issue | P0 | Response | 2 days |
| | | Closure | 14 days |
| | P1 | Response | 7 days |
| | | Closure | 90 days |
| | P2 | Response | 30 days |

\* Response requires at least a response from the reviewer

\* Closure requires the issue/PR to be closed.

### Bug Triage Workflow

Once you have identified a bug, assess it and provide it with an **initial acknowledgement**.

#### Triage Checklist

* [ ] **Check for Duplicates:** Is this a known issue? If so, link to the original, thank the user, and close as duplicate referencing the other issue.
* [ ] **Verify Reproducibility:** Can we reproduce the reported bug with the information provided? If not, request more information.
* [ ] **Apply Labels:** Add the `Priority <>`, `Type <>`, and `Product <>` (if applicable) labels on the GitHub Issue / PR as deemed appropriate. SLOs are based on the "Priority" label. Add the `Status <>` label if applicable.
* [ ] **Assign/Unassign Owner:** Assign a team member to investigate further if necessary. If you are planning to work on the issue, keep yourself assigned and pull it into your sprint. If you are not planning to work on the issue, unassign yourself so that contributors are aware that the issue is not assigned.

#### **Labels**

* Types
  * Bug
  * Feature request (FR)
  * Questions
  * Docs \- requires additional documentation
  * Process \- regular workflow processes, may include testing, release, etc.
  * Cleanup \- System improvements, or internal cleanup/hygiene concern
* Priorities
  * P0
    * Bug \- Major functionality broken that renders a feature unusable.
      * Example: issues with the database connection or a tool consistently erroring
        * An extension fails to load, preventing users from accessing any of its tools.
        * A critical data plane tool consistently returns incorrect results, leading to data corruption.
    * FR \- Reduces friction, a high priority feature to extend major functionality
      * Example: prompt support
  * P1
    * Bug \- Critical feature breakage which impacts the next release.
      * Example: tool or extension doesn't work consistently
        * A newly added tool for creating instances sometimes times out, requiring manual retries.
        * The documentation for a key feature is outdated, causing confusion for developers.
    * FR \- Significant feature improvements or additions targeted for the next release.
      * Example: adding support for an additional authentication method
  * P2
    * Bugs should not be P2s
    * FR \- Nice to have.
      * Example: tweaks to tools
        * The error message for a common permission issue is unclear and could be more actionable.
        * A tool's output is verbose and could be summarized for better readability.
  * P3
    * Bugs should not be P3s
    * Feature requests that are open for contribution can be labeled P3.
      * Example: a request to add a new, non-critical feature to an existing extension.
* Product
  * Each product should have a label. Add labels in [`labels.yaml`](.github/labels.yaml) if one is missing. For every Google Cloud database, there must be a product label in [`.github/labels.yaml`](.github/labels.yaml) and routing configured in [`.github/blunderbuss.yml`](.github/blunderbuss.yml). These GitHub teams should be team-synced from MDB groups and have write access to the repo.
* Status
  * help wanted \- Unplanned work open for contributions from the community.
  * feedback wanted \- Waiting for feedback from community or issue author. If the contributor did not respond for \>60 days, we should just close the PR.
  * waiting for response \- Reviewer awaiting feedback or responses from author. If the contributor did not respond for \>60 days, we should just close the PR.

Here are some sample templates:

Acknowledging a Feature Request:

```text
Thanks for suggesting this feature! We appreciate you taking the time to provide this feedback. We've added this to our backlog for consideration. We can't provide a specific timeline for implementation right now, but we will update this issue with any progress. In the meantime, we welcome pull requests from the community if you are interested in contributing this feature yourself.
```

Needs More Information:

```text
Thanks for opening this issue! We are having trouble reproducing your problem with the information provided.

To help us investigate further, could you please provide:
- A minimal, reproducible code sample that demonstrates the issue.
- The full error message and stack trace.

We will close this issue in 14 days if we don't hear back. Thanks!
```

### Resolution

For bugs, this involves writing code to fix the problem. For features, it involves implementation. For PRs, it involves assessment and review of the features being added.

After triage, the issue should be assigned to a team member for resolution.
If an external contributor expresses interest in working on an issue, assign it
to them to prevent duplicate work.

Flaky tests that we don't own will not be the priority. We should prioritize tests we own. Try to push work back to the upstream product teams. If third-party tests are constantly flaky, consider removing them from the test suites and escalate to the appropriate point of contact.

If we are opening a PR for a Feature Request or a Bug, make sure to link the issue in the description.

For auto-generated PRs (e.g. dependency updates and release automation), make sure the tests and PR checks pass and merge them.

### Handling Pull Requests

#### Reviewer's Checklist

If you are reviewing a PR, here are a few things to consider:

* [ ] Does this PR have a corresponding issue? If so, is it linked?
* [ ] Does the PR's title and description clearly explain *what* it does and *why*?
* [ ] Are there any logic errors or edge cases that haven't been considered?
* [ ] Does this change introduce any breaking changes? If so, are they documented and necessary?
* [ ] Does the PR title follow our guideline?
* [ ] Does the code follow our style guide? (Run the linter)
* [ ] Does the PR include new tests for the added functionality or bug fix?
* [ ] Do the tests cover both happy paths and edge cases?
* [ ] If this changes how a user interacts with the code, is the `README.md` or relevant documentation updated?
* [ ] Are there clear code comments for any complex parts of the logic?
* [ ] Does this PR handle user input? If so, is it properly sanitized?
* [ ] Does it add any new dependencies? If so, have they been vetted?
* [ ] Add the `release candidate` label if it needs to be in the next release.

All the pre-submit tests should pass and the documentation change should be reviewed before approval.

#### Deploying Documentation Previews

Documentation preview links are generated automatically for PRs from branches
within the main repository. For PRs from external forks, previews are disabled
for security reasons, so a maintainer must deploy the preview:

1. **Inspect Changes:** Review the proposed changes in the PR to ensure they are
   safe and do not contain malicious code. Pay close attention to changes in the
   `.github/workflows/` directory.
1. **Deploy Preview:** Apply the `docs: deploy-preview` label to the PR to
   deploy a documentation preview.

#### Running Prebuilt Config Evals

Evals are not part of the pull request gate — they call real models against live
databases, so they run on a schedule and otherwise on request.

Apply the `evals: run` label to a PR that changes a prebuilt config, its
evalset, or the eval CI, then comment `/gcbrun` to start a build. Only the
configs that PR touched are evaluated. The label stays on, so later pushes
re-run them; remove it when you are done.

Unlike a docs preview, a run compiles and executes the PR's code against live
test infrastructure — review the diff first on anything from a fork. On a fork
PR `/gcbrun` also releases the integration test triggers waiting on it.

See [Adding Prebuilt Config Evals](./DEVELOPER.md#adding-prebuilt-config-evals)
for what the evalsets cover.

### Release Communication & Tracking

> For release mechanics — release types, the version-cut steps, supported binaries, and npm/PyPI publishing — see [Releasing](#releasing).

Once the PR is merged, make sure you leave a comment on the open issue making external contributors aware that the fix should be available in the upcoming version.

Here's an example:

```text
This has been resolved in PR #[PR number]. The fix will be available in our next release (vX.Y.Z). Thanks again to @[contributor-username] for the contribution! Closing this issue now.
```

Release PRs are created by release automation and assigned to a team member. Keep an eye out for those or re-assign them to another team member as necessary.

Release plan:

* MCP Toolbox: Generally two a month.
* SDKs: As deemed necessary.

## Maintainer Team

Team `@googleapis/senseai-eco` has been set as
[CODEOWNERS](.github/CODEOWNERS). The GitHub TeamSync tool is used to create
this team from MDB Group, `senseai-eco`. Additionally, database-specific GitHub
teams (e.g., `@googleapis/toolbox-alloydb`) have been created from MDB groups to
manage code ownership and review for individual database products.

## Releasing

### Release Types

Toolbox has two types of releases: versioned and continuous. It uses Google
Cloud project, `database-toolbox`.

* **Versioned Release:** Official, supported distributions tagged as `latest`.
  The release process is defined in
  [versioned.release.cloudbuild.yaml](.ci/versioned.release.cloudbuild.yaml).
* **Continuous Release:** Used for early testing of features between official
  releases and for end-to-end testing. The release process is defined in
  [continuous.release.cloudbuild.yaml](.ci/continuous.release.cloudbuild.yaml).
* **GitHub Release:** `.github/release-please.yml` automatically creates GitHub
  Releases and release PRs.

### How to Release a New Version

1. [Optional] If you want to override the version number, send a
   [PR](https://github.com/googleapis/mcp-toolbox/pull/31) to trigger
   [release-please](https://github.com/googleapis/release-please?tab=readme-ov-file#how-do-i-change-the-version-number).
   You can generate a commit with the following line: `git commit -m "chore:
   release 0.1.0" -m "Release-As: 0.1.0" --allow-empty`
1. [Optional] If you want to edit the changelog, send commits to the release PR
1. Before merging the release PR, update the version dropdowns so the versioned
   docs build picks up the new release:
   1. Add a `[[params.versions]]` block for the new version to both `hugo.toml`
      and `hugo.cloudflare.toml`.
   1. Remove the oldest version's `[[params.versions]]` block from
      `hugo.cloudflare.toml`, and delete that version's directory in the
      `cloudflare-pages` branch. This is required because Cloudflare only allows
      20,000 files per deployment.
1. Approve and merge the PR with the title “[chore(main): release
   x.x.x](https://github.com/googleapis/mcp-toolbox/pull/16)”
1. The
   [trigger](https://pantheon.corp.google.com/cloud-build/triggers;region=us-central1/edit/27bd0d21-264a-4446-b2d7-0df4e9915fb3?e=13802955&inv=1&invt=AbhU8A&mods=logs_tg_staging&project=database-toolbox)
   should automatically run when a new tag is pushed. You can view [triggered
   builds here to check the
   status](https://pantheon.corp.google.com/cloud-build/builds;region=us-central1?query=trigger_id%3D%2227bd0d21-264a-4446-b2d7-0df4e9915fb3%22&e=13802955&inv=1&invt=AbhU8A&mods=logs_tg_staging&project=database-toolbox)
1. Update the Github release notes to include the following table:
    1. Run the following command (from the root directory):

        ```
        export VERSION="v0.0.0"
        .ci/generate_release_table.sh
        ```

    1. Copy the table output
    1. In the GitHub UI, navigate to Releases and click the `edit` button.
    1. Paste the table at the bottom of release note and click `Update release`.
1. Post release in internal chat and on Discord.

#### Supported Binaries

The following operating systems and architectures are supported for binary
releases:

* linux/amd64
* darwin/arm64
* darwin/amd64
* windows/amd64
* windows/arm64

#### Supported Container Images

The following base container images are supported for container image releases:

* distroless

### How to Release the npm Package

MCP Toolbox is available as an npm package: [@toolbox-sdk/server](https://www.npmjs.com/package/@toolbox-sdk/server).

> [!NOTE]
> npm releases are automated through the **OSS Exit Gate** via the
> `publish-npm-to-ar` and `trigger-exit-gate` steps in
> [.ci/versioned.release.cloudbuild.yaml](.ci/versioned.release.cloudbuild.yaml).
> The versioned release pipeline pushes all six packages to the Exit Gate
> Artifact Registry (`us-npm.pkg.dev/oss-exit-gate-prod/mcp-toolbox--npm`) and
> uploads a `publish_all: true` manifest to
> `gs://oss-exit-gate-prod-projects-bucket/mcp-toolbox/npm/manifests/`, which
> triggers Exit Gate to publish externally to npmjs.org.
>
> If the npm portion fails after the Go binaries are already in GCS, retry
> just the npm steps without rebuilding binaries via
> [.ci/npm_retry.cloudbuild.yaml](.ci/npm_retry.cloudbuild.yaml) (invocation
> instructions are in the file header). The retry is idempotent — already-
> published packages are skipped.
>
> **PyPI releases** are automated through the same Exit Gate via the
> `publish-pypi-to-ar` and `trigger-exit-gate-pypi` steps. Each release
> builds five platform-tagged wheels (one per OS/arch) via
> [pypi/setup.py](pypi/setup.py) with `TOOLBOX_PLATFORM` set per wheel,
> uploads them all to `us-python.pkg.dev/oss-exit-gate-prod/mcp-toolbox--pypi`,
> then drops a manifest at
> `gs://oss-exit-gate-prod-projects-bucket/mcp-toolbox/pypi/manifests/` so
> Exit Gate publishes them to pypi.org via trusted publishing. PyPI-only
> retries: [.ci/pypi_retry.cloudbuild.yaml](.ci/pypi_retry.cloudbuild.yaml).
> Idempotency is handled by `twine upload --skip-existing`.
>
> The manual procedure below is retained as a fallback for when the automation
> is broken.

To release a new version manually, follow these steps:

**Pre-requisites**

- **npm Account**: Create an account at [npmjs.com](https://npmjs.com) if you haven't already.
- **2FA Setup:** Ensure Two-Factor Authentication is enabled on your npm account (required for publishing).
- **Permissions:** Request Editor access to the `@toolbox-sdk/` organization by pinging the current maintainers.

**Preparation**

- You will be publishing packages for the following OS/Architecture combinations:
  - `darwin/arm64` -> `server-darwin-arm64`
  - `darwin/x64` -> `server-darwin-x64`
  - `linux/x64` -> `server-linux-x64`
  - `win32/arm64` -> `server-win32-arm64`
  - `win32/x64` -> `server-win32-x64`

**Phase A: Release Platform-Specific Packages**

_Repeat the following steps for each of the 5 combinations listed above._

1. **Navigate to the package directory:**
   ```bash
   cd npm/server-<os>-<arch>
   ```
2. **Verify versioning:**
   - The toolbox binary version is sourced from `cmd/version.txt` at the repo root (the release-please `versionFile`); `downloadBinary.js` reads it from there during `prepack`. Verify it reflects the version to be released.
   - Open `package.json` and verify that the `"version"` field matches `cmd/version.txt`.
3. **Sync Lockfile:**
   ```bash
   npm install --force
   ```
4. **Clean Artifacts:** Remove any pre-existing binaries to ensure a clean pack.
   ```bash
   rm -rf bin/
   ```
5. **Pack and Publish:**
   ```bash
   npm pack .
   npm publish --access public
   ```
6. **Verify:** Check the npm registry to ensure the version is live at `https://www.npmjs.com/package/@toolbox-sdk/server-<os>-<arch>` before moving to the next package.

**Phase B: Release Main Package (@toolbox-sdk/server)**

Once all platform-specific packages are live, release the main wrapper package.

1. **Navigate to the main directory:**
   ```bash
   cd ../server
   ```
2. **Verify Versioning:**
   - Open `package.json` and verify the `"version"` field reflects the target version.
   - Verify that versions for dependencies in `"optionalDependencies"` match the new version for all 5 packages.
3. **Sync Lockfile:** (Before this step, all 5 dep packages need to be published to npm)
   ```bash
   npm install --package-lock-only
   ```
   1. Ensure that a node module entry for each package is present in `package-lock.json`.
   2. Ensure that the integrity hashes for all packages are updated. If not, delete the file and use the `Sync Lockfile` command to generate a new lockfile.
4. **Pack and Publish:**
   ```bash
   npm pack .
   npm publish --access public
   ```
5. **Verify:** Confirm the main package is live with the correct version at `https://www.npmjs.com/package/@toolbox-sdk/server`.

**Committing changes to the repo**

Once all packages have been successfully published, please create a Pull Request containing the updated `package-lock.json` files from all `npm/` subdirectories. Ensure that any additional changes made during the release process are also included in this PR. Finally, set the title of the PR to: `chore(main): release npm vX.Y.Z`.

> [!IMPORTANT]
> Do not commit the binaries to the repo.

**Troubleshooting**

- **Access Token Expired or Need Auth:** Run `npm login`. If the registry is not `https://registry.npmjs.org/`, update it via `npm config set registry https://registry.npmjs.org/` or by modifying your `.npmrc`.
- **Version Mismatches:** Do not re-publish the same version. Increment the patch version and release the new version following the steps above.
- **Deprecation (Preferred):** If a specific version is broken, mark it as deprecated: `npm deprecate <package_name>@<version> "critical bug fixed in vX.Y.Z"`.
- **Unpublishing (Nuclear Option):** Only possible if published within the last 72 hours using `npm unpublish <package-name>@<version>`. Note that this permanently burns the version number.

## Testing & Automation

### Automated Tests

Integration and unit tests are automatically triggered via Cloud Build on each
pull request. Integration tests run on merge and nightly.

#### Failure notifications

On-merge and nightly tests that fail have notification setup via Cloud Build
Failure Reporter [GitHub Actions
Workflow](.github/workflows/schedule_reporter.yml).

#### Trigger Setup

Configure a Cloud Build trigger using the UI or `gcloud` with the following
settings:

* **Event:** Pull request
* **Region:** global (for default worker pools)
* **Source:**
  * Generation: 1st gen
  * Repo: googleapis/mcp-toolbox (GitHub App)
  * Base branch: `^main$`
* **Comment control:** Required except for owners and collaborators
* **Filters:** Add directory filter
* **Config:** Cloud Build configuration file
  * Location: Repository (add path to file)
* **Service account:** Set for demo service to enable ID token creation for
  authenticated services

## Repo Setup & Automation

* .github/blunderbuss.yml - Auto-assign issues and PRs from GitHub teams. Use a
  product label to assign to a product-specific team member. For every Google Cloud
  database, ensure a corresponding product label exists in `.github/labels.yaml`
  and is added to `.github/blunderbuss.yml` (under both `assign_issues_by` and
  `assign_prs_by`). These GitHub teams should be team-synced from MDB groups and
  have write access to the repo.
* .github/renovate.json5 - Tooling for dependency updates. Dependabot is built
  into the GitHub repo for GitHub security warnings
* go/github-issue-mirror - GitHub issues are automatically mirrored into buganizer
* (Suspended) .github/sync-repo-settings.yaml - configure repo settings
* .github/release-please.yml - Creates GitHub releases
* .github/ISSUE_TEMPLATE - templates for GitHub issues
