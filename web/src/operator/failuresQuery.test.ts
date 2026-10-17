import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { TriageGroup } from '../shared/api/types'
import { resolvedBy } from './failuresQuery.ts'

test('resolvedBy credits the latest solver attempt that opened a PR', () => {
  const group = {
    key: 'k',
    issue: {
      status: 'closed',
      resolution: 'fixed',
      solver_attempts: [
        { id: '1', backend: 'opencode', model: 'qwen3.8', state: 'pr_opened', pr_number: 7 },
        { id: '2', backend: 'claude', state: 'agent_failed' }
      ]
    }
  } as unknown as TriageGroup
  assert.equal(resolvedBy(group), 'qwen3.8')
  assert.equal(resolvedBy({ ...group, issue: { ...group.issue, status: 'open', resolution: '' } } as TriageGroup), '')
  assert.equal(resolvedBy({ ...group, issue: { status: 'closed', resolution: 'fixed' } } as TriageGroup), '')
})
