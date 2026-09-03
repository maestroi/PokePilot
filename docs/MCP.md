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
https://pokemon.labstack.cc/mcp
```

Every MCP request must include:

```text
Authorization: Bearer <POKEPILOT_MCP_TOKEN>
```

Treat that token as a compute-control credential: holders can start and cancel runs. Rotate it if it is exposed. An empty token removes the `/mcp` route entirely.

## Tools

| Tool | Effect |
|---|---|
| `pokepilot_start_run` | Queue one finite run. Defaults to `llm`, Squirtle, and `Earn the Boulder Badge.` |
| `pokepilot_list_runs` | List recent runs, optionally filtered by `queued`, `leased`, `running`, or `done` |
| `pokepilot_get_run` | Read one run's live/finished state, including planner state, party and LLM stats when available |
| `pokepilot_cancel_run` | Request cooperative cancellation |
| `pokepilot_get_triage` | Read grouped failures |
| `pokepilot_investigate_failure` | Trigger the existing failure-investigation handoff |

MCP intentionally does **not** expose runner leases, heartbeats, finish/checkpoint uploads, worker registration, run deletion, endless runs, arbitrary HTTP, Docker, or Swarm controls.

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
  "url": "https://pokemon.labstack.cc/mcp",
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
        "url": "https://pokemon.labstack.cc/mcp",
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
POKEPILOT_MCP_URL=https://pokemon.labstack.cc/mcp \
POKEPILOT_MCP_TOKEN='...' \
  go run ./examples/mcp-client
```

It connects with the bearer token, lists the server tools, and exits without starting a run.

## Security boundary

`pokeui` remains the only public-facing process. `/mcp` talks to `pokewall` through a fixed set of operator calls; it cannot proxy an arbitrary path. The Streamable HTTP handler is stateless, request bodies are bounded, cross-origin browser requests are protected, and bearer comparison is constant-time.

This static bearer is intentionally a small first deployment surface, not a full multi-user identity system. If PokePilot becomes shared by multiple untrusted users, replace it with the MCP authorization/OAuth flow and per-user scopes before broadening the tool set.
