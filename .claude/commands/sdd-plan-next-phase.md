Plan the next work item for the project.

## Step 1: Check for clean state

Ensure the working tree is on a fresh, empty change with no pending modifications. If there are pending changes, use AskUserQuestion to ask whether to proceed anyway or abort.

## Step 2: Analyze the backlog

1. List all directories under `specs/` matching `YYYY-MM-DD-*/`.
2. Read the `README.md` in each to get its title, status (from frontmatter), and summary.
3. Categorize items by status: `idea`, `ready`, `in-progress`, `pr-submitted`, `done`.
4. Present a summary to the user showing the current state of the backlog.

## Step 3: Choose what to work on

If the user provided input via $ARGUMENTS, use that as a starting point.

Otherwise, use AskUserQuestion to help the user decide:
- Show `idea` and `ready` items as candidates
- Suggest which item to tackle next based on: dependencies between items, logical ordering, and project goals from `specs/mission.md`
- The user can also describe a new idea to create

## Step 4: Create or refine the work item

### If creating a new item:

1. Create `specs/YYYY-MM-DD-<slug>/README.md` with this structure:
   ```markdown
   ---
   status: idea
   ---
   # <Title>

   <One or two sentence description of the idea.>
   ```
2. Use AskUserQuestion to ask: should we refine this now or leave it as an idea for later?

### If refining an existing `idea` item (or a new item the user wants to refine now):

Use AskUserQuestion iteratively to gather:
- Detailed requirements
- Implementation approach (referencing `specs/tech-stack.md` for tech choices and `specs/mission.md` for design principles)
- Task breakdown with ordering
- Verification steps and acceptance criteria

Update the README.md to the full structure:
```markdown
---
status: ready
---
# <Title>

## Summary
<What this work item delivers and why it matters.>

## Requirements
- <Requirement 1>
- <Requirement 2>
- ...

## Implementation Plan
1. <Task group 1>
2. <Task group 2>
3. ...

## Verification
- [ ] <Verification step 1>
- [ ] <Verification step 2>
- ...

## Acceptance Criteria
- <Criterion 1>
- <Criterion 2>
- ...
```

## Step 5: Review

After writing, re-read the spec and check:
- Does the implementation plan align with `specs/mission.md` design principles?
- Does it use the tech stack from `specs/tech-stack.md` correctly?
- Are acceptance criteria testable and specific?
- Are there any gaps or ambiguities?

Fix straightforward issues directly. Use AskUserQuestion for anything with multiple valid options.
