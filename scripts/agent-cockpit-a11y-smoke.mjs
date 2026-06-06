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

function smokeSettings(daemonBaseUrl, token) {
  return {
    baseUrl: daemonBaseUrl,
    token,
    operator: 'a11y-smoke',
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
  }
}

async function assertA11ySmoke(page, label) {
  const issues = await page.evaluate(() => {
    const problems = []
    const visible = (element) => {
      if (!(element instanceof HTMLElement) || element.hidden) return false
      const style = window.getComputedStyle(element)
      if (style.display === 'none' || style.visibility === 'hidden') return false
      const rect = element.getBoundingClientRect()
      return rect.width > 0 && rect.height > 0
    }
    const labelledByText = (element) => {
      const ids = String(element.getAttribute('aria-labelledby') || '').split(/\s+/).filter(Boolean)
      return ids.map((id) => document.getElementById(id)?.textContent || '').join(' ').trim()
    }
    const accessibleName = (element) => [
      element.getAttribute('aria-label'),
      labelledByText(element),
      element.getAttribute('title'),
      element.textContent,
    ].map((value) => String(value || '').replace(/\s+/g, ' ').trim()).find(Boolean) || ''

    for (const element of document.querySelectorAll('button, [role="button"]')) {
      if (visible(element) && !accessibleName(element)) {
        problems.push(`button missing accessible name: ${element.outerHTML.slice(0, 160)}`)
      }
    }

    for (const element of document.querySelectorAll('input:not([type="hidden"]), textarea, select')) {
      if (!visible(element)) continue
      const id = element.getAttribute('id')
      const hasLabel = Boolean(
        accessibleName(element) ||
        (id && document.querySelector(`label[for="${CSS.escape(id)}"]`)) ||
        element.closest('label')
      )
      if (!hasLabel) problems.push(`form control missing label: ${element.outerHTML.slice(0, 160)}`)
    }

    for (const dialog of document.querySelectorAll('[role="dialog"]')) {
      if (!visible(dialog)) continue
      if (!accessibleName(dialog)) problems.push(`dialog missing accessible name: ${dialog.outerHTML.slice(0, 160)}`)
      if (dialog.getAttribute('aria-modal') === 'true' && !dialog.hasAttribute('tabindex')) {
        problems.push(`modal dialog should be programmatically focusable: ${dialog.outerHTML.slice(0, 160)}`)
      }
    }

    return problems
  })
  if (issues.length) throw new Error(`${label} a11y smoke issues:\n- ${issues.join('\n- ')}`)
}

async function assertFocusInsideDialog(page, label) {
  const ok = await page.evaluate(() => {
    const dialog = document.querySelector('[role="dialog"]')
    return Boolean(dialog && dialog.contains(document.activeElement))
  })
  if (!ok) throw new Error(`${label}: focus is not inside the active dialog`)
}

async function main() {
  const daemonPort = await getFreePort()
  const frontendPort = await getFreePort()
  const daemonBaseUrl = `http://127.0.0.1:${daemonPort}`
  const frontendBaseUrl = `http://127.0.0.1:${frontendPort}`
  const workRoot = path.join(repoRoot, '.tmp/e2e', `agent-cockpit-a11y-smoke-${nowID()}`)
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
    await page.getByLabel(/Agent 指令输入|Agent command input/).waitFor({ state: 'visible', timeout: 45_000 })
    await assertA11ySmoke(page, 'initial /agent')

    await page.getByRole('button', { name: /创建会话|Create session/ }).click()
    const traceButton = page.getByRole('button', { name: /打开 Trace|Open trace/ })
    await traceButton.waitFor({ state: 'visible', timeout: 45_000 })
    await waitUntil('trace button enabled', async () => traceButton.evaluate((button) => !button.disabled).catch(() => false), { timeout: 45_000 })
    await assertA11ySmoke(page, 'session /agent')

    await traceButton.click()
    const dialog = page.getByRole('dialog', { name: /Agent Trace/ })
    await dialog.waitFor({ state: 'visible', timeout: 45_000 })
    await waitUntil('trace drawer focus entered', async () => page.evaluate(() => {
      const dialog = document.querySelector('[role="dialog"]')
      return Boolean(dialog && dialog.contains(document.activeElement))
    }), { timeout: 10_000 })
    await assertFocusInsideDialog(page, 'trace drawer open')
    await assertA11ySmoke(page, 'trace drawer open')

    for (let i = 0; i < 6; i += 1) {
      await page.keyboard.press('Tab')
      await assertFocusInsideDialog(page, `trace drawer tab ${i + 1}`)
    }

    await page.keyboard.press('Escape')
    await dialog.waitFor({ state: 'hidden', timeout: 10_000 })
    await waitUntil('focus returned to trace button', async () => page.evaluate(() => {
      const active = document.activeElement
      return active instanceof HTMLElement && /打开 Trace|Open trace/.test(active.textContent || active.getAttribute('aria-label') || '')
    }), { timeout: 10_000 })

    console.log(JSON.stringify({
      ok: true,
      daemon: daemonBaseUrl,
      frontend: frontendBaseUrl,
      checks: ['button names', 'form labels', 'dialog names', 'focus trap', 'escape close', 'focus return'],
    }, null, 2))
  } catch (error) {
    console.error(`agent cockpit a11y smoke failed: ${error.message}`)
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
