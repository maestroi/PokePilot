import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  badgeID,
  badgeName,
  badgeSpriteUrl,
  milestoneBadgeName,
  milestoneItemName,
  milestoneVisualKind
} from '../src/shared/pokemonProgressAssets.ts'

test('maps the eight Kanto badges to PokéAPI badge artwork', () => {
  assert.equal(badgeName('Boulder'), 'Boulder Badge')
  assert.equal(badgeID('Cascade Badge'), 2)
  assert.equal(badgeID('Thunder'), 3)
  assert.equal(badgeID('Earth Badge'), 8)
  assert.match(badgeSpriteUrl('Rainbow Badge') || '', /sprites\/badges\/4\.png$/)
})

test('recognizes badge milestones', () => {
  assert.equal(milestoneBadgeName('earned the Volcano Badge'), 'Volcano Badge')
  assert.equal(milestoneVisualKind('Rainbow Badge'), 'badge')
})

test('recognizes important item and HM milestones', () => {
  assert.equal(milestoneItemName('received the S.S. Ticket'), 'S.S. Ticket')
  assert.equal(milestoneItemName('obtained Dome Fossil'), 'Dome Fossil')
  assert.equal(milestoneItemName('learned Cut'), 'HM01')
  assert.equal(milestoneItemName('HM5 secured'), 'HM05')
  assert.equal(milestoneVisualKind('Silph Scope acquired'), 'item')
})

test('leaves ordinary story milestones as story visuals', () => {
  assert.equal(milestoneBadgeName('helped Bill'), null)
  assert.equal(milestoneItemName('helped Bill'), null)
  assert.equal(milestoneVisualKind('helped Bill'), 'story')
})
