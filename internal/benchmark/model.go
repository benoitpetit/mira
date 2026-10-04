package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const DefaultEmbeddingModelID = "sentence-transformers/all-MiniLM-L6-v2"

var immutableModelRevision = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)

type EmbeddingModelLock struct {
	ModelID    string          `json:"model_id"`
	Revision   string          `json:"revision"`
	License    string          `json:"license"`
	Upstream   string          `json:"upstream"`
	Conversion string          `json:"conversion"`
	Files      []ModelArtifact `json:"files"`
}

type ModelArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

func LoadEmbeddingModelLock(r io.Reader) (EmbeddingModelLock, error) {
	var lock EmbeddingModelLock
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&lock); err != nil {
		return EmbeddingModelLock{}, err
	}
	return lock, nil
}

func VerifyEmbeddingModel(dir string, lock EmbeddingModelLock) error {
	if lock.ModelID != DefaultEmbeddingModelID {
		return fmt.Errorf("model id must be %q", DefaultEmbeddingModelID)
	}
	if !immutableModelRevision.MatchString(lock.Revision) {
		return fmt.Errorf("model revision must be an immutable 40-character commit SHA")
	}
	if strings.TrimSpace(lock.License) == "" || strings.TrimSpace(lock.Upstream) == "" {
		return fmt.Errorf("model license and upstream source are required")
	}
	if len(lock.Files) == 0 {
		return fmt.Errorf("model lock has no files")
	}
	seen := map[string]bool{}
	for _, artifact := range lock.Files {
		clean := filepath.Clean(artifact.Path)
		if clean == "." || filepath.IsAbs(clean) || clean != artifact.Path || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe model artifact path %q", artifact.Path)
		}
		if seen[clean] {
			return fmt.Errorf("duplicate model artifact %q", clean)
		}
		seen[clean] = true
		if !isSHA256(artifact.SHA256) {
			return fmt.Errorf("model artifact %q has invalid SHA-256", clean)
		}
		path := filepath.Join(dir, clean)
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("read model artifact %q: %w", clean, err)
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, file)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("hash model artifact %q: %w", clean, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close model artifact %q: %w", clean, closeErr)
		}
		if hex.EncodeToString(h.Sum(nil)) != strings.ToLower(artifact.SHA256) {
			return fmt.Errorf("model artifact %q checksum mismatch", clean)
		}
	}
	return nil
}
