import assert from 'node:assert/strict'
import fs from 'node:fs'

const workflow = fs.readFileSync(new URL('../.github/workflows/release.yml', import.meta.url), 'utf8')
const collector = fs.readFileSync(new URL('./collect-release-assets.sh', import.meta.url), 'utf8')

assert.match(workflow, /push:\s*\n\s*tags: \["v\*"\]/, 'version tags must trigger the release workflow')
assert.match(workflow, /publish:\s*\n\s*name: Publish GitHub Release/, 'release workflow must have one publish job')
assert.match(workflow, /needs: artifacts/, 'publishing must wait for every platform artifact')
assert.match(workflow, /smoke-artifacts:/, 'packaged artifacts must be smoke-tested on native runners')
assert.match(workflow, /needs: smoke-artifacts/, 'publishing must wait for packaged artifact smoke tests')
assert.match(workflow, /if: always\(\)/, 'smoke diagnostics must be retained after failures')
assert.match(workflow, /contents: write/, 'only the publish job may write release content')
assert.match(workflow, /gh release create/, 'tag builds must create a GitHub Release')
assert.match(workflow, /--verify-tag/, 'publishing must verify the version tag')
assert.match(workflow, /--generate-notes/, 'publishing must include release notes')
assert.match(collector, /sha256sum oberth-\*/, 'published assets must receive a combined checksum manifest')
assert.match(collector, /\.spdx\.json/, 'each platform SBOM must remain a release asset')

console.log('Release workflow contract passed.')
