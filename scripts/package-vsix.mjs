import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import {spawnSync} from 'node:child_process'
import {fileURLToPath} from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const extensionDir = path.join(root, 'extensions', 'vscode')
const rootPackage = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8'))
const extensionPackage = JSON.parse(fs.readFileSync(path.join(extensionDir, 'package.json'), 'utf8'))
assert.equal(extensionPackage.version, rootPackage.version, 'VS Code extension version must match the Oberth release version')

const output = process.argv[2]
  ? path.resolve(root, process.argv[2])
  : path.join(root, 'dist', `oberth-vscode-${rootPackage.version}.vsix`)
fs.mkdirSync(path.dirname(output), {recursive: true})
const extensionLicense = path.join(extensionDir, 'LICENSE')
fs.copyFileSync(path.join(root, 'LICENSE'), extensionLicense)

try {
  const vsce = path.join(extensionDir, 'node_modules', '@vscode', 'vsce', 'vsce')
  const result = spawnSync(process.execPath, [vsce, 'package', '--no-dependencies', '--out', output], {
    cwd: extensionDir,
    encoding: 'utf8',
    env: {...process.env, SOURCE_DATE_EPOCH: process.env.SOURCE_DATE_EPOCH || '0'},
  })
  if (result.status !== 0) throw new Error(result.error?.message || result.stderr || result.stdout || 'VSIX packaging failed')
  assert.ok(fs.statSync(output).size > 0, 'VSIX package is empty')
  console.log(`Created ${output}`)
} finally {
  fs.rmSync(extensionLicense, {force: true})
}
