## Shipping changes — /ship is MANDATORY

Every change in this repository ships through the `/traya-clickup-shipper:ship` skill, tied to a ClickUp task.
Before editing or committing ANY file here, invoke `/ship <clickup-task-id> <instruction>` — it owns branching,
the dual PRs (master + development, identical diff), the mandatory best-practices review, the compliance gates,
the committed graphify knowledge-graph update, and the ClickUp/Slack close-out. Do not hand-roll git, PR, or
ClickUp steps in this repo. If the task ID is missing, ask for it — never proceed without one.

## Behaviour source of truth

`docs/reference/inventory-*.md` (plus the source JS they cite) define every response field, copy string,
constant and error message. Never invent or "improve" copy. Fixes beyond spec §6.1 need a spec change first.
