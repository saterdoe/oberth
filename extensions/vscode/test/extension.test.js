const assert = require('node:assert/strict')
const test = require('node:test')
const Module = require('node:module')

const vscodeStub = {}
const originalLoad = Module._load
Module._load = function(request, parent, isMain) {
  if (request === 'vscode') return vscodeStub
  return originalLoad.call(this, request, parent, isMain)
}
const {createCommands, selectWorkspaceRoot} = require('../extension')
Module._load = originalLoad

function fixture(folderPaths = ['/repo']) {
  const calls = {cli: [], errors: [], info: [], documents: [], external: [], quickPick: 0}
  const folders = folderPaths.map((fsPath, index) => ({name: `repo-${index + 1}`, uri: {fsPath}}))
  const api = {
    ProgressLocation: {Notification: 15},
    Uri: {parse: value => ({value})},
    env: {openExternal: async uri => { calls.external.push(uri.value) }},
    workspace: {
      workspaceFolders: folders,
      getConfiguration: () => ({get: (_key, fallback) => fallback}),
      openTextDocument: async options => { calls.documents.push(options); return options },
    },
    window: {
      showErrorMessage: async message => { calls.errors.push(message) },
      showInformationMessage: async message => { calls.info.push(message) },
      showInputBox: async () => 'fix the bug',
      showQuickPick: async items => { calls.quickPick += 1; return items[1] },
      showTextDocument: async () => {},
      withProgress: async (_options, callback) => callback(),
    },
  }
  const cli = {run: async (args, cwd) => { calls.cli.push({args, cwd}); return args[0] === 'diff' ? '[]' : 'ok' }}
  return {api, calls, cli}
}

test('single-root workspace is selected without prompting', async () => {
  const {api, calls} = fixture(['/one'])
  assert.equal(await selectWorkspaceRoot(api), '/one')
  assert.equal(calls.quickPick, 0)
})

test('multi-root workspace requires an explicit repository selection', async () => {
  const {api, calls} = fixture(['/one', '/two'])
  assert.equal(await selectWorkspaceRoot(api), '/two')
  assert.equal(calls.quickPick, 1)
})

test('run command sends the intention to the selected repository', async () => {
  const {api, calls, cli} = fixture(['/repo'])
  await createCommands(api, cli)['oberth.run']()
  assert.deepEqual(calls.cli, [{args: ['run', 'fix the bug'], cwd: '/repo'}])
  assert.deepEqual(calls.info, ['ok'])
})

test('status command reports CLI output', async () => {
  const {api, calls, cli} = fixture(['/repo'])
  await createCommands(api, cli)['oberth.status']()
  assert.deepEqual(calls.cli, [{args: ['status'], cwd: '/repo'}])
  assert.deepEqual(calls.info, ['ok'])
})

test('review command opens a diff document', async () => {
  const {api, calls} = fixture(['/repo'])
  const cli = {run: async () => JSON.stringify([{path: 'main.go', content: '+change'}])}
  await createCommands(api, cli)['oberth.review']()
  assert.deepEqual(calls.documents, [{content: 'diff --git a/main.go b/main.go\n+change', language: 'diff'}])
})

test('review command handles invalid JSON without an unhandled rejection', async () => {
  const {api, calls} = fixture(['/repo'])
  await createCommands(api, {run: async () => '{invalid'})['oberth.review']()
  assert.deepEqual(calls.errors, ['Oberth returned invalid JSON while loading the latest diff.'])
})

test('open Control Room command uses the configured URL', async () => {
  const {api, calls, cli} = fixture()
  api.workspace.getConfiguration = () => ({get: () => 'http://127.0.0.1:9091'})
  await createCommands(api, cli)['oberth.openControlRoom']()
  assert.deepEqual(calls.external, ['http://127.0.0.1:9091'])
})

test('commands surface CLI failures through VS Code', async () => {
  const {api, calls} = fixture(['/repo'])
  await createCommands(api, {run: async () => { throw new Error('CLI unavailable') }})['oberth.status']()
  assert.deepEqual(calls.errors, ['CLI unavailable'])
})

test('repository commands explain that a workspace is required', async () => {
  const {api, calls, cli} = fixture([])
  await createCommands(api, cli)['oberth.status']()
  assert.deepEqual(calls.cli, [])
  assert.deepEqual(calls.errors, ['Open a repository workspace before running Oberth.'])
})
