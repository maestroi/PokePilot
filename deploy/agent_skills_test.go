package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// This repository authors its agent workflows as Claude Code skills under
// .claude/skills, but the skill roots a harness scans are <project>/.dsh/skills
// and <project>/.agents/skills (DSH), with .cursor/rules as Cursor's pointer.
// .agents/skills is therefore a symlink to .claude/skills so every harness sees
// one copy. If that link or a skill's frontmatter breaks, discovery fails
// quietly: the model catalog simply loses every repository skill with no error
// an agent can see.
const (
	claudeSkillsDir = "../.claude/skills"
	agentsSkillsDir = "../.agents/skills"
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func TestAgentSkillRootIsSymlinkToClaudeSkills(t *testing.T) {
	info, err := os.Lstat(agentsSkillsDir)
	if err != nil {
		t.Fatalf("%s must exist or harnesses cannot discover the repository skills: %v", agentsSkillsDir, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is %s, not a symlink; a copy would drift from %s", agentsSkillsDir, info.Mode(), claudeSkillsDir)
	}
	link, err := os.Readlink(agentsSkillsDir)
	if err != nil {
		t.Fatal(err)
	}
	if link != "../.claude/skills" {
		t.Fatalf("%s -> %q, want the relative ../.claude/skills so every checkout resolves it", agentsSkillsDir, link)
	}
}

func TestAgentSkillsAreDiscoverableBundles(t *testing.T) {
	want, err := skillBundleNames(claudeSkillsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) == 0 {
		t.Fatalf("no <name>/SKILL.md bundles under %s", claudeSkillsDir)
	}
	got, err := skillBundleNames(agentsSkillsDir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(want, ",") != strings.Join(got, ",") {
		t.Fatalf("%s exposes %v but %s exposes %v", claudeSkillsDir, want, agentsSkillsDir, got)
	}
	for _, name := range want {
		path := filepath.Join(claudeSkillsDir, name, "SKILL.md")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Error(err)
			continue
		}
		fields, err := skillFrontmatter(string(body))
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if fields["name"] != name {
			t.Errorf("%s: frontmatter name %q must equal its directory %q; the catalog keys on the frontmatter", path, fields["name"], name)
		}
		if !skillNamePattern.MatchString(fields["name"]) {
			t.Errorf("%s: name %q is not kebab-case, so discovery drops the skill", path, fields["name"])
		}
		if fields["description"] == "" {
			t.Errorf("%s: description is required; without it no agent knows when to load the skill", path)
		}
	}
}

func TestRunFixSkillCoversTheRunIDEntryPoint(t *testing.T) {
	path := filepath.Join(claudeSkillsDir, "pokefarm-run-fix", "SKILL.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		"name: pokefarm-run-fix",
		"pokepilot_get_run_debug",
		"circuit_key",
		"pokepilot_get_triage",
		"pokepilot_investigate_failure",
		"pokepilot_record_solver_attempt",
		"[triage:<key>]",
		"make test-short",
		"pokefarm-triage",
		"Do not cancel",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("run-fix skill missing %q", want)
		}
	}
}

// skillBundleNames lists the direct child directories of root that hold a
// SKILL.md, which is the only shape discovery recognizes.
func skillBundleNames(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, entry.Name(), "SKILL.md")); err != nil {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

// skillFrontmatter reads the flat `key: value` pairs of a SKILL.md frontmatter
// block. Only the scalar and block-scalar shapes a catalog entry needs are
// understood; a nested block is kept as an empty value rather than rejected, so
// this guard fails on a missing name or description instead of on YAML dialect.
func skillFrontmatter(body string) (map[string]string, error) {
	lines := strings.Split(body, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("missing leading --- frontmatter delimiter")
	}
	fields := map[string]string{}
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "---" {
			return fields, nil
		}
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("frontmatter line %d is not key: value: %q", i+1, line)
		}
		value = strings.TrimSpace(value)
		switch value {
		case "|", "|-", "|+", ">", ">-", ">+":
			var block []string
			for j := i + 1; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) == "" {
					block = append(block, "")
					continue
				}
				if !strings.HasPrefix(lines[j], " ") && !strings.HasPrefix(lines[j], "\t") {
					break
				}
				block = append(block, strings.TrimSpace(lines[j]))
			}
			value = strings.TrimSpace(strings.Join(block, " "))
		}
		fields[strings.TrimSpace(key)] = value
	}
	return nil, fmt.Errorf("frontmatter block is not closed")
}
