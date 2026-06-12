#!/usr/bin/env node
import { spawn } from 'node:child_process'
import { createRequire } from 'node:module'
import fs from 'node:fs/promises'
import fsSync from 'node:fs'
import net from 'node:net'
import path from 'node:path'
import process from 'node:process'
import { performance } from 'node:perf_hooks'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const repoRoot = path.resolve(__dirname, '..')
const require = createRequire(path.join(repoRoot, 'web/frontend/package.json'))

const SETTINGS_KEY = 'cyberagent_console_settings'
const CHROME_PATH = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'

const DEFAULTS = {
  runs: 1000,
  events: 20,
  pageLimit: 1000,
  apiIterations: 5,
  renderLimit: 500,
  skipBrowser: false,
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const nowID = () => new Date().toISOString().replace(/[:.]/g, '-')

function parseArgs(argv) {
  const options = { ...DEFAULTS }
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i]
    const nextValue = () => {
      const value = argv[++i]
      if (value === undefined) throw new Error(`${arg} requires a value`)
      return value
    }
    switch (arg) {
    case '--runs':
      options.runs = parsePositiveInt(nextValue(), arg)
      break
    case '--events':
    case '--events-per-run':
      options.events = parsePositiveInt(nextValue(), arg)
      break
    case '--page-limit':
      options.pageLimit = parsePositiveInt(nextValue(), arg)
      break
    case '--api-iterations':
      options.apiIterations = parsePositiveInt(nextValue(), arg)
      break
    case '--render-limit':
      options.renderLimit = parsePositiveInt(nextValue(), arg)
      break
    case '--skip-browser':
      options.skipBrowser = true
      break
    case '--help':
    case '-h':
      printHelp()
      process.exit(0)
      break
    default:
      throw new Error(`unknown argument: ${arg}`)
    }
  }
  options.pageLimit = Math.min(options.pageLimit, 1000)
  options.renderLimit = Math.min(options.renderLimit, options.pageLimit, options.runs)
  return options
}

function parsePositiveInt(value, label) {
  const parsed = Number(value)
  if (!Number.isInteger(parsed) || parsed <= 0) {
    throw new Error(`${label} must be a positive integer, got ${value}`)
  }
  return parsed
}

function printHelp() {
  console.log(`Usage: node scripts/run-list-benchmark.mjs [options]

Options:
  --runs <n>             synthetic run workspaces to generate (default ${DEFAULTS.runs})
  --events <n>           JSONL events per run (default ${DEFAULTS.events})
  --page-limit <n>       /api/v1/runs page size, capped by daemon at 1000 (default ${DEFAULTS.pageLimit})
  --api-iterations <n>   measured API request iterations after one warmup (default ${DEFAULTS.apiIterations})
  --render-limit <n>     items fulfilled to /history for browser render baseline (default ${DEFAULTS.renderLimit})
  --skip-browser         skip Vite + Playwright /history render baseline
`)
}

async function getFreePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer()
    server.unref()
    server.on('error', reject)
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      const port = typeof address === 'object' && address ? address.port : 0
      server.close(() => resolve(port))
    })
  })
}

async function waitUntil(label, fn, { timeout = 45_000, interval = 250 } = {}) {
  const deadline = Date.now() + timeout
  let lastError
  while (Date.now() < deadline) {
    try {
      const value = await fn()
      if (value) return value
    } catch (error) {
      lastError = error
    }
    await sleep(interval)
  }
  const suffix = lastError ? ` Last error: ${lastError.message}` : ''
  throw new Error(`${label} timed out after ${timeout}ms.${suffix}`)
}

function spawnManaged(name, command, args, options = {}) {
  const child = spawn(command, args, {
    cwd: repoRoot,
    env: { ...process.env, ...(options.env || {}) },
    detached: true,
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  let log = ''
  const append = (stream, chunk) => {
    const text = chunk.toString()
    const lines = text.split('\n').filter(Boolean).map((line) => `[${name}:${stream}] ${line}`)
    log += `${lines.join('\n')}\n`
    if (log.length > 24_000) log = log.slice(-24_000)
  }
  child.stdout?.on('data', (chunk) => append('stdout', chunk))
  child.stderr?.on('data', (chunk) => append('stderr', chunk))
  return {
    name,
    child,
    getLog: () => log.trim(),
    stop: () => stopProcessGroup(child),
  }
}

function stopProcessGroup(child) {
  if (!child || child.killed || !child.pid) return
  try {
    process.kill(-child.pid, 'SIGTERM')
  } catch {
    try { child.kill('SIGTERM') } catch { /* noop */ }
  }
}

async function readTrimmedFile(filePath) {
  return (await fs.readFile(filePath, 'utf8')).trim()
}

async function writeJSON(filePath, value) {
  await fs.mkdir(path.dirname(filePath), { recursive: true })
  await fs.writeFile(filePath, `${JSON.stringify(value, null, 2)}\n`)
}

async function generateSyntheticRuns(runsDir, { runs, events }) {
  const createdBase = Date.now()
  await fs.mkdir(runsDir, { recursive: true })
  for (let i = 0; i < runs; i++) {
    const runID = `bench_run_${String(i + 1).padStart(5, '0')}`
    const runDir = path.join(runsDir, runID)
    const createdAt = new Date(createdBase - i * 60_000).toISOString()
    const finishedAt = new Date(createdBase - i * 60_000 + events * 1000).toISOString()
    const status = i % 19 === 0 ? 'failed' : (i % 11 === 0 ? 'running' : 'succeeded')
    const target = `https://bench-${String(i + 1).padStart(5, '0')}.example.test`
    const profile = i % 3 === 0 ? 'safe' : (i % 3 === 1 ? 'balanced' : 'audit')

    await fs.mkdir(path.join(runDir, 'reports'), { recursive: true })
    await fs.mkdir(path.join(runDir, 'artifacts'), { recursive: true })
    await fs.mkdir(path.join(runDir, 'evidence'), { recursive: true })

    await writeJSON(path.join(runDir, 'metadata.json'), {
      schema_version: 'benchmark.v1',
      created_at: createdAt,
      job_id: runID,
      workspace: runDir,
      profile,
      request: {
        target,
        scope: target,
        profile,
        out: runDir,
        no_external_tools: true,
        interactive: false,
      },
    })
    await writeJSON(path.join(runDir, 'effective_config.json'), {
      schema_version: 'benchmark.v1',
      generated_at: createdAt,
      profile,
      target,
      scope: target,
      out: runDir,
      no_external_tools: true,
      interactive: false,
      target_id: `target_${i + 1}`,
      scope_id: `scope_${i + 1}`,
      job_id: runID,
    })
    await writeJSON(path.join(runDir, 'job.json'), {
      id: runID,
      kind: 'web_scan',
      status,
      target_id: `target_${i + 1}`,
      scope_id: `scope_${i + 1}`,
      profile,
      workspace: runDir,
      created_at: createdAt,
      started_at: createdAt,
      finished_at: status === 'running' ? null : finishedAt,
      options: {
        interactive: false,
        use_llm: false,
        allow_external_tools: false,
        report_formats: ['json', 'markdown'],
      },
      summary: {
        steps_total: events,
        steps_succeeded: status === 'failed' ? Math.max(0, events - 1) : events,
        steps_failed: status === 'failed' ? 1 : 0,
        findings_total: i % 7 === 0 ? 2 : 0,
      },
    })

    const eventLines = []
    for (let e = 0; e < events; e++) {
      eventLines.push(JSON.stringify({
        id: `event_${String(e + 1).padStart(4, '0')}`,
        job_id: runID,
        step_id: `step_${String(e + 1).padStart(4, '0')}`,
        type: e === 0 ? 'job.started' : (e === events - 1 ? 'job.finished' : 'tool.finished'),
        level: e === events - 1 && status === 'failed' ? 'error' : 'info',
        timestamp: new Date(createdBase - i * 60_000 + e * 1000).toISOString(),
        message: `benchmark event ${e + 1}`,
      }))
    }
    await fs.writeFile(path.join(runDir, 'events.jsonl'), `${eventLines.join('\n')}\n`)
    await fs.writeFile(path.join(runDir, 'approvals.json'), '[]\n')
    await writeJSON(path.join(runDir, 'findings.json'), i % 7 === 0 ? [
      {
        id: `finding_${i + 1}_1`,
        job_id: runID,
        title: 'Benchmark finding',
        severity: 'low',
        confidence: 'high',
        category: 'benchmark',
        affected_targets: [target],
        evidence_ids: [`evidence_${i + 1}_1`],
        description: 'Synthetic finding for run list benchmark.',
        impact: 'No impact; synthetic data.',
        reproduction: ['Open the synthetic benchmark run.'],
        remediation: 'N/A',
        source: { tool: 'benchmark', step_id: 'step_0001' },
        created_at: createdAt,
      },
    ] : [])
    await writeJSON(path.join(runDir, 'artifacts/index.json'), {
      schema_version: 'benchmark.v1',
      generated_at: createdAt,
      count: i % 5,
      artifacts: [],
    })
    await writeJSON(path.join(runDir, 'reports/report.json'), {
      run_id: runID,
      target,
      status,
      generated_at: finishedAt,
    })
    await fs.writeFile(path.join(runDir, 'reports/report.md'), `# ${runID}\n\nSynthetic benchmark report.\n`)
  }
}

async function daemonJSON(baseUrl, token, apiPath) {
  const response = await fetch(`${baseUrl}${apiPath}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  const body = await response.json().catch(() => null)
  if (!response.ok || (body && typeof body === 'object' && body.ok === false)) {
    const message = body?.error?.message || response.statusText || 'request failed'
    throw new Error(`${apiPath} failed: ${response.status} ${message}`)
  }
  if (body && typeof body === 'object' && 'data' in body) return body.data
  return body
}

async function benchmarkRunListAPI(baseUrl, token, pageLimit, iterations) {
  const endpoint = `/api/v1/runs?limit=${pageLimit}`
  await daemonJSON(baseUrl, token, endpoint)

  const durations = []
  let page
  for (let i = 0; i < iterations; i++) {
    const started = performance.now()
    page = await daemonJSON(baseUrl, token, endpoint)
    durations.push(performance.now() - started)
  }
  return { endpoint, durations, page }
}

async function benchmarkHistoryRender({ daemonBaseUrl, token, frontendBaseUrl, runsPage, renderLimit }) {
  let chromium
  try {
    chromium = require('playwright').chromium
  } catch (error) {
    return {
      enabled: false,
      skipped: true,
      reason: `playwright unavailable: ${error.message}`,
    }
  }

  const scanItems = (runsPage.items || []).slice(0, renderLimit).map(runSummaryToScanItem)
  const browserLog = []
  let browser
  let page
  let scansFulfilledAt = 0
  let scansRequests = 0

  try {
    browser = await launchChromium(chromium)
    const context = await browser.newContext({ viewport: { width: 1440, height: 960 } })
    await context.addInitScript(({ key, settings }) => {
      window.localStorage.setItem(key, JSON.stringify(settings))
    }, {
      key: SETTINGS_KEY,
      settings: {
        baseUrl: daemonBaseUrl,
        token,
        operator: 'run-list-benchmark',
        language: 'zh',
        operationLevel: 'read_only',
        agentProvider: {
          provider: '',
          model: '',
          apiKey: '',
          providerBaseUrl: '',
          instructions: '',
          maxTurns: 1,
        },
        aiProviders: [],
        activeAiProviderId: '',
        apiAuthProfiles: [],
        defaultApiAuthProfileId: '',
        scanDraft: { target: '', scope: '', multiAgent: false, interactive: false },
        autoContinueLimit: 0,
      },
    })

    page = await context.newPage()
    page.on('console', (msg) => {
      if (['error', 'warning'].includes(msg.type())) browserLog.push(`[${msg.type()}] ${msg.text()}`)
    })
    page.on('pageerror', (error) => browserLog.push(`[pageerror] ${error.message}`))
    await page.route('**/api/local/daemon-config', (route) => route.fulfill({ status: 404, body: '' }))
    await page.route('**/api/scans', async (route) => {
      scansRequests += 1
      scansFulfilledAt = scansFulfilledAt || performance.now()
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(scanItems),
      })
    })

    const started = performance.now()
    await page.goto(`${frontendBaseUrl}/history`, { waitUntil: 'domcontentloaded' })
    await page.waitForFunction(
      (expected) => document.querySelectorAll('a[href^="/scan/"]').length >= expected,
      scanItems.length,
      { timeout: 45_000 },
    )
    const finished = performance.now()
    const renderedItems = await page.locator('a[href^="/scan/"]').count()
    return {
      enabled: true,
      skipped: false,
      route: '/history',
      fulfilled_items: scanItems.length,
      rendered_items: renderedItems,
      route_to_items_ms: round(finished - started),
      after_scans_fulfill_ms: scansFulfilledAt ? round(finished - scansFulfilledAt) : null,
      scans_requests: scansRequests,
      browser_warnings: browserLog.slice(-20),
    }
  } finally {
    if (browser) await browser.close().catch(() => {})
  }
}

async function launchChromium(chromium) {
  if (fsSync.existsSync(CHROME_PATH)) {
    return chromium.launch({ executablePath: CHROME_PATH, headless: true })
  }
  try {
    return await chromium.launch({ channel: 'chrome', headless: true })
  } catch {
    return chromium.launch({ headless: true })
  }
}

function runSummaryToScanItem(run) {
  const status = run.status === 'succeeded' || run.status === 'partially_succeeded'
    ? 'completed'
    : (run.status === 'cancelled' ? 'stopped' : (run.status || 'running'))
  return {
    id: run.run_id,
    target: run.target || run.run_id,
    status,
    started_at: run.created_at || run.updated_at || new Date().toISOString(),
    duration: '1m',
    findings: run.counts?.findings || 0,
    turns: run.counts?.events || 0,
    max_turns: Math.max(1, run.counts?.events || 1),
    provider: run.profile || 'benchmark',
    event_count: run.counts?.events || 0,
    artifact_count: run.counts?.artifacts || 0,
    approval_count: run.counts?.approvals || 0,
    pending_approvals: run.counts?.pending_approvals || 0,
  }
}

function summarizeDurations(values) {
  const sorted = [...values].sort((a, b) => a - b)
  const sum = sorted.reduce((acc, value) => acc + value, 0)
  return {
    iterations: values.length,
    min_ms: round(sorted[0] || 0),
    mean_ms: round(values.length ? sum / values.length : 0),
    p50_ms: round(percentile(sorted, 0.50)),
    p95_ms: round(percentile(sorted, 0.95)),
    max_ms: round(sorted[sorted.length - 1] || 0),
    samples_ms: values.map(round),
  }
}

function percentile(sorted, p) {
  if (!sorted.length) return 0
  const idx = Math.min(sorted.length - 1, Math.max(0, Math.ceil(sorted.length * p) - 1))
  return sorted[idx]
}

function round(value) {
  return Math.round(value * 100) / 100
}

function countStatuses(items = []) {
  return items.reduce((acc, item) => {
    const key = item.status || 'unknown'
    acc[key] = (acc[key] || 0) + 1
    return acc
  }, {})
}

async function main() {
  const options = parseArgs(process.argv.slice(2))
  const workRoot = path.join(repoRoot, '.tmp/bench/run-list', `run-list-benchmark-${nowID()}`)
  const runsDir = path.join(workRoot, 'runs')
  const tokenFile = path.join(workRoot, 'token')
  const goTmpDir = path.join(repoRoot, '.tmp/go-tmp')
  const goCacheDir = path.join(repoRoot, '.tmp/go-build')
  await fs.mkdir(goTmpDir, { recursive: true })
  await fs.mkdir(goCacheDir, { recursive: true })

  const processes = []
  const printLogs = () => {
    for (const proc of processes) {
      const log = proc.getLog()
      if (log) console.error(`\n--- ${proc.name} log tail ---\n${log}`)
    }
  }

  try {
    const fixtureStarted = performance.now()
    await generateSyntheticRuns(runsDir, options)
    const fixtureMs = round(performance.now() - fixtureStarted)

    const daemonPort = await getFreePort()
    const daemonBaseUrl = `http://127.0.0.1:${daemonPort}`
    const daemon = spawnManaged('daemon', 'go', [
      'run', './cmd/cyberagent', 'daemon',
      '--listen', `127.0.0.1:${daemonPort}`,
      '--runs-dir', runsDir,
      '--token-file', tokenFile,
    ], {
      env: {
        GOCACHE: goCacheDir,
        GOTMPDIR: goTmpDir,
      },
    })
    processes.push(daemon)

    const token = await waitUntil('daemon token file', async () => {
      if (!fsSync.existsSync(tokenFile)) return false
      const value = await readTrimmedFile(tokenFile)
      return value || false
    })
    await waitUntil('daemon healthz', async () => {
      const response = await fetch(`${daemonBaseUrl}/api/v1/healthz`, {
        headers: { Authorization: `Bearer ${token}` },
      }).catch(() => null)
      return response?.ok
    })

    const api = await benchmarkRunListAPI(daemonBaseUrl, token, options.pageLimit, options.apiIterations)

    let frontend = { enabled: false, skipped: true, reason: 'browser baseline skipped' }
    if (!options.skipBrowser) {
      const frontendPort = await getFreePort()
      const frontendBaseUrl = `http://127.0.0.1:${frontendPort}`
      const vite = spawnManaged('vite', 'npm', [
        '--prefix', 'web/frontend', 'run', 'dev', '--',
        '--host', '127.0.0.1',
        '--port', String(frontendPort),
        '--strictPort',
      ], { env: { BROWSER: 'none' } })
      processes.push(vite)
      await waitUntil('Vite frontend', async () => {
        const response = await fetch(frontendBaseUrl).catch(() => null)
        return response && response.status < 500
      })
      frontend = await benchmarkHistoryRender({
        daemonBaseUrl,
        token,
        frontendBaseUrl,
        runsPage: api.page,
        renderLimit: options.renderLimit,
      })
    }

    const result = {
      ok: true,
      scenario: {
        runs: options.runs,
        events_per_run: options.events,
        total_events: options.runs * options.events,
        page_limit: options.pageLimit,
        api_iterations: options.apiIterations,
        render_limit: options.renderLimit,
      },
      workspace: {
        work_root: workRoot,
        runs_dir: runsDir,
      },
      fixture: {
        generated_ms: fixtureMs,
      },
      api: {
        endpoint: api.endpoint,
        ...summarizeDurations(api.durations),
        first_page: {
          items: api.page?.items?.length || 0,
          eof: Boolean(api.page?.eof),
          next_cursor: api.page?.next_cursor || '',
          first_run_id: api.page?.items?.[0]?.run_id || '',
          last_run_id: api.page?.items?.[(api.page?.items?.length || 1) - 1]?.run_id || '',
          status_counts: countStatuses(api.page?.items || []),
          counted_events: (api.page?.items || []).reduce((sum, item) => sum + (item.counts?.events || 0), 0),
        },
      },
      frontend,
    }
    console.log(JSON.stringify(result, null, 2))
  } catch (error) {
    printLogs()
    console.error(error.stack || error.message)
    process.exitCode = 1
  } finally {
    for (const proc of processes.reverse()) proc.stop()
  }
}

main()
