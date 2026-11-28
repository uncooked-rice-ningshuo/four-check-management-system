// Run against dist/ after building the selected mode.
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { Script } from 'node:vm'
import { JSDOM } from 'jsdom'

const mode = process.argv[2]
assert.ok(['production', 'webvpn'].includes(mode), 'Expected production or webvpn')
const dist = new URL('../dist/', import.meta.url)
const html = await readFile(new URL('index.html', dist), 'utf8')
const { document } = new JSDOM(html).window
const entries = document.querySelectorAll('script[src]')
assert.equal(entries.length, 1)
const entry = entries[0]
const asset = path => {
  assert.match(path, /^\/assets\//)
  return readFile(new URL(path.slice(1), dist), 'utf8')
}
const source = await asset(entry.getAttribute('src'))
const style = document.querySelector('link[rel="stylesheet"]')
assert.ok(style && (await asset(style.getAttribute('href'))).length > 0)
if (mode === 'webvpn') {
  assert.equal(entry.type, '')
  assert.ok(entry.defer)
  new Script(source)
} else {
  assert.equal(entry.type, 'module')
}
