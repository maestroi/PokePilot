import assert from 'node:assert/strict'
import test from 'node:test'
import { nextDelayMs } from '../src/shared/composables/usePollingResource.ts'

test('failed polls retry on a short backoff capped at the interval', () => {
  assert.equal(nextDelayMs(30000, 0), 30000)
  assert.deepEqual([1, 2, 3, 4, 5, 6].map((n) => nextDelayMs(30000, n)), [1000, 2000, 4000, 8000, 16000, 30000])
  assert.equal(nextDelayMs(5000, 99), 5000)
})
