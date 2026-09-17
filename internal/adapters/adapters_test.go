package adapters

import "testing"

func withFakeLookPath(t *testing.T, found map[string]string) {
	t.Helper()
	orig := lookPath
	lookPath = func(name string) (string, error) {
		if p, ok := found[name]; ok {
			return p, nil
		}
		return "", ErrAdapterUnavailable
	}
	t.Cleanup(func() { lookPath = orig })
}

func TestRegistryExposesBothAdapters(t *testing.T) {
	reg := Registry()
	if _, ok := reg["claude-code"]; !ok {
		t.Fatal("claude-code debe estar registrado")
	}
	if _, ok := reg["agy"]; !ok {
		t.Fatal("agy debe estar registrado")
	}
	if _, ok := reg["codex"]; !ok {
		t.Fatal("codex debe estar registrado")
	}
	if _, ok := reg["opencode"]; !ok {
		t.Fatal("opencode debe estar registrado")
	}
}
