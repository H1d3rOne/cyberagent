import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Rocket } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { api, type ProviderInfo } from '@/lib/api'

export function NewScanPage() {
  const navigate = useNavigate()
  const [providers, setProviders] = useState<ProviderInfo[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [showAdvanced, setShowAdvanced] = useState(false)

  const [form, setForm] = useState({
    target: '',
    scope: '',
    provider: 'anthropic',
    model: '',
    api_key: '',
    base_url: '',
    instructions: '',
    max_turns: 50,
    multi_agent: false,
    no_confirm: true,
    interactive: true,
  })

  useEffect(() => {
    api.getProviders().then(setProviders).catch(() => {})
  }, [])

  const update = (field: string, value: string | number | boolean) => {
    setForm((prev) => ({ ...prev, [field]: value }))
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const { id } = await api.startScan(form)
      navigate(`/scan/${id}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to start scan')
    } finally {
      setLoading(false)
    }
  }

  const selectedProvider = providers.find((p) => p.id === form.provider)

  return (
    <div className="max-w-2xl mx-auto">
      <h2 className="text-3xl font-bold tracking-tight mb-6">New Scan</h2>

      <form onSubmit={handleSubmit}>
        <Card>
          <CardHeader>
            <CardTitle>Target Configuration</CardTitle>
            <CardDescription>Configure the target and scan parameters</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="target">Target URL *</Label>
              <Input
                id="target"
                placeholder="https://example.com"
                value={form.target}
                onChange={(e) => update('target', e.target.value)}
                required
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="scope">Scope (comma-separated domains)</Label>
              <Input
                id="scope"
                placeholder="example.com, api.example.com"
                value={form.scope}
                onChange={(e) => update('scope', e.target.value)}
              />
              <p className="text-xs text-muted-foreground">Leave empty to use target domain</p>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="provider">Provider</Label>
                <select
                  id="provider"
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                  value={form.provider}
                  onChange={(e) => {
                    update('provider', e.target.value)
                    const p = providers.find((x) => x.id === e.target.value)
                    if (p?.default_model) update('model', p.default_model)
                  }}
                >
                  {providers.map((p) => (
                    <option key={p.id} value={p.id}>{p.name}</option>
                  ))}
                </select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="model">Model</Label>
                <Input
                  id="model"
                  placeholder={selectedProvider?.default_model || 'model name'}
                  value={form.model}
                  onChange={(e) => update('model', e.target.value)}
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="api_key">API Key</Label>
              <Input
                id="api_key"
                type="password"
                placeholder="sk-..."
                value={form.api_key}
                onChange={(e) => update('api_key', e.target.value)}
              />
            </div>

            {(form.provider.includes('compatible') || form.provider === 'ollama') && (
              <div className="space-y-2">
                <Label htmlFor="base_url">Base URL</Label>
                <Input
                  id="base_url"
                  placeholder="https://api.example.com/v1"
                  value={form.base_url}
                  onChange={(e) => update('base_url', e.target.value)}
                />
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="instructions">Instructions (optional)</Label>
              <Textarea
                id="instructions"
                placeholder="Focus on authentication and authorization issues..."
                value={form.instructions}
                onChange={(e) => update('instructions', e.target.value)}
              />
            </div>

            <button
              type="button"
              className="text-sm text-muted-foreground hover:text-foreground"
              onClick={() => setShowAdvanced(!showAdvanced)}
            >
              {showAdvanced ? '- Hide' : '+ Show'} Advanced Options
            </button>

            {showAdvanced && (
              <div className="space-y-4 border-t pt-4">
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="max_turns">Max Turns</Label>
                    <Input
                      id="max_turns"
                      type="number"
                      min={1}
                      max={200}
                      value={form.max_turns}
                      onChange={(e) => update('max_turns', parseInt(e.target.value) || 50)}
                    />
                  </div>
                </div>
                <div className="flex items-center gap-6">
                  <label className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={form.interactive}
                      onChange={(e) => update('interactive', e.target.checked)}
                      className="rounded"
                    />
                    Interactive mode (chat with agent)
                  </label>
                  <label className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={form.multi_agent}
                      onChange={(e) => update('multi_agent', e.target.checked)}
                      className="rounded"
                    />
                    Multi-agent mode (recon + deep analysis)
                  </label>
                  <label className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={form.no_confirm}
                      onChange={(e) => update('no_confirm', e.target.checked)}
                      className="rounded"
                    />
                    Skip confirmations
                  </label>
                </div>
              </div>
            )}

            {error && (
              <div className="text-sm text-red-500 bg-red-500/10 p-3 rounded-md">{error}</div>
            )}

            <Button type="submit" className="w-full" disabled={loading || !form.target}>
              <Rocket className="mr-2 h-4 w-4" />
              {loading ? 'Starting...' : 'Start Scan'}
            </Button>
          </CardContent>
        </Card>
      </form>
    </div>
  )
}
