import { createServer } from 'node:http'
import { readFile, mkdir } from 'node:fs/promises'
import { extname, normalize } from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium } from 'playwright'

const root = new URL('../ui/dist/', import.meta.url)
const artifacts = new URL('../artifacts/e2e/', import.meta.url)
const artifactPath = name => fileURLToPath(new URL(name, artifacts))
await mkdir(artifacts, { recursive: true })

const now = new Date().toISOString()
const fixture = () => ({
  decisions: [], exports: 0,
  projects: [{ id: 'project-1', name: 'Demo', path: '/fixtures/demo' }],
  tasks: [
    { id: 'task-review', repository_id: 'project-1', title: 'Review recovery', description: 'Inspect the recovered change', task_type: 'implementation', status: 'review', created_at: now, updated_at: now },
    { id: 'task-recovered', repository_id: 'project-1', title: 'Recovered run', description: 'Interrupted safely', task_type: 'implementation', status: 'blocked', created_at: now, updated_at: now },
  ],
})
let state = fixture()

const data = value => JSON.stringify({ data: value })
const send = (response, value, status = 200, headers = {}) => {
  response.writeHead(status, { 'content-type': 'application/json', ...headers })
  response.end(typeof value === 'string' ? value : data(value))
}

const sessions = () => state.tasks.map(task => ({
  id: `session-${task.id}`, task_id: task.id, task_description: task.description, task_type: task.task_type,
  repo_path: '/fixtures/demo', branch: `oberth/${task.id}`, status: task.status === 'review' ? 'review' : task.status,
  model: 'fixture-model', tokens_input: 10, tokens_output: 5, cost: 0, started_at: now, plan: [],
}))
const runs = () => state.tasks.map(task => ({
  id: `run-${task.id}`, task_id: task.id, session_id: `session-${task.id}`, state: task.status === 'blocked' ? 'interrupted' : task.status,
  base_repository: '/fixtures/demo', base_commit: 'abcdef123456', worktree_path: `/fixtures/worktrees/${task.id}`,
  branch: `oberth/${task.id}`, started_at: now,
}))
const resultBundle = {
  verification_status: 'passed', diff_hash: `sha256:${'a'.repeat(64)}`, summary: 'Verified fixture change',
  diff: [{ path: 'README.md', status: 'modified', content: 'diff --git a/README.md b/README.md\n+release ready\n' }],
  warnings: [], cost: 0, tokens_input: 10, tokens_output: 5,
}

const server = createServer(async (request, response) => {
  const url = new URL(request.url, 'http://127.0.0.1')
  if (url.pathname.startsWith('/api/v1/')) {
    if (request.method === 'GET' && url.pathname === '/api/v1/status') return send(response, { server: { state: 'healthy', version: 'e2e' }, vault: { state: 'healthy', note_count: 0 } })
    if (request.method === 'GET' && url.pathname === '/api/v1/projects') return send(response, state.projects)
    if (request.method === 'GET' && url.pathname === '/api/v1/tasks') return send(response, { tasks: state.tasks })
    if (request.method === 'GET' && url.pathname === '/api/v1/sessions') return send(response, { sessions: sessions() })
    if (request.method === 'GET' && url.pathname === '/api/v1/runs') return send(response, runs())
    if (request.method === 'GET' && url.pathname === '/api/v1/runs') return send(response, { runs: runs() })
    if (request.method === 'GET' && url.pathname === '/api/v1/providers') return send(response, [])
    if (request.method === 'GET' && url.pathname === '/api/v1/costs') return send(response, {})
    if (request.method === 'GET' && url.pathname.includes('/promotion-readiness')) return send(response, { ready: true })
    if (request.method === 'GET' && /\/runs\/run-task-[^/]+\/events/.test(url.pathname)) {
      const recovered = url.pathname.includes('task-recovered')
      return send(response, { events: recovered ? [{ sequence: 1, type: 'run_interrupted', payload: { artifacts_preserved: true } }] : [] })
    }
    if (request.method === 'GET' && /\/runs\/run-task-[^/]+$/.test(url.pathname)) {
      const run = runs().find(item => url.pathname.endsWith(item.id))
      return send(response, { ...run, result_bundle: run?.task_id === 'task-review' ? resultBundle : {} })
    }
    if (request.method === 'GET' && /\/tasks\/task-/.test(url.pathname)) {
      return send(response, state.tasks.find(task => url.pathname.endsWith(task.id)))
    }
    if (request.method === 'GET' && url.pathname.includes('/export')) {
      state.exports++
      response.writeHead(200, { 'content-type': 'text/markdown', 'content-disposition': 'attachment; filename="oberth-run.md"' })
      return response.end('# Oberth run\n\nVerified fixture change.\n')
    }
    if (request.method === 'POST' && url.pathname === '/api/v1/tasks') {
      let body = ''
      for await (const chunk of request) body += chunk
      const input = JSON.parse(body)
      const task = { id: `task-created-${state.tasks.length}`, repository_id: input.repository_id, title: input.title, description: input.description, task_type: input.task_type, status: 'pending', created_at: now, updated_at: now }
      state.tasks.unshift(task)
      return send(response, task, 201)
    }
    if (request.method === 'POST' && /\/tasks\/task-created-\d+\/run$/.test(url.pathname)) {
      const id = url.pathname.split('/').at(-2)
      state.tasks.find(task => task.id === id).status = 'running'
      return send(response, { run_id: `run-${id}` })
    }
    if (request.method === 'POST' && url.pathname.endsWith('/outcome')) {
      let body = ''
      for await (const chunk of request) body += chunk
      const input = JSON.parse(body)
      state.decisions.push(input.outcome)
      state.tasks.find(task => task.id === 'task-review').status = input.outcome === 'accepted' ? 'completed' : 'cancelled'
      return send(response, { outcome: input.outcome })
    }
    if (request.method === 'POST') return send(response, {})
    return send(response, [])
  }
  const relative = url.pathname === '/' ? 'index.html' : normalize(url.pathname).replace(/^[/\\]+/, '')
  try {
    const content = await readFile(new URL(relative, root))
    const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml' }
    response.writeHead(200, { 'content-type': types[extname(relative)] || 'application/octet-stream' })
    response.end(content)
  } catch {
    const content = await readFile(new URL('index.html', root))
    response.writeHead(200, { 'content-type': 'text/html' })
    response.end(content)
  }
})
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
const origin = `http://127.0.0.1:${server.address().port}`
// Manual UI inspection uses the same isolated fixtures, never a developer vault.
if (process.argv.includes('--serve')) {
  console.log(`Fixture QA server: ${origin}`)
  await new Promise(() => {})
}
const waitForFixture = async (predicate, label) => {
  const deadline = Date.now() + 2000
  while (!predicate()) {
    if (Date.now() >= deadline) throw new Error(`${label} was not observed`)
    await new Promise(resolve => setTimeout(resolve, 20))
  }
}

const browser = await chromium.launch({ headless: true })
const runJourney = async locale => {
  state = fixture()
  const context = await browser.newContext({ locale: locale === 'es' ? 'es-AR' : 'en-US', acceptDownloads: true })
  await context.addInitScript(value => {
    localStorage.setItem('oberth.locale', value)
    localStorage.setItem('oberth.product-tour.v1', 'completed')
  }, locale)
  await context.tracing.start({ screenshots: true, snapshots: true, sources: true })
  const page = await context.newPage()
  page.on('pageerror', error => console.error(`[${locale}] page error:`, error.stack || error.message))
  try {
    await page.goto(origin)
    await page.getByRole('heading', { name: locale === 'es' ? 'Inicio' : 'Home' }).waitFor()
    await page.getByRole('button', { name: locale === 'es' ? 'Sesión' : 'Session', exact: true }).click()
    await page.locator('.repository-field select').selectOption('project-1')
    await page.locator('.intention-field textarea').fill(locale === 'es' ? 'Preparar una release segura' : 'Prepare a safe release')
    await page.getByRole('button', { name: /Demo/ }).first().click()
    await page.getByText(locale === 'es' ? 'En ejecución' : 'Running').first().waitFor()

    await page.locator('.side-icon').first().click()
    await page.getByText('Review recovery', { exact: true }).click()
    await page.getByText('README.md').first().waitFor()
    await page.locator('.run-evidence .task-toolbar button').first().click()
    await waitForFixture(() => state.exports === 1, 'run export')
    await page.locator('.review-actions .task-toolbar button').first().click()
    await waitForFixture(() => state.decisions.includes('accepted'), 'review decision')

    await page.locator('.side-icon').first().click()
    await page.getByText('Recovered run', { exact: true }).click()
    await page.locator('.technical-details summary').first().click()
    await page.locator('.timeline-item').filter({ hasText: 'run_interrupted', visible: true }).first().waitFor()
    await context.tracing.stop()
  } catch (error) {
    await page.screenshot({ path: artifactPath(`${locale}-failure.png`), fullPage: true })
    await context.tracing.stop({ path: artifactPath(`${locale}-trace.zip`) })
    throw error
  } finally {
    await context.close()
  }
}

const auditCompactWindows = async () => {
  state = fixture()
  const context = await browser.newContext({ viewport: { width: 1024, height: 768 }, locale: 'en-US' })
  await context.addInitScript(() => {
    localStorage.setItem('oberth.locale', 'en')
    localStorage.setItem('oberth.product-tour.v1', 'completed')
  })
  const page = await context.newPage()
  const assertNoGlobalOverflow = async label => {
    const dimensions = await page.evaluate(() => ({ client: document.documentElement.clientWidth, scroll: document.documentElement.scrollWidth }))
    if (dimensions.scroll > dimensions.client) throw new Error(`${label} has global horizontal overflow: ${dimensions.scroll}px > ${dimensions.client}px`)
  }
  try {
    await page.goto(origin)
    await page.getByRole('heading', { name: 'Home' }).waitFor()
    await assertNoGlobalOverflow('Home at 1024x768')
    for (const name of ['Session', 'Vault', 'Routes', 'Costs', 'Settings']) {
      await page.getByRole('button', { name, exact: true }).click()
      await assertNoGlobalOverflow(`${name} at 1024x768`)
    }
    await page.setViewportSize({ width: 800, height: 600 })
    await page.getByRole('button', { name: 'Session', exact: true }).click()
    await assertNoGlobalOverflow('Session at 800x600')
    if (await page.locator('.side-label').first().isVisible()) throw new Error('Compact navigation did not collapse at 800px')
    const primary = page.locator('.task-create>button[type="submit"]')
    await primary.scrollIntoViewIfNeeded()
    if (!await primary.isVisible()) throw new Error('Primary task action is not visible in a compact window')
  } catch (error) {
    await page.screenshot({ path: artifactPath('compact-window-failure.png'), fullPage: true })
    throw error
  } finally {
    await context.close()
  }
}

try {
  await runJourney('en')
  await runJourney('es')
  await auditCompactWindows()
  console.log('Browser E2E passed: bilingual journeys and compact-window layout at 1024x768 and 800x600.')
} finally {
  await browser.close()
  await new Promise(resolve => server.close(resolve))
}
