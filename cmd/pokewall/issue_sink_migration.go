package main

import "strings"

// reconcileIssueSink drops persisted remote identities that belong to a
// different issue UI. Failure history and durable run evidence stay intact;
// only the stale remote binding is removed so the next occurrence can be
// reported to the newly configured sink instead of being quarantined forever
// against an unreachable issue from the previous backend.
func (w *Wall) reconcileIssueSink(uiBase string) int {
	base := strings.TrimRight(strings.TrimSpace(uiBase), "/")
	if base == "" {
		return 0
	}
	prefix := base + "/issues/"
	removed := 0
	w.mu.Lock()
	for key, link := range w.issueLinks {
		if link.IssueID == "" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(link.IssueURL), prefix) {
			continue
		}
		delete(w.issueLinks, key)
		for externalID, entry := range w.outbox {
			if entry.Key != key {
				continue
			}
			if entry.Status == outboxComplete || entry.Status == outboxQuarantined || entry.Status == outboxError {
				delete(w.outbox, externalID)
			}
		}
		removed++
	}
	w.mu.Unlock()
	if removed > 0 {
		w.saveState()
	}
	return removed
}
