import assert from 'node:assert/strict'
import crypto from 'node:crypto'
import fs from 'node:fs'
import net from 'node:net'
import os from 'node:os'
import path from 'node:path'
import {spawn, spawnSync} from 'node:child_process'

const packageDir = path.resolve(process.argv[2] || 'package')
const platform = process.argv[3] || process.platform
const architecture = process.argv[4] || process.arch
const suffix = platform === 'windows' ? '.exe' : ''
const cliName = `oberth-${platform}-${architecture}${suffix}`
const serverName = `oberth-server-${platform}-${architecture}${suffix}`
const cli = path.join(packageDir, cliName)
const server = path.join(packageDir, serverName)
const logDir = path.resolve('smoke-logs')
const logFile = path.join(logDir, `${platform}.log`)
fs.mkdirSync(logDir, {recursive: true})
let log = ''
const record = value => { log += `${value}\n`; fs.writeFileSync(logFile, log) }

function requireFile(name) {
  const candidate = path.join(packageDir, name)
  assert.ok(fs.statSync(candidate).isFile(), `package is missing ${name}`)
  return candidate
}

for (const name of [cliName, serverName, 'LICENSE', 'NOTICE', 'THIRD_PARTY_NOTICES.md', 'SHA256SUMS', 'sbom.spdx.json']) {
  requireFile(name)
}

if (platform !== 'windows') {
  fs.chmodSync(cli, 0o755)
  fs.chmodSync(server, 0o755)
}

const checksumLines = fs.readFileSync(path.join(packageDir, 'SHA256SUMS'), 'utf8').trim().split(/\r?\n/)
for (const line of checksumLines) {
  const match = line.match(/^([a-f0-9]{64})\s+\*?(.+)$/i)
  assert.ok(match, `invalid checksum line: ${line}`)
  const data = fs.readFileSync(path.join(packageDir, match[2]))
  assert.equal(crypto.createHash('sha256').update(data).digest('hex'), match[1].toLowerCase(), `checksum mismatch for ${match[2]}`)
}
record(`verified ${checksumLines.length} checksums and required package contents`)

const expectedVersion = JSON.parse(fs.readFileSync(new URL('../package.json', import.meta.url), 'utf8')).version
const version = spawnSync(cli, ['version'], {encoding: 'utf8'})
assert.equal(version.status, 0, version.stderr || 'oberth version failed')
assert.match(version.stdout, new RegExp(expectedVersion.replaceAll('.', '\\.')), 'binary version does not match package version')
record(version.stdout.trim())

const smokeRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'oberth-artifact-smoke-'))
const repository = path.join(smokeRoot, 'repository')
fs.mkdirSync(repository)
assert.equal(spawnSync('git', ['init'], {cwd: repository, encoding: 'utf8'}).status, 0, 'git init failed')
const initialized = spawnSync(cli, ['init'], {cwd: repository, encoding: 'utf8'})
assert.equal(initialized.status, 0, initialized.stderr || 'oberth init failed')
assert.ok(fs.existsSync(path.join(repository, '.oberth.yaml')), 'oberth init did not create .oberth.yaml')
assert.ok(fs.existsSync(path.join(repository, '.agent-vault', 'memory-index.md')), 'oberth init did not create the memory index')
record('oberth init created a usable repository configuration')

const port = await new Promise((resolve, reject) => {
  const listener = net.createServer()
  listener.once('error', reject)
  listener.listen(0, '127.0.0.1', () => {
    const selected = listener.address().port
    listener.close(error => error ? reject(error) : resolve(selected))
  })
})
const serverRoot = path.join(smokeRoot, 'server')
fs.mkdirSync(serverRoot)
const child = spawn(server, [], {
  cwd: serverRoot,
  env: {...process.env, OBERTH_SERVER_HOST: '127.0.0.1', OBERTH_SERVER_PORT: String(port), OBERTH_AUTH_TOKEN: 'artifact-smoke-token'},
  stdio: ['ignore', 'pipe', 'pipe'],
})
child.stdout.on('data', chunk => record(chunk.toString().trimEnd()))
child.stderr.on('data', chunk => record(chunk.toString().trimEnd()))

const deadline = Date.now() + 120_000
let healthy = false
try {
  while (Date.now() < deadline) {
    if (child.exitCode !== null) throw new Error(`oberth-server exited with code ${child.exitCode}`)
    try {
      const response = await fetch(`http://127.0.0.1:${port}/api/v1/health`)
      if (response.ok) {
        healthy = true
        record(`health check passed on port ${port}`)
        break
      }
    } catch {}
    await new Promise(resolve => setTimeout(resolve, 500))
  }
  assert.ok(healthy, 'oberth-server did not become healthy within 120 seconds')
} finally {
  child.kill('SIGTERM')
  await Promise.race([
    new Promise(resolve => child.once('exit', resolve)),
    new Promise(resolve => setTimeout(resolve, 10_000)),
  ])
  if (child.exitCode === null) child.kill('SIGKILL')
}

console.log(`Packaged ${platform} artifact smoke test passed.`)
