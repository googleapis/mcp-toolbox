# How to contribute

We'd love to accept your patches and contributions to this project.

## Before you begin

### Sign our Contributor License Agreement

Contributions to this project must be accompanied by a
[Contributor License Agreement](https://cla.developers.google.com/about) (CLA).
You (or your employer) retain the copyright to your contribution; this simply
gives us permission to use and redistribute your contributions as part of the
project.

If you or your current employer have already signed the Google CLA (even if it
was for a different project), you probably don't need to do it again.

Visit <https://cla.developers.google.com/> to see your current agreements or to
sign a new one.

### Review our community guidelines

This project follows
[Google's Open Source Community Guidelines](https://opensource.google/conduct/).

## Contribution process

> [!NOTE]
> New contributions should always include both unit and integration tests.

All submissions, including submissions by project members, require review. We
use GitHub pull requests for this purpose. Consult
[GitHub Help](https://help.github.com/articles/about-pull-requests/) for more
information on using pull requests.

### Code reviews

* Within 2-5 days, a reviewer will review your PR. They may approve it, or request
changes.
* When requesting changes, reviewers should self-assign the PR to ensure
they are aware of any updates.
* If additional changes are needed, push additional commits to your PR branch -
this helps the reviewer know which parts of the PR have changed.
* Commits will be
squashed when merged.
* Please follow up with changes promptly.
* If a PR is awaiting changes by the
author for more than 10 days, maintainers may mark that PR as Draft. PRs that
are inactive for more than 30 days may be closed.

### Automated Code Reviews

This repository uses **Gemini Code Assist** to provide automated code reviews on Pull Requests. While this does not replace human review, it provides immediate feedback on code quality and potential issues.

You can manually trigger the bot by commenting on your Pull Request:

*   `/gemini`: Manually invokes Gemini Code Assist in comments
*   `/gemini review`: Posts a code review of the changes in the pull request
*   `/gemini summary`: Posts a summary of the changes in the pull request.
*   `/gemini help`: Overview of the available commands

## Guidelines for Pull Requests

1. Please keep your PR small for more thorough review and easier updates. In case of regression, it also allows us to roll back a single feature instead of multiple ones.
1. For non-trivial changes, consider opening an issue and discussing it with the code owners first.
1. Provide a good PR description as a record of what change is being made and why it was made. Link to a GitHub issue if it exists.
1. Make sure your code is thoroughly tested with unit tests and integration tests. Remember to clean up the test instances properly in your code to avoid memory leaks.

## Implementation Guides

For technical details on how to implement new features, please refer to the
[Developer Documentation](./DEVELOPER.md).

* [Adding a New Database Source](./DEVELOPER.md#adding-a-new-database-source)
* [Adding a New Tool](./DEVELOPER.md#adding-a-new-tool)
* [Adding Integration Tests](./DEVELOPER.md#adding-integration-tests)
* [Adding Documentation](./DEVELOPER.md#adding-documentation)
* [Adding Prebuilt Tools](./DEVELOPER.md#adding-prebuilt-tools)
* [Adding Prebuilt Config Evals](./DEVELOPER.md#adding-prebuilt-config-evals)

## Submitting a Pull Request

Submit a pull request to the repository with your changes. Be sure to include a
detailed description of your changes and any requests for long term testing
resources.

* **Title:** All pull request titles should follow the formatting of
  [Conventional
  Commit](https://www.conventionalcommits.org/) guidelines: `<type>[optional
  scope]: description`. For example, if you are adding a new field in postgres
  source, the title should be `feat(source/postgres): add support for
  "new-field" field in postgres source`.
  
  Here are some commonly used `type` in this GitHub repo.

  |     **type**    |                                **description**                                                        |
  |-----------------|-------------------------------------------------------------------------------------------------------|
  | Breaking Change | Anything with this type of a `!` after the type/scope introduces a breaking change.                   |
  | feat            | Adding a new feature to the codebase.                                                                 |
  | fix             | Fixing a bug or typo in the codebase. This does not include fixing docs.                              |
  | test            | Changes made to test files.                                                                           |
  | ci              | Changes made to the cicd configuration files or scripts.                                              |
  | docs            | Documentation-related PRs, including fixes on docs.                                                   |
  | chore           | Other small tasks or updates that don't fall into any of the above types.                             |
  | refactor        | Change src code but unlike feat, there are no tests broke and no line lost coverage.                  |
  | revert          | Revert changes made in another commit.                                                                |
  | style           | Update src code, with only formatting and whitespace updates (e.g. code formatter or linter changes). |

  Pull requests should always add scope whenever possible. The scope is
  formatted as `<scope-resource>/<scope-type>` (e.g., `sources/postgres`, or
  `tools/mssql-sql`).
  
  Ideally, **each PR covers only one scope**, if this is
  inevitable, multiple scopes can be separated with a comma (e.g.
  `sources/postgres,sources/alloydbpg`). If the PR covers multiple `scope-type`
  (such as adding a new database), you can disregard the `scope-type`, e.g.
  `feat(new-db): adding support for new-db source and tool`.

* **PR Description:** PR description should **always** be included. It should
  include a concise description of the changes, its impact, along with a
  summary of the solution. If the PR is related to a specific issue, the issue
  number should be mentioned in the PR description (e.g. `Fixes #1`).
