const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const {runTests, runVSCodeCommand} = require('@vscode/test-electron')

async function main() {
  const vsix = path.resolve(process.argv[2])
  const version = '1.90.0'
  const extensionVersion = JSON.parse(fs.readFileSync(path.resolve(__dirname, '../../package.json'), 'utf8')).version
  const installed = await runVSCodeCommand(['--install-extension', vsix, '--force'], {version})
  assert.match(`${installed.stdout}\n${installed.stderr}`, /successfully installed|was successfully installed/i)
  const listed = await runVSCodeCommand(['--list-extensions', '--show-versions'], {version})
  assert.ok(listed.stdout.includes(`oberth.oberth@${extensionVersion}`), 'installed VSIX version is not listed by VS Code')

  await runTests({
    version,
    // Load only the tiny test harness as a development extension. Oberth must
    // be discovered from the VSIX installed above, otherwise this smoke test
    // would accidentally activate the source tree.
    extensionDevelopmentPath: path.resolve(__dirname, 'harness'),
    extensionTestsPath: path.resolve(__dirname, 'suite', 'index.js'),
    launchArgs: ['--disable-workspace-trust'],
  })
}

main().catch(error => {
  console.error(error)
  process.exit(1)
})
