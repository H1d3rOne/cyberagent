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

function smokeSettings(daemonBaseUrl, token) {
  const provider = {
    id: 'visual-smoke-provider',
    name: 'Visual Smoke Provider',
    type: 'openai',
    model: 'visual-smoke-model',
    apiKey: 'visual-smoke-key',
    baseUrl: LLM_FAILURE_BASE_URL,
    instructions: '',
    maxTurns: 1,
  }
  return {
    baseUrl: daemonBaseUrl,
    token,
    operator: 'visual-smoke',
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
  const composer = page.getByLabel(/Agent 指令输入|Agent command input/)
  await composer.fill(prompt)
  await page.getByRole('button', { name: '发送' }).click()
}

async function saveScreenshot(page, outputDir, name) {
  const filePath = path.join(outputDir, name)
  await page.screenshot({ path: filePath, fullPage: false, animations: 'disabled', caret: 'hide' })
  return path.relative(repoRoot, filePath)
}

async function installVisualDeterminism(page) {
  await page.addStyleTag({
    content: `
      *, *::before, *::after {
        animation-duration: 1ms !important;
        animation-iteration-count: 1 !important;
        scroll-behavior: auto !important;
        transition-duration: 1ms !important;
      }
    `,
  })
}

async function assertVisualContract(page, label) {
  const issues = await page.evaluate(() => {
    const problems = []
    const visible = (element) => {
      if (!(element instanceof HTMLElement) || element.hidden) return false
      const style = window.getComputedStyle(element)
      if (style.display === 'none' || style.visibility === 'hidden' || Number(style.opacity) === 0) return false
      const rect = element.getBoundingClientRect()
      return rect.width > 0 && rect.height > 0
    }
    const alphaOf = (color) => {
      const match = String(color || '').match(/rgba?\(([^)]+)\)/i)
      if (!match) return 1
      const parts = match[1].split(',').map((part) => part.trim())
      if (parts.length < 4) return 1
      return Number.parseFloat(parts[3])
    }
    const hasTranslucentPaint = (style) => {
      const backgroundImage = String(style.backgroundImage || '')
      if (/rgba\([^)]*,\s*0?\./i.test(backgroundImage)) return true
      if (/linear-gradient|radial-gradient/i.test(backgroundImage)) return true
      return alphaOf(style.backgroundColor) < 0.99
    }
    const hasNonTransparentBorder = (style) => alphaOf(style.borderTopColor) > 0.05 && style.borderTopStyle !== 'none'
    const hasBlurOrFallback = (style) => {
      const backdrop = String(style.backdropFilter || style.webkitBackdropFilter || '')
      return backdrop.includes('blur') || backdrop === 'none'
    }
    const firstVisible = (selector) => Array.from(document.querySelectorAll(selector)).find(visible)

    const bodyStyle = window.getComputedStyle(document.body)
    if (!/radial-gradient|linear-gradient/i.test(bodyStyle.backgroundImage || '')) {
      problems.push('body background should keep layered gradient backdrop')
    }

    const glass = Array.from(document.querySelectorAll('.glass-surface, .glass-panel, .glass-alert, .agent-console')).filter(visible)
    if (glass.length < 3) problems.push(`expected multiple visible glass surfaces, found ${glass.length}`)

    for (const [name, selector] of [
      ['glass panel', '.glass-panel, .glass-surface-panel'],
      ['elevated surface', '.glass-surface-elevated'],
      ['semi-transparent card', '.glass-surface-subtle, .glass-alert'],
    ]) {
      const element = firstVisible(selector)
      if (!element) {
        problems.push(`${name} is not visible`)
        continue
      }
      const style = window.getComputedStyle(element)
      if (!hasTranslucentPaint(style)) problems.push(`${name} lost translucent/gradient paint`)
      if (!hasNonTransparentBorder(style)) problems.push(`${name} lost visible border`)
      if (!hasBlurOrFallback(style)) problems.push(`${name} lost backdrop blur/fallback contract`)
      if (style.boxShadow === 'none') problems.push(`${name} lost elevation shadow/inset highlight`)
    }

    return problems
  })
  if (issues.length) throw new Error(`${label} visual contract issues:\n- ${issues.join('\n- ')}`)
}

async function assertApprovalVisualContract(page) {
  const issues = await page.evaluate(() => {
    const problems = []
    const visible = (element) => {
      if (!(element instanceof HTMLElement) || element.hidden) return false
      const style = window.getComputedStyle(element)
      if (style.display === 'none' || style.visibility === 'hidden') return false
      const rect = element.getBoundingClientRect()
      return rect.width > 0 && rect.height > 0
    }
    const rgb = (color) => String(color || '').match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/)?.slice(1, 4).map(Number) || []
    const warning = Array.from(document.querySelectorAll('.glass-alert-warning')).find((element) => visible(element) && /需要授权确认|Authorization required/.test(element.textContent || ''))
    if (!warning) {
      problems.push('approval pending warning banner is not visible')
    } else {
      const style = window.getComputedStyle(warning)
      const [r, g, b] = rgb(style.borderTopColor)
      if (!(r > 200 && g > 140 && b < 140)) problems.push(`approval warning border is not amber enough: ${style.borderTopColor}`)
      if (!/rgba\(/i.test(style.backgroundColor)) problems.push(`approval warning background should remain translucent: ${style.backgroundColor}`)
    }

    const allow = Array.from(document.querySelectorAll('button')).find((element) => visible(element) && /允许并继续|Allow & continue/.test(element.textContent || ''))
    if (!allow) {
      problems.push('approval allow button is not visible')
    } else {
      const style = window.getComputedStyle(allow)
      const [r, g, b] = rgb(style.backgroundColor)
      if (!(r > 220 && g > 160 && b < 120)) problems.push(`approval allow button is not amber filled: ${style.backgroundColor}`)
    }

    const deny = Array.from(document.querySelectorAll('button')).find((element) => visible(element) && /拒绝|Deny/.test(element.textContent || ''))
    if (!deny) problems.push('destructive deny button is not visible')

    return problems
  })
  if (issues.length) throw new Error(`approval visual contract issues:\n- ${issues.join('\n- ')}`)
}

async function assertDestructiveHoverContract(page) {
  const issues = await page.evaluate(() => {
    const problems = []
    const visible = (element) => {
      if (!(element instanceof HTMLElement) || element.hidden) return false
      const style = window.getComputedStyle(element)
      if (style.display === 'none' || style.visibility === 'hidden') return false
      const rect = element.getBoundingClientRect()
      return rect.width > 0 && rect.height > 0
    }
    const rgb = (color) => String(color || '').match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/)?.slice(1, 4).map(Number) || []
    const deny = Array.from(document.querySelectorAll('button')).find((element) => visible(element) && /拒绝|Deny/.test(element.textContent || ''))
    if (!deny) return ['destructive deny button is not visible']
    const style = window.getComputedStyle(deny)
    const [bgR, bgG, bgB] = rgb(style.backgroundColor)
    const [textR, textG, textB] = rgb(style.color)
    const redTintedBackground = bgR > bgG + 40 && bgR > bgB + 40
    const redTintedText = textR >= textG && textR >= textB && textR > 200
    if (!redTintedBackground && !redTintedText) {
      problems.push(`destructive hover is not red-tinted enough: bg=${style.backgroundColor}, color=${style.color}`)
    }
    return problems
  })
  if (issues.length) throw new Error(`destructive hover visual contract issues:\n- ${issues.join('\n- ')}`)
}

async function main() {
  const daemonPort = await getFreePort()
  const frontendPort = await getFreePort()
  const daemonBaseUrl = `http://127.0.0.1:${daemonPort}`
  const frontendBaseUrl = `http://127.0.0.1:${frontendPort}`
  const workRoot = path.join(repoRoot, '.tmp/e2e', `agent-cockpit-visual-smoke-${nowID()}`)
  const runsDir = path.join(workRoot, 'runs')
  const tokenFile = path.join(workRoot, 'token')
  const outputDir = path.join(workRoot, 'screenshots')
  await fs.mkdir(runsDir, { recursive: true })
  await fs.mkdir(outputDir, { recursive: true })

  const processes = []
  let browser
  let page
  const screenshots = []
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
    const context = await browser.newContext({ viewport: { width: 1440, height: 960 }, reducedMotion: 'reduce', colorScheme: 'dark' })
    await context.addInitScript(({ key, settings }) => {
      window.localStorage.setItem(key, JSON.stringify(settings))
    }, { key: SETTINGS_KEY, settings: smokeSettings(daemonBaseUrl, token) })

    page = await context.newPage()
    page.on('console', (msg) => {
      if (['error', 'warning'].includes(msg.type())) browserLog.push(`[${msg.type()}] ${msg.text()}`)
    })
    page.on('pageerror', (error) => browserLog.push(`[pageerror] ${error.message}`))

    await page.goto(`${frontendBaseUrl}/agent`, { waitUntil: 'domcontentloaded' })
    await installVisualDeterminism(page)
    await page.getByLabel(/Agent 指令输入|Agent command input/).waitFor({ state: 'visible', timeout: 45_000 })
    await assertVisualContract(page, 'initial desktop')
    screenshots.push(await saveScreenshot(page, outputDir, '01-agent-initial-desktop.png'))

    await page.getByRole('button', { name: /创建会话|Create session/ }).click()
    const traceButton = page.getByRole('button', { name: /打开 Trace|Open trace/ })
    await traceButton.waitFor({ state: 'visible', timeout: 45_000 })
    await waitUntil('trace button enabled', async () => traceButton.evaluate((button) => !button.disabled).catch(() => false), { timeout: 45_000 })
    await assertVisualContract(page, 'session desktop')
    screenshots.push(await saveScreenshot(page, outputDir, '02-agent-session-desktop.png'))

    await traceButton.click()
    const traceDialog = page.getByRole('dialog', { name: /Agent Trace/ })
    await traceDialog.waitFor({ state: 'visible', timeout: 45_000 })
    await assertVisualContract(page, 'trace drawer desktop')
    screenshots.push(await saveScreenshot(page, outputDir, '03-trace-drawer-desktop.png'))
    await page.keyboard.press('Escape')
    await traceDialog.waitFor({ state: 'hidden', timeout: 10_000 })

    await sendPrompt(page, 'write file visual-smoke.txt content: visual-smoke')
    await waitForPageText(page, /需要授权确认/, 'authorization banner', 60_000)
    await waitForPageText(page, /workspace\.files\.write/, 'workspace write action label', 60_000)
    await assertVisualContract(page, 'approval pending desktop')
    await assertApprovalVisualContract(page)
    screenshots.push(await saveScreenshot(page, outputDir, '04-approval-pending-desktop.png'))

    const denyButton = page.getByRole('button', { name: /拒绝|Deny/ })
    await denyButton.hover()
    await assertDestructiveHoverContract(page)
    screenshots.push(await saveScreenshot(page, outputDir, '05-destructive-hover-desktop.png'))

    await page.setViewportSize({ width: 390, height: 844 })
    await waitUntil('mobile layout settled', async () => page.evaluate(() => document.documentElement.clientWidth === 390), { timeout: 10_000 })
    await assertVisualContract(page, 'approval pending mobile')
    await assertApprovalVisualContract(page)
    screenshots.push(await saveScreenshot(page, outputDir, '06-approval-pending-mobile.png'))

    console.log(JSON.stringify({
      ok: true,
      daemon: daemonBaseUrl,
      frontend: frontendBaseUrl,
      screenshots,
      checks: [
        'layered gradient backdrop',
        'glass blur/fallback + border + elevation',
        'semi-transparent cards',
        'trace drawer elevated overlay',
        'approval pending warning state',
        'destructive hover state',
        'mobile approval layout',
      ],
    }, null, 2))
  } catch (error) {
    console.error(`agent cockpit visual smoke failed: ${error.message}`)
    if (page) {
      const url = page.url()
      const text = await page.locator('body').innerText().catch(() => '')
      console.error(`\n--- page ${url} text tail ---\n${text.slice(-3000)}`)
      if (outputDir) {
        const failurePath = path.join(outputDir, 'failure.png')
        await page.screenshot({ path: failurePath, fullPage: false }).catch(() => {})
        console.error(`failure screenshot: ${path.relative(repoRoot, failurePath)}`)
      }
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
