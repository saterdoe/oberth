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
  'docs/SUPPORT_MATRIX.md': [
    'Windows 11 / Server 2022+',
    'Ubuntu 22.04+',
    'macOS 14+',
    'amd64',
    'arm64',
    'Tooling compatibility',
    'WSL',
    'Support policy',
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
assert.ok(read('README.md').includes('docs/SUPPORT_MATRIX.md'), 'README must link the support matrix')
assert.ok(read('docs/QUICKSTART.md').includes('SUPPORT_MATRIX.md'), 'Quickstart must link the support matrix')

const ci = read('.github/workflows/ci.yml')
const release = read('.github/workflows/release.yml')
for (const target of ['linux/amd64', 'linux/arm64', 'darwin/amd64', 'darwin/arm64', 'windows/amd64']) {
  const [goos, goarch] = target.split('/')
  assert.ok(ci.includes(`goos: ${goos}`) && ci.includes(`goarch: ${goarch}`), `CI must include ${target}`)
  assert.ok(release.includes(`goos: ${goos}`) && release.includes(`goarch: ${goarch}`), `release must include ${target}`)
}
assert.ok(release.includes('oberth-${{ matrix.goos }}-${{ matrix.goarch }}'), 'release assets must include OS and architecture')

console.log('Documentation contract passed.')
