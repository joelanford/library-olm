# Conventions

## Commit Messages

- Use [Conventional Commits](https://www.conventionalcommits.org/) format: `<type>: <subject>`
- Imperative mood, max 72 characters for the subject line
- Body (after blank line) explains **why**, not what. Required for non-obvious changes; optional for trivial ones
- One logical change per commit — separate refactors from features from renames

### Types

| Type | When to use |
|---|---|
| `feat` | New functionality or public API addition |
| `fix` | Bug fix |
| `refactor` | Code restructuring without behavior change |
| `test` | Adding or updating tests only |
| `docs` | Documentation changes only |
| `chore` | Maintenance (dependency updates, CI, tooling) |

### Examples

```
feat: add FromFS parser for Helm bundles
```

```
fix: handle nil CertificateProvider in webhook rendering

The renderer panicked when no CertificateProvider was set but the
bundle contained conversion webhooks. Now it returns a clear error.
```

```
refactor: extract webhook validation into dedicated validator
```

## Pull Requests

- **Title:** conventional commit format matching the primary change (e.g. `feat: add Helm bundle support`)
- **Body template:**
  ```
  ## Summary
  <what changed and why>

  ## Test plan
  <how this was verified — new tests, manual checks, etc.>

  Spec: specs/YYYY-MM-DD-<slug>/  (if applicable)
  ```
- Prefer single-commit PRs. If a PR has multiple commits, each should be independently reviewable and pass tests
- Draft PRs for work-in-progress; mark ready when tests pass and spec acceptance criteria are met

## Branch Naming

- `YYYY-MM-DD-<slug>` to match a work item directory name
- `<descriptive-slug>` for ad-hoc work without a spec
- No long-lived feature branches — keep PRs small and merge often

## Test Coverage

- Overall project coverage must not decrease
- New code must have at least 70% statement coverage

## Code Review

- All changes go through PR review before merging
- Reviewer checks: tests pass, public API is intentional, no unnecessary dependencies added, legacy dependency usage is minimized
