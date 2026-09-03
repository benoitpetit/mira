package extraction

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidModelConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if validModelConfig(path) {
		t.Fatal("missing config must not be valid")
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if validModelConfig(path) {
		t.Fatal("empty config must not be valid")
	}
	if err := os.WriteFile(path, []byte(`{"hidden_size":384}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !validModelConfig(path) {
		t.Fatal("valid JSON model config must be accepted")
	}
}
