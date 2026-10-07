import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { resolve } from 'node:path'
import { test } from 'node:test'
import { runInNewContext } from 'node:vm'

const source = await readFile(resolve(import.meta.dirname, '../../further-ui/assets/home.js'), 'utf8')

function harness(observing = true) {
  const events = new Map(), classes = new Set(), calls = []
  let callback, disconnected = 0, unobserved = 0
  const surface = {
    classList: { add: name => classes.add(name) },
    animate: (frames, options) => {
      let complete
      const effect = { frames, options, cancelled: false, finished: new Promise(resolve => { complete = resolve }), cancel() { this.cancelled = true }, complete: () => complete() }
      calls.push(effect)
      return effect
    },
  }
  const anchor = { classList: { add: name => classes.add(name) }, querySelector: () => surface }
  const window = { addEventListener(name, handler) { if (!events.has(name)) events.set(name, []); events.get(name).push(handler) } }
  const document = { documentElement: { dataset: {} }, querySelector: () => null, querySelectorAll: selector => selector === '[data-home-scroll]' ? [anchor] : [] }
  const IntersectionObserver = class {
    constructor(handler) { callback = handler }
    observe() {}
    unobserve(target) { assert.equal(target, anchor); unobserved++ }
    disconnect() { disconnected++ }
  }
  if (observing) window.IntersectionObserver = IntersectionObserver
  runInNewContext(source, { window, document, location: { origin: 'https://example.test' }, IntersectionObserver })
  return { calls, classes, document, events, intersect: visible => callback([{ target: anchor, isIntersecting: visible }]), hide: () => events.get('pagehide').forEach(handler => handler()), disconnected: () => disconnected, unobserved: () => unobserved }
}

test('reveals a section once without replaying it when scroll visibility changes', () => {
  const motion = harness()
  motion.intersect(false)
  assert.equal(motion.calls.length, 0)
  motion.intersect(true)
  motion.intersect(false)
  motion.intersect(true)
  assert.equal(motion.calls.length, 1)
  assert.equal(motion.classes.has('is-visible'), true)
  assert.equal(motion.events.has('scroll'), false)
  assert(motion.calls[0].frames.every(frame => !/scale|blur/.test(frame.transform)))
})

test('cancels pending effects and disconnects the observer when the page leaves', () => {
  const motion = harness()
  motion.intersect(true)
  motion.hide()
  assert.equal(motion.calls[0].cancelled, true)
  assert.equal(motion.disconnected(), 1)
  assert.equal(motion.classes.has('is-visible'), true)
})

test('releases finished effects so leaving does not cancel stale animations', async () => {
  const motion = harness()
  motion.intersect(true)
  motion.calls[0].complete()
  await Promise.resolve()
  motion.hide()
  assert.equal(motion.calls[0].cancelled, false)
})

test('leaves content visible when intersection observation is unavailable', () => {
  const motion = harness(false)
  assert.equal(motion.classes.has('is-visible'), true)
  assert.equal(motion.calls.length, 0)
  assert.equal(motion.document.documentElement.dataset.homeMotion, 'ready')
})
