import { beforeEach, describe, expect, it } from 'vitest'
import { defaultApiAuthProfile, defaultSettings, loadSettings, saveSettings, type ConsoleSettings } from './api'

const settingsKey = 'cyberagent_console_settings'

function installLocalStorage() {
  const values = new Map<string, string>()
  const storage: Storage = {
    get length() {
      return values.size
    },
    clear() {
      values.clear()
    },
    getItem(key: string) {
      return values.get(key) ?? null
    },
    key(index: number) {
      return Array.from(values.keys())[index] ?? null
    },
    removeItem(key: string) {
      values.delete(key)
    },
    setItem(key: string, value: string) {
      values.set(key, value)
    },
  }
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: storage,
  })
  return storage
}

function settingsWithProviderSecret(apiKey: string, apiKeyConfigured: boolean): ConsoleSettings {
  const provider = {
    ...defaultSettings.aiProviders[0],
    apiKey,
    apiKeyConfigured,
  }
  return {
    ...defaultSettings,
    agentProvider: {
      ...defaultSettings.agentProvider,
      apiKey,
    },
    aiProviders: [provider],
    activeAiProviderId: provider.id,
  }
}

function settingsWithApiAuthProfileSecret(configured = false): ConsoleSettings {
  return {
    ...defaultSettings,
    apiAuthProfiles: [{
      ...defaultApiAuthProfile,
      id: 'auth-local-test',
      name: 'Auth Local Test',
      type: 'bearer',
      token: 'bearer-secret',
      password: 'password-secret',
      headerValue: 'header-secret',
      cookie: 'session=secret',
      secretConfigured: configured,
      tokenConfigured: configured,
      passwordConfigured: configured,
      headerValueConfigured: configured,
      cookieConfigured: configured,
    }],
    defaultApiAuthProfileId: 'auth-local-test',
  }
}

describe('provider secret local storage handling', () => {
  beforeEach(() => {
    installLocalStorage()
  })

  it('does not persist AI provider API keys to console settings localStorage', () => {
    saveSettings(settingsWithProviderSecret('sk-local-only', false))

    const raw = JSON.parse(localStorage.getItem(settingsKey) || '{}')
    expect(raw.agentProvider.apiKey).toBe('')
    expect(raw.aiProviders[0].apiKey).toBe('')
    expect(raw.aiProviders[0].apiKeyConfigured).toBe(false)
  })

  it('preserves server-side configured status without persisting a raw API key', () => {
    saveSettings(settingsWithProviderSecret('', true))

    const raw = JSON.parse(localStorage.getItem(settingsKey) || '{}')
    expect(raw.agentProvider.apiKey).toBe('')
    expect(raw.aiProviders[0].apiKey).toBe('')
    expect(raw.aiProviders[0].apiKeyConfigured).toBe(true)
  })

  it('sanitizes legacy local API keys on load without marking them as daemon-configured', () => {
    localStorage.setItem(settingsKey, JSON.stringify(settingsWithProviderSecret('sk-legacy-local', false)))

    const settings = loadSettings()

    expect(settings.agentProvider.apiKey).toBe('')
    expect(settings.aiProviders[0].apiKey).toBe('')
    expect(settings.aiProviders[0].apiKeyConfigured).toBe(false)
  })

  it('does not persist API auth profile secrets to console settings localStorage', () => {
    saveSettings(settingsWithApiAuthProfileSecret(false))

    const raw = JSON.parse(localStorage.getItem(settingsKey) || '{}')
    expect(raw.apiAuthProfiles[0].token).toBe('')
    expect(raw.apiAuthProfiles[0].password).toBe('')
    expect(raw.apiAuthProfiles[0].headerValue).toBe('')
    expect(raw.apiAuthProfiles[0].cookie).toBe('')
    expect(raw.apiAuthProfiles[0].secretConfigured).toBe(false)
    expect(raw.apiAuthProfiles[0].tokenConfigured).toBe(false)
    expect(raw.apiAuthProfiles[0].passwordConfigured).toBe(false)
    expect(raw.apiAuthProfiles[0].headerValueConfigured).toBe(false)
    expect(raw.apiAuthProfiles[0].cookieConfigured).toBe(false)
  })

  it('preserves server-side API auth configured status without persisting raw secrets', () => {
    const configured = settingsWithApiAuthProfileSecret(true)
    configured.apiAuthProfiles[0].token = ''
    configured.apiAuthProfiles[0].password = ''
    configured.apiAuthProfiles[0].headerValue = ''
    configured.apiAuthProfiles[0].cookie = ''
    saveSettings(configured)

    const raw = JSON.parse(localStorage.getItem(settingsKey) || '{}')
    expect(raw.apiAuthProfiles[0].token).toBe('')
    expect(raw.apiAuthProfiles[0].password).toBe('')
    expect(raw.apiAuthProfiles[0].headerValue).toBe('')
    expect(raw.apiAuthProfiles[0].cookie).toBe('')
    expect(raw.apiAuthProfiles[0].secretConfigured).toBe(true)
    expect(raw.apiAuthProfiles[0].tokenConfigured).toBe(true)
    expect(raw.apiAuthProfiles[0].passwordConfigured).toBe(true)
    expect(raw.apiAuthProfiles[0].headerValueConfigured).toBe(true)
    expect(raw.apiAuthProfiles[0].cookieConfigured).toBe(true)
  })

  it('sanitizes legacy local API auth secrets on load without marking them as daemon-configured', () => {
    localStorage.setItem(settingsKey, JSON.stringify(settingsWithApiAuthProfileSecret(false)))

    const settings = loadSettings()

    expect(settings.apiAuthProfiles[0].token).toBe('')
    expect(settings.apiAuthProfiles[0].password).toBe('')
    expect(settings.apiAuthProfiles[0].headerValue).toBe('')
    expect(settings.apiAuthProfiles[0].cookie).toBe('')
    expect(settings.apiAuthProfiles[0].secretConfigured).toBe(false)
    expect(settings.apiAuthProfiles[0].tokenConfigured).toBe(false)
    expect(settings.apiAuthProfiles[0].passwordConfigured).toBe(false)
    expect(settings.apiAuthProfiles[0].headerValueConfigured).toBe(false)
    expect(settings.apiAuthProfiles[0].cookieConfigured).toBe(false)
  })
})
