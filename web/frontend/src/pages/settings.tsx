import { useState } from 'react'
import { Save } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export function SettingsPage() {
  const [saved, setSaved] = useState(false)
  const [settings, setSettings] = useState({
    anthropic_key: '',
    openai_key: '',
    ollama_url: 'http://localhost:11434',
    default_provider: 'anthropic',
    default_max_turns: '50',
  })

  const update = (field: string, value: string) => {
    setSettings((prev) => ({ ...prev, [field]: value }))
    setSaved(false)
  }

  const handleSave = () => {
    localStorage.setItem('cyberagent_settings', JSON.stringify(settings))
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  useState(() => {
    const stored = localStorage.getItem('cyberagent_settings')
    if (stored) {
      try {
        setSettings(JSON.parse(stored))
      } catch {
        // ignore
      }
    }
  })

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      <h2 className="text-3xl font-bold tracking-tight">Settings</h2>

      <Card>
        <CardHeader>
          <CardTitle>API Keys</CardTitle>
          <CardDescription>Configure your LLM provider credentials (stored locally in browser)</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="anthropic_key">Anthropic API Key</Label>
            <Input
              id="anthropic_key"
              type="password"
              placeholder="sk-ant-..."
              value={settings.anthropic_key}
              onChange={(e) => update('anthropic_key', e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="openai_key">OpenAI API Key</Label>
            <Input
              id="openai_key"
              type="password"
              placeholder="sk-..."
              value={settings.openai_key}
              onChange={(e) => update('openai_key', e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="ollama_url">Ollama Base URL</Label>
            <Input
              id="ollama_url"
              placeholder="http://localhost:11434"
              value={settings.ollama_url}
              onChange={(e) => update('ollama_url', e.target.value)}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Defaults</CardTitle>
          <CardDescription>Default values for new scans</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="default_provider">Default Provider</Label>
            <select
              id="default_provider"
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              value={settings.default_provider}
              onChange={(e) => update('default_provider', e.target.value)}
            >
              <option value="anthropic">Anthropic</option>
              <option value="openai">OpenAI</option>
              <option value="ollama">Ollama</option>
              <option value="anthropic-compatible">Anthropic Compatible</option>
              <option value="openai-compatible">OpenAI Compatible</option>
            </select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="default_max_turns">Default Max Turns</Label>
            <Input
              id="default_max_turns"
              type="number"
              value={settings.default_max_turns}
              onChange={(e) => update('default_max_turns', e.target.value)}
            />
          </div>
        </CardContent>
      </Card>

      <Button onClick={handleSave} className="w-full">
        <Save className="mr-2 h-4 w-4" />
        {saved ? 'Saved!' : 'Save Settings'}
      </Button>
    </div>
  )
}
