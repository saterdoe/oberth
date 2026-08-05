import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'

const root = path.resolve(import.meta.dirname, '..')
const read = relative => fs.readFileSync(path.join(root, relative), 'utf8')
const required = {
  'README.md': [
    'Coding agents, under control.',
    'Public Alpha',
    '0.1.0-alpha.2',
    'docs/QUICKSTART.md',
    'Apache-2.0',
  ],
  'docs/QUICKSTART.md': [
    'Requirements',
    'Configure a provider',
    'Run and review a task',
    'Troubleshooting',
  ],
  'docs/ARCHITECTURE.md': [
    'Main components',
    'Task lifecycle',
    'Trust boundaries',
    'Compatibility',
  ],
  'docs/RELEASE_RUNBOOK.md': [
    'Release invariants',
    'Dry run',
    'Cut the release branch',
    'Verify the candidate',
    'Tag and publish',
    'Rollback before publishing',
    'Rollback after publishing',
    'Recovery and evidence',
  ],
  'CONTRIBUTING.md': [
    'Development requirements',
    'Pull requests',
    'Security',
    'Apache License 2.0',
  ],
  'SECURITY.md': [
    'Supported versions',
    'Reporting a vulnerability',
    'Security boundaries',
  ],
  'CHANGELOG.md': ['Unreleased', '0.1.0-alpha.2', '0.1.0-alpha.1'],
  'THIRD_PARTY_NOTICES.md': ['Third-party notices', 'Apache-2.0', 'MPL-2.0'],
  'LICENSE': ['Apache License', 'Version 2.0'],
}

for (const [file, sections] of Object.entries(required)) {
  assert.ok(fs.existsSync(path.join(root, file)), `${file} must exist`)
  const content = read(file)
  for (const section of sections) {
    assert.ok(content.includes(section), `${file} must include ${section}`)
  }
}

const packageVersion = JSON.parse(read('package.json')).version
const uiVersion = JSON.parse(read('ui/package.json')).version
assert.equal(packageVersion, '0.1.0-alpha.2')
assert.equal(uiVersion, packageVersion)
assert.ok(read('README.md').includes('LICENSE'), 'README must link the license')
assert.ok(
  read('README.md').includes('docs/RELEASE_RUNBOOK.md'),
  'README must link the release runbook',
)
assert.ok(
  read('CONTRIBUTING.md').includes('docs/RELEASE_RUNBOOK.md'),
  'CONTRIBUTING must link the release runbook',
)

const runbook = read('docs/RELEASE_RUNBOOK.md')
for (const invariant of [
  'main` is the source of truth',
  'release/<version>',
  'git tag -a v<version>',
  'git merge --ff-only v<version>',
  'Published tags are immutable',
  'Persisted data is never downgraded',
  'Force-pushing or resetting',
]) {
  assert.ok(runbook.includes(invariant), `release runbook must preserve: ${invariant}`)
}

console.log('Documentation contract passed.')
