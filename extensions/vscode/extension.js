const vscode = require('vscode')
const {createCliAdapter} = require('./cli')

async function selectWorkspaceRoot(api) {
  const folders = api.workspace.workspaceFolders || []
  if (!folders.length) {
    await api.window.showErrorMessage('Open a repository workspace before running Oberth.')
    return undefined
  }
  if (folders.length === 1) return folders[0].uri.fsPath
  const selected = await api.window.showQuickPick(
    folders.map(folder => ({label: folder.name, description: folder.uri.fsPath, folder})),
    {placeHolder: 'Select the repository for this Oberth command'},
  )
  return selected?.folder.uri.fsPath
}

function createCommands(api, cliAdapter) {
  const reportErrors = handler => async () => {
    try {
      return await handler()
    } catch (error) {
      await api.window.showErrorMessage(error instanceof Error ? error.message : String(error))
      return undefined
    }
  }

  return {
    'oberth.run': reportErrors(async () => {
      const cwd = await selectWorkspaceRoot(api)
      if (!cwd) return
      const intention = await api.window.showInputBox({prompt: '¿Qué querés lograr en este repositorio?'})
      if (!intention) return
      await api.window.withProgress({location: api.ProgressLocation.Notification, title: 'oberth'}, async () => {
        const output = await cliAdapter.run(['run', intention], cwd)
        await api.window.showInformationMessage(output || 'Tarea iniciada')
      })
    }),
    'oberth.status': reportErrors(async () => {
      const cwd = await selectWorkspaceRoot(api)
      if (!cwd) return
      await api.window.showInformationMessage(await cliAdapter.run(['status'], cwd))
    }),
    'oberth.review': reportErrors(async () => {
      const cwd = await selectWorkspaceRoot(api)
      if (!cwd) return
      const raw = await cliAdapter.run(['diff', '--output', 'json'], cwd)
      let files
      try {
        files = JSON.parse(raw)
      } catch {
        throw new Error('Oberth returned invalid JSON while loading the latest diff.')
      }
      if (!Array.isArray(files)) throw new Error('Oberth returned an invalid diff response.')
      const content = files.map(file => `diff --git a/${file.path} b/${file.path}\n${file.content || ''}`).join('\n\n')
      const document = await api.workspace.openTextDocument({content: content || 'No file changes were recorded.', language: 'diff'})
      await api.window.showTextDocument(document, {preview: false})
    }),
    'oberth.openControlRoom': reportErrors(async () => {
      const url = api.workspace.getConfiguration('oberth').get('uiUrl', 'http://127.0.0.1:5173')
      await api.env.openExternal(api.Uri.parse(url))
    }),
  }
}

async function activate(context) {
  const configuredPath = vscode.workspace.getConfiguration('oberth').get('cliPath', '')
  const commands = createCommands(vscode, createCliAdapter({configuredPath}))
  for (const [name, handler] of Object.entries(commands)) {
    context.subscriptions.push(vscode.commands.registerCommand(name, handler))
  }
}

module.exports = {activate, createCommands, deactivate() {}, selectWorkspaceRoot}
