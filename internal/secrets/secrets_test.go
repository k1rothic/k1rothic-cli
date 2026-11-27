package secrets

import "testing"

func TestEnvVar(t *testing.T) {
	if got := EnvVar("openai-compatible"); got != "K1ROTHIC_OPENAI_COMPATIBLE_API_KEY" {
		t.Fatal(got)
	}
}

func TestEnvFallbackIsSessionOnly(t *testing.T) {
	// A provider name that can never exist in a real keychain.
	p := "k1rothic-test-provider-does-not-exist"
	t.Setenv(EnvVar(p), "  env-key  ")
	k, src, err := OS{}.Get(p)
	if err != nil || k != "env-key" || src != Env {
		t.Fatalf("got %q %v %v", k, src, err)
	}
}
