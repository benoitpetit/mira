package benchmark

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const DatasetSchemaVersion = "1"

type Dataset struct {
	SchemaVersion string     `json:"schema_version"`
	Name          string     `json:"name"`
	Version       string     `json:"version,omitempty"`
	Memories      []Memory   `json:"memories"`
	Queries       []Query    `json:"queries"`
	Judgments     []Judgment `json:"judgments"`
}

type Memory struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	SessionID string `json:"session_id,omitempty"`
	Wing      string `json:"wing,omitempty"`
	Room      string `json:"room,omitempty"`
}

type Query struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Wing   string `json:"wing"`
	Room   string `json:"room"`
	Budget int    `json:"budget"`
}

type Judgment struct {
	QueryID  string `json:"query_id"`
	MemoryID string `json:"memory_id"`
	Grade    int    `json:"grade"`
}

type DatasetManifest struct {
	SchemaVersion    string `json:"schema_version"`
	GeneratorVersion string `json:"generator_version"`
	Seed             int64  `json:"seed"`
	SHA256           string `json:"sha256"`
	MemoryCount      int    `json:"memory_count"`
	QueryCount       int    `json:"query_count"`
	JudgmentCount    int    `json:"judgment_count"`
}

func LoadDataset(r io.Reader) (Dataset, error) {
	var dataset Dataset
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&dataset); err != nil {
		return Dataset{}, fmt.Errorf("decode benchmark dataset: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return Dataset{}, fmt.Errorf("decode benchmark dataset: trailing JSON value")
		}
		return Dataset{}, fmt.Errorf("decode benchmark dataset trailing data: %w", err)
	}
	if err := ValidateDataset(dataset); err != nil {
		return Dataset{}, err
	}
	return dataset, nil
}

func ValidateDataset(d Dataset) error {
	if d.SchemaVersion != DatasetSchemaVersion {
		return fmt.Errorf("schema_version must be %q", DatasetSchemaVersion)
	}
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("dataset name is required")
	}
	if len(d.Memories) == 0 {
		return fmt.Errorf("dataset must contain at least one memory")
	}
	if len(d.Queries) == 0 {
		return fmt.Errorf("dataset must contain at least one query")
	}
	memoryIDs := make(map[string]struct{}, len(d.Memories))
	for i, m := range d.Memories {
		if strings.TrimSpace(m.ID) == "" {
			return fmt.Errorf("memory[%d].id is required", i)
		}
		if _, exists := memoryIDs[m.ID]; exists {
			return fmt.Errorf("duplicate memory id %q", m.ID)
		}
		memoryIDs[m.ID] = struct{}{}
		if strings.TrimSpace(m.Content) == "" {
			return fmt.Errorf("memory %q content is required", m.ID)
		}
	}
	queryIDs := make(map[string]struct{}, len(d.Queries))
	for i, q := range d.Queries {
		if strings.TrimSpace(q.ID) == "" {
			return fmt.Errorf("query[%d].id is required", i)
		}
		if _, exists := queryIDs[q.ID]; exists {
			return fmt.Errorf("duplicate query id %q", q.ID)
		}
		queryIDs[q.ID] = struct{}{}
		if strings.TrimSpace(q.Text) == "" {
			return fmt.Errorf("query %q text is required", q.ID)
		}
		if q.Budget < 0 {
			return fmt.Errorf("query %q budget cannot be negative", q.ID)
		}
	}
	judged := make(map[string]bool, len(d.Queries))
	for i, j := range d.Judgments {
		if _, exists := queryIDs[j.QueryID]; !exists {
			return fmt.Errorf("judgment[%d] references unknown query %q", i, j.QueryID)
		}
		if _, exists := memoryIDs[j.MemoryID]; !exists {
			return fmt.Errorf("judgment[%d] references unknown memory %q", i, j.MemoryID)
		}
		if j.Grade < 0 || j.Grade > 2 {
			return fmt.Errorf("judgment[%d] grade must be between 0 and 2", i)
		}
		key := j.QueryID + "\x00" + j.MemoryID
		if judged[key] {
			return fmt.Errorf("duplicate judgment for query %q and memory %q", j.QueryID, j.MemoryID)
		}
		judged[key] = true
		judged[j.QueryID] = true
	}
	for _, q := range d.Queries {
		if !judged[q.ID] {
			return fmt.Errorf("query %q has no judgments", q.ID)
		}
	}
	return nil
}

func CanonicalDatasetJSON(d Dataset) ([]byte, error) {
	if err := ValidateDataset(d); err != nil {
		return nil, err
	}
	return json.Marshal(d)
}

func BuildManifest(d Dataset, seed int64, generatorVersion string) (DatasetManifest, error) {
	canonical, err := CanonicalDatasetJSON(d)
	if err != nil {
		return DatasetManifest{}, err
	}
	if strings.TrimSpace(generatorVersion) == "" {
		return DatasetManifest{}, fmt.Errorf("generator version is required")
	}
	sum := sha256.Sum256(canonical)
	return DatasetManifest{SchemaVersion: d.SchemaVersion, GeneratorVersion: generatorVersion, Seed: seed, SHA256: hex.EncodeToString(sum[:]), MemoryCount: len(d.Memories), QueryCount: len(d.Queries), JudgmentCount: len(d.Judgments)}, nil
}

func WriteDataset(w io.Writer, d Dataset) error {
	if err := ValidateDataset(d); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(d); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}
