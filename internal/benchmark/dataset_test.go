package benchmark

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDatasetAndValidateDataset(t *testing.T) {
	valid := `{"schema_version":"1","name":"test","memories":[{"id":"m1","content":"a note"}],"queries":[{"id":"q1","text":"what note?","wing":"default","room":"default","budget":100}],"judgments":[{"query_id":"q1","memory_id":"m1","grade":2}]}`
	dataset, err := LoadDataset(strings.NewReader(valid))
	if err != nil {
		t.Fatalf("LoadDataset() error = %v", err)
	}
	if err := ValidateDataset(dataset); err != nil {
		t.Fatalf("ValidateDataset() error = %v", err)
	}

	malformed := []struct {
		name string
		edit func(*Dataset)
	}{
		{"duplicate memory IDs", func(d *Dataset) { d.Memories = append(d.Memories, d.Memories[0]) }},
		{"duplicate query IDs", func(d *Dataset) { d.Queries = append(d.Queries, d.Queries[0]) }},
		{"missing query judgments", func(d *Dataset) { d.Judgments = nil }},
		{"unknown memory", func(d *Dataset) { d.Judgments[0].MemoryID = "absent" }},
		{"invalid grade", func(d *Dataset) { d.Judgments[0].Grade = 3 }},
		{"empty memory content", func(d *Dataset) { d.Memories[0].Content = " " }},
		{"empty query", func(d *Dataset) { d.Queries[0].Text = "" }},
		{"empty corpus", func(d *Dataset) { d.Memories = nil }},
	}
	for _, tt := range malformed {
		t.Run(tt.name, func(t *testing.T) {
			d, err := LoadDataset(strings.NewReader(valid))
			if err != nil {
				t.Fatal(err)
			}
			tt.edit(&d)
			if err := ValidateDataset(d); err == nil {
				t.Fatal("ValidateDataset() accepted invalid dataset")
			}
		})
	}
	if _, err := LoadDataset(strings.NewReader(`{"schema_version":"1"} trailing`)); err == nil {
		t.Fatal("LoadDataset() accepted trailing data")
	}
}

func TestDatasetManifestAndSyntheticGeneration(t *testing.T) {
	first, err := GenerateSyntheticV1(42)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateSyntheticV1(42)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if !bytes.Equal(a, b) {
		t.Fatal("same seed produced different datasets")
	}
	other, err := GenerateSyntheticV1(43)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := json.Marshal(other)
	if bytes.Equal(a, c) {
		t.Fatal("different seed produced identical datasets")
	}
	manifest, err := BuildManifest(first, 42, "synthetic-v1")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SHA256 == "" || manifest.MemoryCount != len(first.Memories) || manifest.QueryCount != len(first.Queries) || manifest.JudgmentCount != len(first.Judgments) {
		t.Fatalf("manifest does not describe dataset: %+v", manifest)
	}
	canonical, err := CanonicalDatasetJSON(first)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	if manifest.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("manifest hash = %s, want canonical sha256", manifest.SHA256)
	}
	const expectedSeed42SHA256 = "2202fb02f9154531a811e227773bba5df328b7203700adbb9d7688f795c2d396"
	if manifest.SHA256 != expectedSeed42SHA256 {
		t.Fatalf("seed-42 canonical hash = %s, want %s", manifest.SHA256, expectedSeed42SHA256)
	}
	fixturePath := filepath.Join("..", "..", "benchmarks", "datasets", "synthetic-v1.json")
	fixtureBytes, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := LoadDataset(bytes.NewReader(fixtureBytes))
	if err != nil {
		t.Fatal(err)
	}
	fixtureCanonical, err := CanonicalDatasetJSON(fixture)
	if err != nil {
		t.Fatal(err)
	}
	fixtureSum := sha256.Sum256(fixtureCanonical)
	if hex.EncodeToString(fixtureSum[:]) != expectedSeed42SHA256 {
		t.Fatal("checked-in dataset fixture differs from canonical seed-42 generation")
	}
}
