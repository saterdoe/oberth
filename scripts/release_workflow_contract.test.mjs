import assert from 'node:assert/strict'
import fs from 'node:fs'

const workflow = fs.readFileSync(new URL('../.github/workflows/release.yml', import.meta.url), 'utf8')
const collector = fs.readFileSync(new URL('./collect-release-assets.sh', import.meta.url), 'utf8')
const integrationRunner = fs.readFileSync(new URL('../extensions/vscode/test/integration/runTest.cjs', import.meta.url), 'utf8')

assert.match(workflow, /push:\s*\n\s*tags: \["v\*"\]/, 'version tags must trigger the release workflow')
assert.match(workflow, /publish:\s*\n\s*name: Publish GitHub Release/, 'release workflow must have one publish job')
assert.match(workflow, /needs: artifacts/, 'publishing must wait for every platform artifact')
assert.match(workflow, /smoke-artifacts:/, 'packaged artifacts must be smoke-tested on native runners')
assert.match(workflow, /needs: (?:smoke-artifacts|\[smoke-artifacts, vscode-extension\])/, 'publishing must wait for packaged artifact smoke tests')
assert.match(workflow, /if: always\(\)/, 'smoke diagnostics must be retained after failures')
assert.match(workflow, /vscode-extension:/, 'each release must package the VS Code extension')
assert.match(workflow, /cmp vsix-a\/oberth-vscode\.vsix vsix-b\/oberth-vscode\.vsix/, 'VSIX output must be reproducible')
assert.match(workflow, /test:integration/, 'the packaged extension must be installed and activated')
assert.match(integrationRunner, /extensionDevelopmentPath: path\.resolve\(__dirname, 'harness'\)/, 'VSIX activation must use a separate test harness')
assert.ok(workflow.indexOf('output-file: dist/sbom.spdx.json') < workflow.indexOf('sha256sum * > SHA256SUMS'), 'the SBOM must exist before package checksums are generated')
assert.match(workflow, /needs: \[smoke-artifacts, vscode-extension\]/, 'publishing must wait for VSIX verification')
assert.match(workflow, /contents: write/, 'only the publish job may write release content')
assert.match(workflow, /gh release create/, 'tag builds must create a GitHub Release')
assert.match(workflow, /--verify-tag/, 'publishing must verify the version tag')
assert.match(workflow, /--generate-notes/, 'publishing must include release notes')
assert.match(collector, /sha256sum \*/, 'every published asset must receive a combined checksum manifest')
for (const legalFile of ['LICENSE', 'NOTICE', 'THIRD_PARTY_NOTICES.md']) {
  assert.ok(collector.includes(legalFile), `published assets must include ${legalFile}`)
}
assert.match(collector, /\.spdx\.json/, 'each platform SBOM must remain a release asset')
assert.match(collector, /vscode-extension\/\*\.vsix/, 'the verified VSIX must be included in release assets')

console.log('Release workflow contract passed.')
