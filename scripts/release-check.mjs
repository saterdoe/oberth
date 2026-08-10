import fs from 'node:fs'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'

const root = path.resolve(import.meta.dirname, '..')

export const RELEASE_STEPS = Object.freeze([
  { name: 'Install root dependencies', command: 'npm', args: ['ci'] },
  { name: 'Install UI dependencies', command: 'npm', args: ['--prefix', 'ui', 'ci'] },
  { name: 'Install VS Code extension dependencies', command: 'npm', args: ['--prefix', 'extensions/vscode', 'ci'] },
  { name: 'Go tests', command: 'go', args: ['test', './cmd/...', './internal/...', './pkg/...'] },
  { name: 'Go vet', command: 'go', args: ['vet', './cmd/...', './internal/...', './pkg/...'] },
  { name: 'Go builds', command: 'go', args: ['build', './cmd/oberth', './cmd/oberth-server'] },
  {
    name: 'Race detector',
    command: 'go',
    args: ['test', '-race', './internal/api', './internal/gateway', './internal/toolrunner', './pkg/git'],
  },
  {
    name: 'Durable E2E',
    command: 'go',
    args: ['test', '-tags', 'e2e', './internal/api', '-run', 'TestDurableRunHTTPHappyPath', '-count=1'],
  },
  { name: 'UI tests', command: 'npm', args: ['test'] },
  { name: 'UI build', command: 'npm', args: ['run', 'build:all'] },
  { name: 'Documentation contract', command: 'npm', args: ['run', 'test:docs'] },
  { name: 'Release pipeline contract', command: 'node', args: ['scripts/release_check_contract.test.mjs'] },
  { name: 'Publishing workflow contract', command: 'node', args: ['scripts/release_workflow_contract.test.mjs'] },
])

function executable(command) {
  return process.platform === 'win32' && command === 'npm' ? 'npm.cmd' : command
}

function run(command, args, options = {}) {
  const useWindowsNpm = process.platform === 'win32' && command === 'npm'
  const childCommand = useWindowsNpm ? (process.env.ComSpec || 'cmd.exe') : executable(command)
  const childArgs = useWindowsNpm
    ? ['/d', '/s', '/c', ['npm.cmd', ...args].map(value =>
        /[\s"&|<>^]/.test(value) ? `"${value.replaceAll('"', '""')}"` : value,
      ).join(' ')]
    : args
  const result = spawnSync(childCommand, childArgs, {
    cwd: root,
    encoding: 'utf8',
    stdio: options.capture ? 'pipe' : 'inherit',
  })
  if (result.error) {
    throw new Error(`${command} is required but could not be executed: ${result.error.message}`)
  }
  if (result.status !== 0) {
    if (options.capture && result.stderr) process.stderr.write(result.stderr)
    throw new Error(`${command} ${args.join(' ')} failed with exit code ${result.status}`)
  }
  return options.capture ? result.stdout.trim() : ''
}

function requirePrerequisites() {
  for (const [command, args] of [
    ['git', ['--version']],
    ['go', ['version']],
    ['node', ['--version']],
    ['npm', ['--version']],
  ]) {
    run(command, args, { capture: true })
  }

  const nodeMajor = Number(process.versions.node.split('.')[0])
  if (nodeMajor < 22) throw new Error(`Node.js 22 or newer is required; found ${process.version}`)
}

function auditReleaseTree() {
  const tracked = run('git', ['ls-files', '--', 'data', '.tmp-qa'], { capture: true })
  if (tracked) {
    throw new Error(`Release blocked: runtime or QA files are tracked:\n${tracked}`)
  }
}

export function runReleaseCheck() {
  requirePrerequisites()
  auditReleaseTree()
  for (const step of RELEASE_STEPS) {
    process.stdout.write(`\n==> ${step.name}\n`)
    run(step.command, step.args)
  }
  process.stdout.write('\nRelease candidate verification passed.\n')
}

const invokedPath = process.argv[1] ? path.resolve(process.argv[1]) : ''
if (invokedPath === fileURLToPath(import.meta.url)) {
  try {
    runReleaseCheck()
  } catch (error) {
    process.stderr.write(`Release candidate verification failed: ${error.message}\n`)
    process.exitCode = 1
  }
}
