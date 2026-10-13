import assert from 'node:assert/strict'
import test from 'node:test'

import { fitMapTiles, MAP_CANVAS_MAX_EDGE, MAP_ZOOM_MAX } from '../src/shared/mapViewport.ts'

test('100% shows the whole map inside the box', () => {
  const fit = fitMapTiles({
    viewportWidth: 1100,
    viewportHeight: 480,
    mapWidth: 10,
    mapHeight: 9,
    zoom: 1,
    fillBox: true
  })
  assert.equal(fit.zoom, 1)
  assert.equal(fit.tile, 53)
  assert.ok(fit.tile * 10 <= 1100)
  assert.ok(fit.tile * 9 <= 480)
})

test('embedded maps stay inside the box', () => {
  const fit = fitMapTiles({
    viewportWidth: 1100,
    viewportHeight: 480,
    mapWidth: 10,
    mapHeight: 9,
    zoom: 1,
    fillBox: false
  })
  assert.equal(fit.tile, 53)
  assert.ok(fit.tile * 10 <= 1100)
  assert.ok(fit.tile * 9 <= 480)
})

test('zoom in stays inside the canvas edge cap', () => {
  const fit = fitMapTiles({
    viewportWidth: 1200,
    viewportHeight: 600,
    mapWidth: 20,
    mapHeight: 72,
    zoom: MAP_ZOOM_MAX,
    fillBox: true,
    minTile: 6
  })
  assert.ok(fit.tile * 20 <= MAP_CANVAS_MAX_EDGE)
  assert.ok(fit.tile * 72 <= MAP_CANVAS_MAX_EDGE)
  assert.ok(fit.zoom <= fit.zoomCeiling)
  assert.ok(fit.zoomCeiling <= MAP_ZOOM_MAX)
})

test('a viewport that grows with the canvas cannot run away', () => {
  let width = 800
  let height = 400
  let zoom = 1
  for (let step = 0; step < 8; step++) {
    const fit = fitMapTiles({
      viewportWidth: width,
      viewportHeight: height,
      mapWidth: 20,
      mapHeight: 72,
      zoom,
      fillBox: true,
      minTile: 6
    })
    const canvasW = fit.tile * 20
    const canvasH = fit.tile * 72
    assert.ok(canvasW <= MAP_CANVAS_MAX_EDGE, `width ${canvasW} at step ${step}`)
    assert.ok(canvasH <= MAP_CANVAS_MAX_EDGE, `height ${canvasH} at step ${step}`)
    width = canvasW
    height = canvasH
    zoom = fit.zoom + 0.25
  }
})

test('a readability floor cannot magnify the default view past the box', () => {
  const fit = fitMapTiles({
    viewportWidth: 90,
    viewportHeight: 40,
    mapWidth: 8,
    mapHeight: 6,
    zoom: 1,
    fillBox: true,
    minTile: 18,
    evenTiles: true
  })
  assert.equal(fit.zoom, 1)
  assert.equal(fit.tile % 2, 0)
  assert.ok(fit.tile * 8 <= 90)
  assert.ok(fit.tile * 6 <= 40)
})
