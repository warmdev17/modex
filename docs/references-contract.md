# Prompt references

`references/list` accepts optional `workspace` and boolean `force_reload`.
The workspace follows the existing bridge workspace validation. It calls native
`skills/list` with `cwds: [workspace]` and `forceReload`, plus read-only
`plugin/installed` with the same workspace.

```json
{"id":20,"method":"references/list","params":{"force_reload":true}}
```

The result contains `entries`, `supported_kinds: ["skill", "plugin"]`, and `issues`.
Each entry has `kind`, `name`, `path`, `marker`, `title`,
`description`, `scope`, `enabled`, and optional `plugin_id`. Only the exact
requested workspace is projected. Discovery error bodies and skill bodies
are omitted; `issues` counts reported discovery problems or excluded entries.
The catalogue is bounded to 2048 entries. Skills use absolute paths and `$name`
markers. Installed plugins use `plugin://<id>` paths and `@name` markers; their
marketplace is the scope. Uninstalled plugins are omitted. Disabled plugins,
unknown/unavailable availability and non-null disabled reasons cannot be selected.
Plugin discovery failure preserves skills, increments issues and omits `plugin`
from supported kinds. The host never returns raw discovery errors.

```json
{"id":21,"method":"turn/start","params":{"thread_id":"one","text":"$review inspect this","references":[{"kind":"skill","name":"review","path":"/home/user/.agents/skills/review/SKILL.md"}]}}
```

References are optional; at most eight selections are accepted. The host checks
the thread workspace, reloads the native catalogue, and requires an enabled exact
kind/name/path match. Unknown fields/kinds, null references, disabled/missing or
forged paths fail before turn dispatch. Identical selections are deduplicated;
same-name references of the same kind with different paths cannot be combined. Native turn inputs
include `{type: "skill", name, path}` for skills and `{type: "mention", name, path}`
for plugins alongside existing text and attachments. A failed or stale plugin
discovery never authorizes a plugin mention.
No prompts are retried automatically after an uncertain result.

Android binds selected entries to exact `$name` / `@name` markers in each session draft.
`@` opens suggestions at a token boundary and uses cursor position; emails and
noncollapsed selections do not trigger discovery UI. Removing a marker drops its
binding; removing a chip deletes its markers. Catalogue state clears on session
switch/disconnect. Request ID and workspace guards reject stale responses.
`skills/changed` and manual Refresh reload the catalogue. An older host may reject
this RPC; Android displays a discovery error without blocking ordinary chat.

## Adding another kind

Keep catalogue identity and composer bindings typed by `kind`. Implement a native
source projection and validation/input adapter for that kind in the host, then
advertise it in `supported_kinds` and supply its display metadata. Add native
contract and lifecycle tests before enabling execution. MCP tools, subagent runs
and plugin management need distinct contracts; a generic name/path does not
authorize any of them. Installed plugin skills already use the skill path.
