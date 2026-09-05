package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Build provenance is injected by the farm image build. The title is base64
// encoded before it reaches the linker so arbitrary PR punctuation cannot turn
// into shell syntax in the Docker build.
var (
	buildPR       = "0"
	buildTitleB64 = ""
	buildRepo     = "maestroi/PokePilot"
)

type buildProvenance struct {
	Version   string `json:"version"`
	PRNumber  string `json:"pr_number,omitempty"`
	Title     string `json:"title,omitempty"`
	PRURL     string `json:"pr_url,omitempty"`
	CommitURL string `json:"commit_url,omitempty"`
}

func currentBuildProvenance() buildProvenance {
	p := buildProvenance{Version: strings.TrimSpace(version)}
	if raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(buildTitleB64)); err == nil {
		p.Title = strings.TrimSpace(string(raw))
	}

	if n, err := strconv.Atoi(strings.TrimSpace(buildPR)); err == nil && n > 0 {
		p.PRNumber = strconv.Itoa(n)
	}

	repo := strings.Trim(strings.TrimSpace(buildRepo), "/")
	if !validRepositorySlug(repo) {
		return p
	}
	base := "https://github.com/" + repo
	if p.PRNumber != "" {
		p.PRURL = base + "/pull/" + p.PRNumber
	}
	if validGitSHA(p.Version) {
		p.CommitURL = base + "/commit/" + p.Version
	}
	return p
}

func validRepositorySlug(repo string) bool {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	for _, part := range parts {
		for _, r := range part {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("-_.", r) {
				continue
			}
			return false
		}
	}
	return true
}

func validGitSHA(sha string) bool {
	if len(sha) < 7 || len(sha) > 64 {
		return false
	}
	for _, r := range sha {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// injectBuildProvenance upgrades the existing tiny version label instead of
// adding another permanent header control. ui.js may continue rewriting the
// element's text every poll; the human-friendly label is a CSS pseudo-element
// backed by data-build-label, so the two concerns do not fight each other.
func injectBuildProvenance(page []byte, p buildProvenance) []byte {
	if !bytes.Contains(page, []byte(`id="versions"`)) || !bytes.Contains(page, []byte("</body>")) {
		return page
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return page
	}

	snippet := fmt.Sprintf(`<style>
#versions[data-build-ready="1"]{cursor:pointer;position:relative;display:inline-flex;align-items:center;min-height:26px;padding:2px 6px;border-radius:6px;font-size:0;transition:background .12s ease,color .12s ease}
#versions[data-build-ready="1"]::before{content:attr(data-build-label);font:11px/1.4 var(--mono);color:var(--muted)}
#versions[data-build-ready="1"]:hover,#versions[data-build-ready="1"]:focus-visible{background:var(--surface-2);color:var(--ink)}
.build-popover{position:absolute;z-index:80;top:calc(100%% + 8px);right:0;width:min(360px,calc(100vw - 32px));padding:11px 12px;border:1px solid var(--line);border-radius:9px;background:var(--surface);box-shadow:var(--shadow);color:var(--ink);font-size:13px;line-height:1.4}
.build-popover[hidden]{display:none!important}.build-popover .build-k{color:var(--muted);font-size:10px;font-weight:800;letter-spacing:.06em;text-transform:uppercase}.build-popover .build-title{margin-top:3px;font-weight:750;overflow-wrap:anywhere}.build-popover .build-links{display:flex;gap:10px;flex-wrap:wrap;margin-top:8px}.build-popover a{color:var(--blue);font:12px/1.4 var(--mono);text-decoration:none}.build-popover a:hover{text-decoration:underline}
</style>
<script>
(() => {
  const meta = %s;
  const el = document.getElementById("versions");
  if (!el || el.dataset.buildReady === "1") return;
  const short = (v) => String(v || "").slice(0, 7);
  const pr = String(meta.pr_number || "");
  const sha = short(meta.version) || "dev";
  el.dataset.buildLabel = pr ? `live #${pr} · ${sha}` : `live · ${sha}`;
  el.dataset.buildReady = "1";
  el.setAttribute("role", "button");
  el.setAttribute("tabindex", "0");
  el.setAttribute("aria-expanded", "false");
  el.setAttribute("aria-controls", "build-popover");
  el.setAttribute("title", "Show live deployment details");

  const host = el.closest(".counts") || el.parentElement;
  if (!host) return;
  host.style.position = "relative";
  const panel = document.createElement("div");
  panel.id = "build-popover";
  panel.className = "build-popover";
  panel.hidden = true;

  const key = document.createElement("div");
  key.className = "build-k";
  key.textContent = "Live deployment";
  panel.appendChild(key);

  const title = document.createElement("div");
  title.className = "build-title";
  title.textContent = meta.title || (pr ? `PR #${pr}` : "Direct/manual build");
  panel.appendChild(title);

  const links = document.createElement("div");
  links.className = "build-links";
  const addLink = (label, href) => {
    if (!href) return;
    const a = document.createElement("a");
    a.href = href;
    a.target = "_blank";
    a.rel = "noopener";
    a.textContent = label;
    links.appendChild(a);
  };
  addLink(pr ? `PR #${pr}` : "", meta.pr_url);
  addLink(`commit ${sha}`, meta.commit_url);
  panel.appendChild(links);
  host.appendChild(panel);

  const setOpen = (open) => {
    panel.hidden = !open;
    el.setAttribute("aria-expanded", String(open));
  };
  el.addEventListener("click", (event) => {
    event.stopPropagation();
    setOpen(panel.hidden);
  });
  el.addEventListener("keydown", (event) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    setOpen(panel.hidden);
  });
  document.addEventListener("click", (event) => {
    if (!panel.hidden && !panel.contains(event.target) && event.target !== el) setOpen(false);
  });
})();
</script>`, payload)

	return bytes.Replace(page, []byte("</body>"), []byte(snippet+"\n</body>"), 1)
}

func init() {
	indexHTML = injectBuildProvenance(indexHTML, currentBuildProvenance())
}
