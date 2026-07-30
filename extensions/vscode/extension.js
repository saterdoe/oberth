const vscode = require('vscode')
const {execFile} = require('child_process')
const fs = require('fs')
const path = require('path')

function cli() {
  const configured = vscode.workspace.getConfiguration('oberth').get('cliPath', '')
  if (configured) return configured
  const installed = process.env.LOCALAPPDATA && path.join(process.env.LOCALAPPDATA, 'Programs', 'oberth', 'oberth.exe')
  return installed && fs.existsSync(installed) ? installed : 'oberth'
}

function run(args, cwd) {
  return new Promise((resolve, reject) => {
    execFile(cli(), args, {cwd, windowsHide: true}, (error, stdout, stderr) => {
      if (error) reject(new Error((stderr || error.message).trim()))
      else resolve(stdout.trim())
    })
  })
}

function root() {
  return vscode.workspace.workspaceFolders?.[0]?.uri.fsPath
}

async function activate(context) {
  context.subscriptions.push(vscode.commands.registerCommand('oberth.run', async () => {
    const intention = await vscode.window.showInputBox({prompt: '¿Qué querés lograr en este repositorio?'})
    if (!intention || !root()) return
    await vscode.window.withProgress({location: vscode.ProgressLocation.Notification, title: 'oberth'}, async () => {
      const output = await run(['run', intention], root())
      vscode.window.showInformationMessage(output || 'Tarea iniciada')
    })
  }))
  context.subscriptions.push(vscode.commands.registerCommand('oberth.status', async () => {
    if (!root()) return
    vscode.window.showInformationMessage(await run(['status'], root()))
  }))
  context.subscriptions.push(vscode.commands.registerCommand('oberth.review', async () => {
    if (!root()) return
    const raw = await run(['diff', '--output', 'json'], root())
    const files = JSON.parse(raw)
    const content = files.map(file => `diff --git a/${file.path} b/${file.path}\n${file.content || ''}`).join('\n\n')
    const document = await vscode.workspace.openTextDocument({content: content || 'No file changes were recorded.', language: 'diff'})
    await vscode.window.showTextDocument(document, {preview: false})
  }))
  context.subscriptions.push(vscode.commands.registerCommand('oberth.openControlRoom', async () => {
    const url = vscode.workspace.getConfiguration('oberth').get('uiUrl', 'http://127.0.0.1:5173')
    await vscode.env.openExternal(vscode.Uri.parse(url))
  }))
}

module.exports = {activate, deactivate() {}}
