import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('../src/operator/main.ts', import.meta.url), 'utf8')

test('operator replaces missing run-card frames with a neutral placeholder', () => {
  assert.match(source, /source\.startsWith\('\/frame\?run='\)/)
  assert.match(source, /host\.dataset\.runFrameMissing = 'true'/)
  assert.match(source, /Last recorded frame unavailable\./)
  assert.match(source, /content: "NO FRAME"/)
  assert.match(source, /document\.addEventListener\('error',[\s\S]*true\)/)
  assert.match(source, /document\.addEventListener\('load',[\s\S]*true\)/)
})
