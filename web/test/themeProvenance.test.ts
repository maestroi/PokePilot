import assert from 'node:assert/strict'
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

import { renderThemeOptions, validateThemeAssetReference } from '../src/shared/renderTheme.ts'

interface Provenance {
  source?: string
  license?: string
  files?: Record<string, unknown>
}

const assetRoot = new URL('../public/theme-assets/', import.meta.url)

function referencedAssets(): string[] {
  const refs: string[] = []
  for (const theme of renderThemeOptions()) {
    for (const section of Object.values(theme.assets || {})) {
      if (!section) continue
      refs.push(...Object.values(section))
    }
  }
  return refs
}

function imageFiles(root: URL, prefix = ''): string[] {
  const out: string[] = []
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    const relative = prefix ? `${prefix}/${entry.name}` : entry.name
    const url = new URL(`${entry.name}${entry.isDirectory() ? '/' : ''}`, root)
    if (entry.isDirectory()) out.push(...imageFiles(url, relative))
    else if (/\.(png|webp)$/i.test(entry.name)) out.push(relative)
  }
  return out
}

test('every bundled theme image has machine-readable provenance', () => {
  for (const pack of readdirSync(assetRoot, { withFileTypes: true }).filter((entry) => entry.isDirectory())) {
    const packRoot = new URL(`${pack.name}/`, assetRoot)
    const provenanceURL = new URL('provenance.json', packRoot)
    assert.equal(existsSync(provenanceURL), true, `${pack.name} is missing provenance.json`)
    const provenance = JSON.parse(readFileSync(provenanceURL, 'utf8')) as Provenance
    assert.ok(provenance.source?.trim(), `${pack.name} provenance is missing source`)
    assert.ok(provenance.license?.trim(), `${pack.name} provenance is missing license`)
    assert.ok(provenance.files && typeof provenance.files === 'object', `${pack.name} provenance is missing files`)

    for (const file of imageFiles(packRoot)) {
      assert.ok(file in (provenance.files || {}), `${pack.name}/${file} is missing provenance`)
      assert.ok(statSync(fileURLToPath(new URL(file, packRoot))).size > 0, `${pack.name}/${file} is empty`)
    }
  }
})

test('every bundled theme reference stays local and resolves to a provenanced image', () => {
  for (const reference of referencedAssets()) {
    assert.equal(validateThemeAssetReference(reference), null, reference)
    const rawPath = decodeURIComponent(reference.split(/[?#]/, 1)[0])
    const match = rawPath.match(/^\/theme-assets\/([^/]+)\/(.+)$/)
    assert.ok(match, `unexpected asset reference ${reference}`)
    const [, pack, relative] = match
    const fileURL = new URL(`${pack}/${relative}`, assetRoot)
    assert.equal(existsSync(fileURL), true, `missing bundled asset ${rawPath}`)
    const provenance = JSON.parse(
      readFileSync(new URL(`${pack}/provenance.json`, assetRoot), 'utf8')
    ) as Provenance
    assert.ok(relative in (provenance.files || {}), `${rawPath} is not listed in provenance.json`)
  }
})
