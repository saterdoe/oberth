const assert = require('node:assert/strict')
const vscode = require('vscode')

async function run() {
  const extension = vscode.extensions.getExtension('oberth.oberth')
  assert.ok(extension, 'extension metadata is discoverable')
  await extension.activate()
  const commands = await vscode.commands.getCommands(true)
  for (const command of ['oberth.run', 'oberth.status', 'oberth.review', 'oberth.openControlRoom']) {
    assert.ok(commands.includes(command), `${command} was not registered during activation`)
  }
}

module.exports = {run}
