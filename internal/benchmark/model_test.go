package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyEmbeddingModel(t *testing.T) {
	dir := t.TempDir()
	data := []byte("model fixture")
	if err := os.WriteFile(filepath.Join(dir, "spago_model.bin"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	lock := EmbeddingModelLock{ModelID: DefaultEmbeddingModelID, Revision: "c21050a7ef692090620a6d037dd736908f9c7cf6", License: "Apache-2.0", Upstream: "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2", Files: []ModelArtifact{{Path: "spago_model.bin", SHA256: hex.EncodeToString(sum[:])}}}
	if err := VerifyEmbeddingModel(dir, lock); err != nil {
		t.Fatalf("valid model rejected: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*EmbeddingModelLock)
	}{
		{"wrong model", func(l *EmbeddingModelLock) { l.ModelID = "fake/model" }},
		{"mutable revision", func(l *EmbeddingModelLock) { l.Revision = "main" }},
		{"changed checksum", func(l *EmbeddingModelLock) {
			l.Files[0].SHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		}},
		{"unsafe artifact path", func(l *EmbeddingModelLock) { l.Files[0].Path = "../secret" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bad := lock
			bad.Files = append([]ModelArtifact(nil), lock.Files...)
			tt.mutate(&bad)
			if err := VerifyEmbeddingModel(dir, bad); err == nil {
				t.Fatal("invalid model lock accepted")
			}
		})
	}
}
