#!/usr/bin/env node
import { spawn } from 'node:child_process'
import { createRequire } from 'node:module'
import fs from 'node:fs/promises'
import fsSync from 'node:fs'
import net from 'node:net'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const repoRoot = path.resolve(__dirname, '..')
const require = createRequire(path.join(repoRoot, 'web/frontend/package.json'))
const { chromium } = require('playwright')

const SETTINGS_KEY = 'cyberagent_console_settings'
const CHROME_PATH = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const LLM_FAILURE_BASE_URL = 'http://127.0.0.1:1/v1'

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const nowID = () => new Date().toISOString().replace(/[:.]/g, '-')

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

async function waitUntil(label, fn, { timeout = 30_000, interval = 250 } = {}) {
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
    log += text.split('\n').filter(Boolean).map((line) => `[${name}:${stream}] ${line}`).join('\n') + '\n'
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

async function daemonJSON(baseUrl, token, apiPath, options = {}) {
  const headers = new Headers(options.headers || {})
  headers.set('Authorization', `Bearer ${token}`)
  if (options.body) headers.set('Content-Type', 'application/json')
  const response = await fetch(`${baseUrl}${apiPath}`, { ...options, headers })
  const body = await response.json().catch(() => null)
  if (!response.ok || (body && typeof body === 'object' && body.ok === false)) {
    const message = body?.error?.message || response.statusText || 'request failed'
    throw new Error(`${apiPath} failed: ${response.status} ${message}`)
  }
  if (body && typeof body === 'object' && 'data' in body) return body.data
  return body
}

function smokeSettings(daemonBaseUrl, token) {
  const provider = {
    id: 'browser-smoke-provider',
    name: 'Browser Smoke Provider',
    type: 'openai',
    model: 'browser-smoke-model',
    apiKey: 'browser-smoke-key',
    baseUrl: LLM_FAILURE_BASE_URL,
    instructions: '',
    maxTurns: 1,
  }
  return {
    baseUrl: daemonBaseUrl,
    token,
    operator: 'browser-smoke',
    language: 'zh',
    operationLevel: 'read_only',
    agentProvider: {
      provider: provider.type,
      model: provider.model,
      apiKey: provider.apiKey,
      providerBaseUrl: provider.baseUrl,
      instructions: provider.instructions,
      maxTurns: provider.maxTurns,
    },
    aiProviders: [provider],
    activeAiProviderId: provider.id,
    apiAuthProfiles: [],
    defaultApiAuthProfileId: '',
    scanDraft: { target: '', scope: '', multiAgent: false, interactive: false },
    autoContinueLimit: 0,
  }
}

async function waitForPageText(page, pattern, label, timeout = 45_000) {
  await page.waitForFunction(
    ({ source, flags }) => new RegExp(source, flags).test(document.body.innerText || ''),
    { source: pattern.source, flags: pattern.flags },
    { timeout },
  ).catch(async (error) => {
    const text = await page.locator('body').innerText().catch(() => '')
    throw new Error(`${label} not found. ${error.message}\n--- page text tail ---\n${text.slice(-3000)}`)
  })
}

async function sendPrompt(page, prompt) {
  const composer = page.getByPlaceholder('输入指令，Enter 发送，Shift+Enter 换行')
  await composer.fill(prompt)
  await page.getByRole('button', { name: '发送' }).click()
}

async function main() {
  const daemonPort = await getFreePort()
  const frontendPort = await getFreePort()
  const daemonBaseUrl = `http://127.0.0.1:${daemonPort}`
  const frontendBaseUrl = `http://127.0.0.1:${frontendPort}`
  const workRoot = path.join(repoRoot, '.tmp/e2e', `agent-cockpit-browser-smoke-${nowID()}`)
  const runsDir = path.join(workRoot, 'runs')
  const tokenFile = path.join(workRoot, 'token')
  await fs.mkdir(runsDir, { recursive: true })

  const processes = []
  let browser
  let page
  const browserLog = []

  const printLogs = () => {
    for (const proc of processes) {
      const log = proc.getLog()
      if (log) console.error(`\n--- ${proc.name} log tail ---\n${log}`)
    }
    if (browserLog.length) console.error(`\n--- browser log tail ---\n${browserLog.slice(-80).join('\n')}`)
  }

  try {
    const daemon = spawnManaged('daemon', 'go', [
      'run', './cmd/cyberagent', 'daemon',
      '--listen', `127.0.0.1:${daemonPort}`,
      '--runs-dir', runsDir,
      '--token-file', tokenFile,
    ])
    processes.push(daemon)

    const token = await waitUntil('daemon token file', async () => {
      if (!fsSync.existsSync(tokenFile)) return false
      const value = await readTrimmedFile(tokenFile)
      return value || false
    }, { timeout: 45_000 })

    await waitUntil('daemon healthz', async () => {
      const response = await fetch(`${daemonBaseUrl}/api/v1/healthz`, {
        headers: { Authorization: `Bearer ${token}` },
      }).catch(() => null)
      return response?.ok
    }, { timeout: 45_000 })

    const frontend = spawnManaged('vite', 'npm', [
      '--prefix', 'web/frontend', 'run', 'dev', '--',
      '--host', '127.0.0.1',
      '--port', String(frontendPort),
      '--strictPort',
    ], { env: { BROWSER: 'none' } })
    processes.push(frontend)

    await waitUntil('Vite frontend', async () => {
      const response = await fetch(frontendBaseUrl).catch(() => null)
      return response && response.status < 500
    }, { timeout: 45_000 })

    const launchOptions = fsSync.existsSync(CHROME_PATH)
      ? { executablePath: CHROME_PATH, headless: true }
      : { channel: 'chrome', headless: true }
    browser = await chromium.launch(launchOptions)
    const context = await browser.newContext({ viewport: { width: 1440, height: 960 } })
    await context.addInitScript(({ key, settings }) => {
      window.localStorage.setItem(key, JSON.stringify(settings))
    }, { key: SETTINGS_KEY, settings: smokeSettings(daemonBaseUrl, token) })

    page = await context.newPage()
    page.on('console', (msg) => {
      if (['error', 'warning'].includes(msg.type())) browserLog.push(`[${msg.type()}] ${msg.text()}`)
    })
    page.on('pageerror', (error) => browserLog.push(`[pageerror] ${error.message}`))

    await page.goto(`${frontendBaseUrl}/agent`, { waitUntil: 'domcontentloaded' })
    await page.getByPlaceholder('输入指令，Enter 发送，Shift+Enter 换行').waitFor({ state: 'visible', timeout: 45_000 })

    await sendPrompt(page, '检查 daemon 健康状态')
    await waitForPageText(page, /Daemon 在线|Daemon online|GET \/api\/v1\/doctor|LLM chat/i, 'daemon health result')

    await sendPrompt(page, 'write file e2e-smoke.txt content: browser-smoke')
    await waitForPageText(page, /需要授权确认/, 'authorization banner', 60_000)
    await waitForPageText(page, /workspace\.files\.write/, 'workspace write action label', 60_000)

    await page.getByRole('button', { name: /允许并继续/ }).click()
    await page.getByRole('button', { name: /允许并继续/ }).waitFor({ state: 'hidden', timeout: 75_000 })
    await waitForPageText(page, /已写入工作空间文件|Wrote workspace file|browser-smoke/, 'workspace write result', 75_000)

    const sessions = await daemonJSON(daemonBaseUrl, token, '/api/v1/agent/sessions?limit=1')
    const sessionId = sessions?.items?.[0]?.session_id
    if (!sessionId) throw new Error('unable to find smoke agent session')

    await waitUntil('workspace file content', async () => {
      const data = await daemonJSON(
        daemonBaseUrl,
        token,
        `/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/read`,
        { method: 'POST', body: JSON.stringify({ path: 'e2e-smoke.txt' }) },
      ).catch(() => null)
      return data?.content === 'browser-smoke' ? data : false
    }, { timeout: 30_000 })

    console.log(JSON.stringify({
      ok: true,
      daemon: daemonBaseUrl,
      frontend: frontendBaseUrl,
      session_id: sessionId,
      verified_file: 'e2e-smoke.txt',
    }, null, 2))
  } catch (error) {
    console.error(`agent cockpit browser smoke failed: ${error.message}`)
    if (page) {
      const url = page.url()
      const text = await page.locator('body').innerText().catch(() => '')
      console.error(`\n--- page ${url} text tail ---\n${text.slice(-3000)}`)
    }
    printLogs()
    process.exitCode = 1
  } finally {
    if (browser) await browser.close().catch(() => {})
    for (const proc of processes.reverse()) proc.stop()
    await sleep(500)
  }
}

main().catch((error) => {
  console.error(error)
  process.exitCode = 1
})
