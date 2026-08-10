const assert = require('node:assert/strict')
const test = require('node:test')
const path = require('node:path')
const {createCliAdapter, resolveCli} = require('../cli')

test('resolveCli prefers configured path', () => {
  assert.equal(resolveCli({configuredPath: '/custom/oberth'}), '/custom/oberth')
})

test('resolveCli detects the standard Windows installation', () => {
  const expected = path.join('C:\\Users\\test\\AppData\\Local', 'Programs', 'oberth', 'oberth.exe')
  assert.equal(resolveCli({env: {LOCALAPPDATA: 'C:\\Users\\test\\AppData\\Local'}, existsSync: candidate => candidate === expected}), expected)
})

test('adapter invokes the CLI with an explicit cwd', async () => {
  let invocation
  const adapter = createCliAdapter({configuredPath: 'oberth-test', execute: (file, args, options, callback) => {
    invocation = {file, args, options}
    callback(null, ' done \n', '')
  }})
  assert.equal(await adapter.run(['status'], '/repo'), 'done')
  assert.deepEqual(invocation, {file: 'oberth-test', args: ['status'], options: {cwd: '/repo', windowsHide: true}})
})

test('adapter explains how to recover when the CLI is missing', async () => {
  const adapter = createCliAdapter({execute: (_file, _args, _options, callback) => callback(Object.assign(new Error('missing'), {code: 'ENOENT'}), '', '')})
  await assert.rejects(adapter.run(['status'], '/repo'), /Configure oberth\.cliPath or install the CLI/)
})

test('adapter reports CLI stderr', async () => {
  const adapter = createCliAdapter({execute: (_file, _args, _options, callback) => callback(new Error('exit 1'), '', 'provider unavailable\n')})
  await assert.rejects(adapter.run(['status'], '/repo'), /provider unavailable/)
})
