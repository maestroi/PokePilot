# Remote MCP control plane

PokePilot exposes an optional remote [Model Context Protocol](https://modelcontextprotocol.io/) endpoint from `pokeui` so an LLM host can queue runs, inspect progress, cancel a run, and hand interesting failures to the existing triage flow.

The endpoint is deliberately a thin operator adapter. `pokewall` remains the orchestrator and runners still speak the private farm protocol.

## Enable it

MCP is **off by default**. `pokeui` only mounts `/mcp` when `POKEPILOT_MCP_TOKEN` is non-empty.

Generate a long random bearer token and keep it outside Git:

```sh
printf 'POKEPILOT_MCP_TOKEN=%s\n' "$(openssl rand -hex 32)" >> ~/.config/pokepilot/env
make farm-up
```

`make farm-up` already sources `~/.config/pokepilot/env` and `.env`; the Swarm stack passes `POKEPILOT_MCP_TOKEN` only to `pokeui`.

For the public deployment, point the existing reverse proxy at `pokeui` as usual. The MCP URL is then:

```text
https://admin.rompilot.app/mcp
```

The previous private hostname `https://pokemon.labstack.cc/mcp` remains a compatibility alias until that host is retired.

Every MCP request must include:

```text
Authorization: Bearer <POKEPILOT_MCP_TOKEN>
```

Treat that token as a compute-control credential: holders can start and cancel runs. Rotate it if it is exposed. An empty token removes the `/mcp` route entirely.

## Tools

| Tool | Effect |
|---|---|
| `pokepilot_start_run` | Queue one finite run. Defaults to `llm`, Squirtle, and `Earn the Boulder Badge.` |
| `pokepilot_list_runs` | List recent runs, optionally filtered by `queued`, `leased`, `running`, or `done`. Finished failures carry their linked issue status/resolution when known. |
| `pokepilot_get_run` | Read one run's live/finished state, including planner state, party and LLM stats when available |
| `pokepilot_cancel_run` | Request cooperative cancellation |
| `pokepilot_get_triage` | Read the actionable grouped failures. Resolved issue groups are hidden by default; pass `include_resolved: true` to audit history. |
| `pokepilot_investigate_failure` | Trigger the existing failure-investigation handoff |
| `pokepilot_get_run_debug` | Read the compact finish/trace/progress/artifact-reference bundle for one run |
| `pokepilot_get_run_recovery_audit` | Read one run's recovery-focused audit packet: full bounded recovery/failure activity (without the normal 40-event MCP compaction), attempt revisions when available, and related triage groups including resolved history |
| `pokepilot_get_run_artifacts` | List one run's artifact names/media types/storage references, without bytes |
| `pokepilot_get_run_artifact_content` | Fetch one small artifact's bytes as base64 (a `.state`/`.ram`/JSON checkpoint) for local reproduction, whether pokewall still holds it inline or has durabilized it to S3; artifacts too large for the MCP response cap (e.g. `run.gbrun`) still need the operator UI/replay service |

MCP intentionally does **not** expose runner leases, heartbeats, finish/checkpoint uploads, worker registration, run deletion, endless runs, arbitrary HTTP, Docker, or Swarm controls.

### Run recovery audits

`pokepilot_get_run_recovery_audit(run_id)` is the run-centric companion to failure triage. It intentionally reads the wall's raw debug timeline before MCP's normal event compaction, then returns only recovery/failure-relevant activity plus the triage groups that reference that run. Resolved groups are included because they are required to distinguish stale pre-fix evidence from a real post-fix regression.

This tool does not decide that a recovery is a bug and does not create work. Agents should collapse duplicate evidence by stable triage fingerprint first, use runner/fixed revisions to prove stale-vs-regression status, and keep expected gameplay (for example an ordinary battle loss) and infrastructure rollover separate from software-defect recovery. The repository workflow is documented in `.claude/skills/pokefarm-recovery-audit/SKILL.md`.

### Failure resolution and history

`pokewall` already stores the stable failure fingerprint and its linked Agent Orchestrator issue. The issue status, resolution, occurrence count, and fixed revision are synchronized back into that link. MCP now treats those values as the durable resolution signal instead of treating every old failed run as fresh work.

`pokepilot_get_triage` is therefore the work queue: groups whose linked issue is resolved (for example `resolution: fixed`) are annotated `actionable: false` and omitted from the default response. `resolved_hidden` reports how many historical groups were suppressed. `include_resolved: true` returns everything for audits and debugging while preserving the `actionable` annotation and `resolution_state` / `fixed_revision` metadata.

A later occurrence can reopen the linked issue. Active states such as `open`, `reopened`, or `investigating` take precedence over an old resolution value, so the same fingerprint becomes actionable again as a regression rather than silently staying hidden.

`pokepilot_list_runs` remains raw run history. It now preserves each run's linked issue metadata, so an LLM inspecting old runs can see that a matching failure was already fixed. Do not build a new work queue by grouping historical `detail` strings; use `pokepilot_get_triage` for that.

## Example prompt

Once your LLM host has the server attached, a useful first request is:

```text
Start three Squirtle runs whose goal is badges:1 with seeds 1, 2, and 3.
Give each 40 rounds. Compare their progress and failures; do not cancel a run
unless I ask you to.
```

The model can call `pokepilot_start_run` three times, keep the returned run IDs, and use `pokepilot_get_run` / `pokepilot_list_runs` to inspect them later.

## Generic remote-MCP configuration

MCP clients use different config file names, but hosts that accept a remote Streamable HTTP server generally need these three values:

```json
{
  "name": "pokepilot",
  "url": "https://admin.rompilot.app/mcp",
  "headers": {
    "Authorization": "Bearer ${POKEPILOT_MCP_TOKEN}"
  }
}
```

Do not paste the real token into a committed config file; use the client's secret/environment mechanism.

## Claude Messages API example

Anthropic's current remote MCP connector can let Claude call the tools directly from the Messages API. The bearer value is passed as `authorization_token`:

```python
import os
import anthropic

client = anthropic.Anthropic()

response = client.beta.messages.create(
    model="claude-opus-5",
    max_tokens=2000,
    messages=[{
        "role": "user",
        "content": (
            "Start a Squirtle run with goal badges:1 and max 40 rounds. "
            "Return the run id, then inspect its current status."
        ),
    }],
    mcp_servers=[{
        "type": "url",
        "url": "https://admin.rompilot.app/mcp",
        "name": "pokepilot",
        "authorization_token": os.environ["POKEPILOT_MCP_TOKEN"],
    }],
    tools=[{
        "type": "mcp_toolset",
        "mcp_server_name": "pokepilot",
    }],
    betas=["mcp-client-2025-11-20"],
)

print(response)
```

See Anthropic's MCP connector documentation for the client-side beta/API details; PokePilot itself only depends on standard Streamable HTTP MCP.

## Protocol-level smoke client

The repository also contains `examples/mcp-client`, which uses the same official Go MCP SDK as the server. It is useful before involving a model:

```sh
POKEPILOT_MCP_URL=https://admin.rompilot.app/mcp \
POKEPILOT_MCP_TOKEN='...' \
  go run ./examples/mcp-client
```

It connects with the bearer token, lists the server tools, and exits without starting a run.

## Security boundary

`pokeui` remains the only public-facing process. `/mcp` talks to `pokewall` through a fixed set of operator calls; it cannot proxy an arbitrary path. The Streamable HTTP handler is stateless, request bodies are bounded, cross-origin browser requests are protected, and bearer comparison is constant-time.

`admin.rompilot.app` sits behind Cloudflare Access for its plain `/v1/*` browser routes, including `GET /v1/runs/{id}/artifacts/{name}/content`; an automated agent holding only `POKEPILOT_MCP_TOKEN` gets a 302 to the Access login page there, not a 401. `pokepilot_get_run_artifact_content` exists because `/mcp` calls `pokewall`/`pokereplay` server-to-server (the private overlay addresses) the same way every other MCP tool does, so it never crosses the public Access-gated edge. It picks the same backend the browser's identical route would (`pokereplay` when configured, so a finish artifact durabilized to S3 minutes after the run ended still resolves; `pokewall` directly otherwise), bounded by `mcpMaxResponseBytes` (2 MiB) either way. Use it instead of trying to `curl` the content route directly from outside the Access session.

This static bearer is intentionally a small first deployment surface, not a full multi-user identity system. If PokePilot becomes shared by multiple untrusted users, replace it with the MCP authorization/OAuth flow and per-user scopes before broadening the tool set.
