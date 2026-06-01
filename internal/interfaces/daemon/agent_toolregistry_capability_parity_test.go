package daemon

import (
	"reflect"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
)

// TestToolRegistryParityWithCapabilityMetadata pins the per-tool capability
// metadata (IntentTags / CanDo / CannotDo / Examples) so the daemon can
// replace agentToolCapabilityMetadataByName() with the registry-derived view
// without behaviour change. Both languages are checked because the daemon
// localises Examples lazily via agentText.
func TestToolRegistryParityWithCapabilityMetadata(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		legacy := agentToolCapabilityMetadataByName(lang)
		modern := toolregistry.NewWithBuiltins().CapabilityMetadataByName(lang)
		for name, got := range legacy {
			want, ok := modern[name]
			if !ok {
				t.Fatalf("[%s] registry is missing capability metadata for %q", lang, name)
				continue
			}
			if !reflect.DeepEqual(got.IntentTags, want.IntentTags) {
				t.Fatalf("[%s] %s IntentTags mismatch: legacy=%v registry=%v", lang, name, got.IntentTags, want.IntentTags)
			}
			if !reflect.DeepEqual(got.CanDo, want.CanDo) {
				t.Fatalf("[%s] %s CanDo mismatch: legacy=%v registry=%v", lang, name, got.CanDo, want.CanDo)
			}
			if !reflect.DeepEqual(got.CannotDo, want.CannotDo) {
				t.Fatalf("[%s] %s CannotDo mismatch: legacy=%v registry=%v", lang, name, got.CannotDo, want.CannotDo)
			}
			if !reflect.DeepEqual(got.Examples, want.Examples) {
				t.Fatalf("[%s] %s Examples mismatch: legacy=%v registry=%v", lang, name, got.Examples, want.Examples)
			}
		}
	}
}
