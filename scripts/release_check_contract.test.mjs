import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { RELEASE_STEPS } from './release-check.mjs'

const root = path.resolve(import.meta.dirname, '..')
const read = relative => fs.readFileSync(path.join(root, relative), 'utf8')

assert.deepEqual(
  RELEASE_STEPS.map(step => step.name),
  [
    'Install root dependencies',
    'Install UI dependencies',
    'Install VS Code extension dependencies',
    'Go tests',
    'Isolated performance budgets',
    'Go vet',
    'Go builds',
    'Race detector',
    'Durable E2E',
    'UI tests',
    'UI build',
    'Documentation contract',
    'Release pipeline contract',
    'Publishing workflow contract',
  ],
  'the release gate order is a versioned platform contract',
)

for (const script of ['scripts/check-release.ps1', 'scripts/check-release.sh']) {
  const content = read(script)
  assert.ok(content.includes('release-check.mjs'), `${script} must delegate to the shared runner`)
  assert.ok(!content.includes('optional web checks skipped'), `${script} must not silently skip UI gates`)
}

const workflow = read('.github/workflows/ci.yml')
assert.deepEqual(RELEASE_STEPS.find(step => step.name === 'Isolated performance budgets').args,
  ['test', '-tags', 'performance', './internal/perfbench', '-run', 'TestRegressionBudgets', '-count=1'],
  'the isolated performance gate must remain mandatory and uncached')
assert.ok(read('internal/perfbench/performance_test.go').startsWith('//go:build performance'))
for (const runner of ['ubuntu-latest', 'macos-latest', 'windows-latest']) {
  assert.ok(workflow.includes(runner), `CI must run the release contract on ${runner}`)
}
assert.ok(workflow.includes('node scripts/release-check.mjs'), 'CI must use the same release runner as local checks')

console.log('Release pipeline contract passed.')
