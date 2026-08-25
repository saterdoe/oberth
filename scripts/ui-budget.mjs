import fs from 'node:fs'
import path from 'node:path'
import zlib from 'node:zlib'

const root = path.resolve(import.meta.dirname, '..')
const assets = path.join(root, 'ui', 'dist', 'assets')
const output = process.env.OBERTH_UI_BUDGET_REPORT ?? path.join(root, 'artifacts', 'performance', 'ui-bundle.json')
const limits = { javascriptGzip: 100 * 1024, cssGzip: 25 * 1024, totalRaw: 500 * 1024 }
if (!fs.existsSync(assets)) throw new Error('ui/dist is missing; run the UI build first')

let javascriptGzip = 0, cssGzip = 0, totalRaw = 0
for (const name of fs.readdirSync(assets).sort()) {
  const data = fs.readFileSync(path.join(assets, name))
  totalRaw += data.length
  const compressed = zlib.gzipSync(data, { level: 9 }).length
  if (name.endsWith('.js')) javascriptGzip += compressed
  if (name.endsWith('.css')) cssGzip += compressed
}
const measurements = { javascriptGzip, cssGzip, totalRaw }
const failures = Object.entries(limits).filter(([key, limit]) => measurements[key] > limit)
fs.mkdirSync(path.dirname(output), { recursive: true })
fs.writeFileSync(output, JSON.stringify({ schema_version: '1', measurements, limits }, null, 2))
if (failures.length) {
  for (const [key, limit] of failures) console.error(`${key} ${measurements[key]} exceeds ${limit}`)
  process.exit(1)
}
console.log(`UI bundle budget passed: JS gzip ${javascriptGzip}, CSS gzip ${cssGzip}, raw ${totalRaw}.`)
