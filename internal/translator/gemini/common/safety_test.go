package common

import (
	"bytes"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/featureflags"
	"github.com/tidwall/gjson"
)

func TestAttachDefaultSafetySettingsHonorsFeatureFlag(t *testing.T) {
	previous := featureflags.Current()
	t.Cleanup(func() {
		featureflags.Apply(previous)
	})

	input := []byte(`{"contents":[]}`)

	disabled := featureflags.Defaults()
	disabled.Gemini.AttachDefaultSafetySettings = false
	featureflags.Apply(disabled)

	got := AttachDefaultSafetySettings(input, "safetySettings")
	if !bytes.Equal(got, input) {
		t.Fatalf("expected disabled flag to leave payload unchanged, got %s", string(got))
	}

	featureflags.Apply(featureflags.Defaults())
	got = AttachDefaultSafetySettings(input, "safetySettings")
	if !gjson.GetBytes(got, "safetySettings").Exists() {
		t.Fatalf("expected default safety settings to be attached, got %s", string(got))
	}
}

func TestAttachDefaultSafetySettingsPreservesExistingSettings(t *testing.T) {
	previous := featureflags.Current()
	t.Cleanup(func() {
		featureflags.Apply(previous)
	})
	featureflags.Apply(featureflags.Defaults())

	input := []byte(`{"safetySettings":[{"category":"HARM_CATEGORY_HARASSMENT","threshold":"BLOCK_ONLY_HIGH"}]}`)
	got := AttachDefaultSafetySettings(input, "safetySettings")
	if !bytes.Equal(got, input) {
		t.Fatalf("expected existing safety settings to be preserved, got %s", string(got))
	}
}
