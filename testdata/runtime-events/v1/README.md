# Runtime event fixtures v1

These fixtures pin the additive `runtime.event.v1` UI-facing contract for backend runtime events.
They do not replace the legacy `runtime.Event` SSE or SQLite payload.

Structure:

- `catalog.json`: sorted runtime event catalog, including emitted and reserved event names.
- `envelopes/`: one normalized `EventEnvelopeV1` per representative event.
- `sequences/`: ordered envelope arrays for reducer/replay contract checks.
- `legacy/`: fallback behavior for unknown or bridged legacy event types.

Regenerate the fixture JSON from runtime tests with:

```sh
UPDATE_RUNTIME_EVENT_FIXTURES=1 go test ./internal/agent/runtime
```
