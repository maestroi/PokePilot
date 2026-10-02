# syntax=docker/dockerfile:1
#
# Headless farm fixer. This is not the farm runtime image: it carries a
# compiler, git, gh, OpenCode (local qwen tier) and the Cursor CLI (auto and
# pinned-model tiers) so it can reproduce a failure and open a PR. The ROM
# is never copied in; Swarm bind-mounts it read-only.

FROM golang:1.26-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
		ca-certificates curl git make python3 tini \
	&& curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
		-o /usr/share/keyrings/githubcli-archive-keyring.gpg \
	&& echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
		>/etc/apt/sources.list.d/github-cli.list \
	&& apt-get update && apt-get install -y --no-install-recommends gh \
	&& rm -rf /var/lib/apt/lists/* \
	&& curl -fsSL https://cursor.com/install | bash

# OpenCode 2.x (what the qwen tier is tuned on) is not published; make
# fixer-image passes the operator's binary as the "opencode" build context.
COPY --from=opencode opencode /root/.opencode/bin/opencode

ENV PATH="/root/.local/bin:/root/.cursor/bin:/root/.opencode/bin:/usr/local/go/bin:${PATH}"
RUN command -v agent && command -v opencode && command -v gh && command -v go && command -v flock

COPY deploy/qwagent-triage.sh deploy/qwagent-triage.prompt.md deploy/fixer-loop.sh /usr/local/share/pokepilot/
RUN chmod +x /usr/local/share/pokepilot/qwagent-triage.sh /usr/local/share/pokepilot/fixer-loop.sh

# tini -g forwards stop to the triage process group so the EXIT trap can
# release a GitHub issue claim before Swarm kills the task.
ENTRYPOINT ["/usr/bin/tini", "-g", "--", "/usr/local/share/pokepilot/fixer-loop.sh"]
