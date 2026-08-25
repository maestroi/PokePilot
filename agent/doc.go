// Package agent is the intent layer above skill: a small, typed description
// of what to do, which Execute turns into calls on the skills that already
// exist. There is no planner and no loop here; those come later. The
// dependency points one way only: agent imports skill, never the reverse.
package agent
