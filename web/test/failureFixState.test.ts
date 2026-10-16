import assert from 'node:assert/strict'
import { test } from 'node:test'
import { failureFixState } from '../src/operator/failureFixState.ts'

const failed = (message: string, attempt = 1, round = 5) => ({ source: 'skill', kind: 'failed', message, attempt, round })

test('failure rows report the fixer state of their triage group', () => {
  const attempts = [{
    attempt: 1,
    problems: [
      { triage_key: 'k-queued', objective: 'go to x', first_round: 1, last_round: 3, queued: true },
      { triage_key: 'k-pr', objective: 'go to x', first_round: 4, last_round: 9, solver: { backend: 'cursor', pr_number: 12, pr_url: 'u' } },
      { triage_key: 'k-recovered', objective: 'beat gym', recovered: true }
    ]
  }]
  assert.equal(failureFixState(failed('go to x', 1, 2), attempts)?.label, 'Queued for fixer')
  const pr = failureFixState(failed('go to x', 1, 5), attempts)
  assert.equal(pr?.label, 'Fixer PR #12')
  assert.equal(pr?.key, 'k-pr')
  assert.equal(failureFixState(failed('beat gym'), attempts)?.label, 'Not queued')
  assert.equal(failureFixState(failed('train lead'), attempts)?.label, 'Not filed')
  assert.equal(failureFixState(failed('go to x', 2), attempts)?.label, 'Pending')
})

test('agent-written links must be web URLs', () => {
  const attempts = [{ attempt: 1, problems: [{
    triage_key: 'k', objective: 'go to x', issue_url: 'javascript:alert(1)',
    solver: { pr_number: 3, pr_url: 'https://github.com/o/r/pull/3' }
  }] }]
  const state = failureFixState(failed('go to x'), attempts)
  assert.equal(state?.issueURL, undefined)
  assert.equal(state?.prURL, 'https://github.com/o/r/pull/3')
})

test('non-objective events and an unavailable endpoint show no fixer state', () => {
  assert.equal(failureFixState({ source: 'system', kind: 'failure', message: 'x' }, []), null)
  assert.equal(failureFixState(failed('go to x'), null), null)
})
