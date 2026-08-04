import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'

const root = path.resolve(import.meta.dirname, '..')
const coverageDir = path.join(root, 'artifacts', 'coverage')
const goProfile = path.join(coverageDir, 'go.out')
const goSummary = path.join(coverageDir, 'go-summary.txt')
const goMinimum = Number(process.env.GO_COVERAGE_MIN ?? '45')
const npm = process.platform === 'win32' ? 'npm.cmd' : 'npm'

fs.mkdirSync(coverageDir, { recursive: true })

function run(command, args, options = {}) {
  console.log(`> ${command} ${args.join(' ')}`)
  const result = spawnSync(command, args, {
    cwd: root,
    encoding: 'utf8',
    stdio: options.capture ? 'pipe' : 'inherit',
    ...options,
  })
  if (result.error) throw result.error
  if (result.status !== 0) process.exit(result.status ?? 1)
  return result.stdout ?? ''
}

function runNpm(args) {
  if (process.platform === 'win32') {
    return run(process.env.ComSpec ?? 'cmd.exe', ['/d', '/s', '/c', npm, ...args])
  }
  return run(npm, args)
}

const packageOutput = run('go', ['list', './cmd/...', './internal/...', './pkg/...'], { capture: true })
const goPackages = packageOutput
  .split(/\r?\n/)
  .filter(Boolean)
  .filter(packageName => !packageName.includes('/internal/generated/'))
if (goPackages.length === 0) throw new Error('Go package discovery returned no maintained packages')

run('go', ['vet', ...goPackages])
run('go', ['test', ...goPackages, `-coverprofile=${goProfile}`])
const summary = run('go', ['tool', 'cover', `-func=${goProfile}`], { capture: true })
fs.writeFileSync(goSummary, summary)

const total = summary.match(/^total:\s+\(statements\)\s+([0-9.]+)%$/m)
if (!total) throw new Error('Go coverage output did not contain a total')
const percentage = Number(total[1])
console.log(total[0])
if (percentage < goMinimum) {
  console.error(`Go statement coverage ${percentage}% is below the ${goMinimum}% baseline.`)
  process.exit(1)
}

runNpm(['--prefix', 'ui', 'run', 'typecheck'])
runNpm(['--prefix', 'ui', 'run', 'test:coverage'])
console.log(`Quality gates passed. Go statement coverage: ${percentage}% (minimum ${goMinimum}%).`)
