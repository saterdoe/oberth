import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'

const root = path.resolve(import.meta.dirname, '..')
const versionFile = path.join(root, 'VERSION')
const semver = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/
const read = relative => fs.readFileSync(path.join(root, relative), 'utf8')
const write = (relative, content) => fs.writeFileSync(path.join(root, relative), content)
const canonical = () => fs.readFileSync(versionFile, 'utf8').trim()
const windowsVersion = version => {
  const parts = version.split('-', 1)[0].split('.').map(Number)
  return [...parts, 0].slice(0, 4).join('.')
}

const jsonTargets = ['package.json', 'ui/package.json', 'extensions/vscode/package.json']
const lockTargets = ['package-lock.json', 'ui/package-lock.json', 'extensions/vscode/package-lock.json']
const textTargets = [
  'README.md',
  'CONTRIBUTING.md',
  'docs/QUICKSTART.md',
  'SECURITY.md',
  'ui/src/i18n.test.tsx',
]

function replaceFirst(content, from, to, relative) {
  const index = content.indexOf(from)
  assert.notEqual(index, -1, `${relative} must contain ${from}`)
  return content.slice(0, index) + to + content.slice(index + from.length)
}

function updateJson(relative, previous, version) {
  const content = read(relative)
  write(relative, replaceFirst(content, `"version": "${previous}"`, `"version": "${version}"`, relative))
}

function updateLock(relative, previous, version) {
  let content = read(relative)
  for (let count = 0; count < 2; count += 1) {
    content = replaceFirst(content, `"version": "${previous}"`, `"version": "${version}"`, relative)
  }
  write(relative, content)
}

function replaceVersion(relative, previous, version) {
  const content = read(relative)
  assert.ok(content.includes(previous), `${relative} must contain ${previous}`)
  write(relative, content.replaceAll(previous, version))
}

function setVersion(version) {
  assert.match(version, semver, `invalid semantic version: ${version}`)
  const previous = canonical()
  for (const target of jsonTargets) updateJson(target, previous, version)
  for (const target of lockTargets) updateLock(target, previous, version)
  for (const target of textTargets) replaceVersion(target, previous, version)

  const buildInfo = read('internal/buildinfo/version.go').replace(
    /var Version = "[^"]+"/,
    `var Version = "${version}"`,
  )
  write('internal/buildinfo/version.go', buildInfo)

  let winres = read('desktop/winres.json')
  winres = winres.replaceAll(windowsVersion(previous), windowsVersion(version))
  winres = winres.replaceAll(previous, version)
  write('desktop/winres.json', winres)
  fs.writeFileSync(versionFile, `${version}\n`)
  console.log(`Updated Oberth version from ${previous} to ${version}.`)
}

function checkVersion() {
  const version = canonical()
  assert.match(version, semver, 'VERSION must contain a valid semantic version')
  for (const target of jsonTargets) assert.equal(JSON.parse(read(target)).version, version, `${target} version`)
  for (const target of lockTargets) {
    const lock = JSON.parse(read(target))
    assert.equal(lock.version, version, `${target} version`)
    assert.equal(lock.packages?.['']?.version, version, `${target} root package version`)
  }
  assert.ok(read('internal/buildinfo/version.go').includes(`var Version = "${version}"`), 'Go runtime version must match VERSION')
  const resource = JSON.parse(read('desktop/winres.json')).RT_VERSION['#1']['0000']
  assert.equal(resource.fixed.file_version, windowsVersion(version), 'Windows file version')
  assert.equal(resource.fixed.product_version, windowsVersion(version), 'Windows product version')
  assert.equal(resource.info['0409'].FileVersion, version, 'Windows display file version')
  assert.equal(resource.info['0409'].ProductVersion, version, 'Windows display product version')
  for (const target of textTargets) assert.ok(read(target).includes(version), `${target} must reference ${version}`)
  assert.ok(read('CHANGELOG.md').includes(`## ${version}`), `CHANGELOG.md must contain a ${version} release section`)
  console.log(`Version contract passed (${version}).`)
}

const setIndex = process.argv.indexOf('--set')
if (setIndex >= 0) {
  const version = process.argv[setIndex + 1]
  assert.ok(version, 'usage: node scripts/version.mjs --set <semver>')
  setVersion(version)
}
checkVersion()
