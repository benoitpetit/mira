package causal

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/benoitpetit/mira/types"
)

func setupTestGraph(t *testing.T) (*Graph, *sql.DB, func()) {
	tmpDir, err := os.MkdirTemp("", "mira-causal-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := tmpDir + "/test.db"
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to open db: %v", err)
	}

	// Create tables
	schema := `
	CREATE TABLE IF NOT EXISTS causal_nodes (
		id BLOB PRIMARY KEY,
		node_type TEXT NOT NULL,
		summary TEXT NOT NULL,
		timestamp REAL NOT NULL,
		wing TEXT NOT NULL,
		room TEXT
	);
	CREATE TABLE IF NOT EXISTS causal_edges (
		from_id BLOB NOT NULL,
		to_id BLOB NOT NULL,
		relation TEXT NOT NULL,
		weight REAL DEFAULT 1.0,
		detected_at REAL NOT NULL,
		PRIMARY KEY (from_id, to_id, relation)
	);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create schema: %v", err)
	}

	graph := New(db)
	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return graph, db, cleanup
}

func TestAddNode(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	node := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "decision",
		Summary:   "Use PostgreSQL",
		Timestamp: time.Now(),
		Wing:      "backend",
	}

	tx, _ := graph.db.Begin()
	if err := graph.AddNodeTx(tx, node); err != nil {
		tx.Rollback()
		t.Fatalf("Failed to add node: %v", err)
	}
	tx.Commit()

	// Verify
	retrieved, err := graph.GetNode(node.ID)
	if err != nil {
		t.Fatalf("Failed to get node: %v", err)
	}

	if retrieved.ID != node.ID {
		t.Error("ID mismatch")
	}
	if retrieved.Summary != node.Summary {
		t.Errorf("Summary mismatch: got %v, want %v", retrieved.Summary, node.Summary)
	}
}

func TestAddEdge(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	// Create two nodes
	node1 := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "fact",
		Summary:   "Benchmark showed PostgreSQL faster",
		Timestamp: time.Now(),
		Wing:      "backend",
	}
	node2 := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "decision",
		Summary:   "Choose PostgreSQL",
		Timestamp: time.Now().Add(time.Hour),
		Wing:      "backend",
	}

	tx, _ := graph.db.Begin()
	graph.AddNodeTx(tx, node1)
	graph.AddNodeTx(tx, node2)
	tx.Commit()

	// Create edge
	edge := &types.CausalEdge{
		FromID:     node1.ID,
		ToID:       node2.ID,
		Relation:   types.RelBecause,
		Weight:     0.8,
		DetectedAt: time.Now(),
	}

	tx, _ = graph.db.Begin()
	if err := graph.AddEdgeTx(tx, edge); err != nil {
		tx.Rollback()
		t.Fatalf("Failed to add edge: %v", err)
	}
	tx.Commit()

	// Verify edge exists
	if !graph.HasEdge(node1.ID, node2.ID) {
		t.Error("Edge should exist")
	}
	if graph.HasEdge(node2.ID, node1.ID) {
		t.Error("Reverse edge should not exist")
	}
}

func TestGetChain(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	// Create chain: fact1 -> decision1 -> decision2
	fact1 := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "fact",
		Summary:   "Need database",
		Timestamp: time.Now(),
		Wing:      "backend",
	}
	decision1 := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "decision",
		Summary:   "Evaluate options",
		Timestamp: time.Now().Add(time.Hour),
		Wing:      "backend",
	}
	decision2 := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "decision",
		Summary:   "Choose PostgreSQL",
		Timestamp: time.Now().Add(2 * time.Hour),
		Wing:      "backend",
	}

	tx, _ := graph.db.Begin()
	graph.AddNodeTx(tx, fact1)
	graph.AddNodeTx(tx, decision1)
	graph.AddNodeTx(tx, decision2)

	// fact1 -> decision1
	graph.AddEdgeTx(tx, &types.CausalEdge{
		FromID:     fact1.ID,
		ToID:       decision1.ID,
		Relation:   types.RelTriggered,
		Weight:     1.0,
		DetectedAt: time.Now(),
	})
	// decision1 -> decision2
	graph.AddEdgeTx(tx, &types.CausalEdge{
		FromID:     decision1.ID,
		ToID:       decision2.ID,
		Relation:   types.RelBecause,
		Weight:     0.9,
		DetectedAt: time.Now(),
	})
	tx.Commit()

	// Get chain starting from decision2 (should find decision1 and fact1)
	chain, err := graph.GetChain(decision2.ID, 5)
	if err != nil {
		t.Fatalf("Failed to get chain: %v", err)
	}

	// Should include decision2, decision1, and fact1
	if len(chain) < 2 {
		t.Errorf("Expected chain length >= 2, got %d", len(chain))
	}
}

func TestGetConsequences(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	// Create: decision -> action1, action2
	decision := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "decision",
		Summary:   "Migrate to cloud",
		Timestamp: time.Now(),
		Wing:      "infra",
	}
	action1 := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "action",
		Summary:   "Set up AWS account",
		Timestamp: time.Now().Add(time.Hour),
		Wing:      "infra",
	}
	action2 := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "action",
		Summary:   "Configure VPC",
		Timestamp: time.Now().Add(2 * time.Hour),
		Wing:      "infra",
	}

	tx, _ := graph.db.Begin()
	graph.AddNodeTx(tx, decision)
	graph.AddNodeTx(tx, action1)
	graph.AddNodeTx(tx, action2)

	graph.AddEdgeTx(tx, &types.CausalEdge{
		FromID:     decision.ID,
		ToID:       action1.ID,
		Relation:   types.RelTriggered,
		DetectedAt: time.Now(),
	})
	graph.AddEdgeTx(tx, &types.CausalEdge{
		FromID:     decision.ID,
		ToID:       action2.ID,
		Relation:   types.RelTriggered,
		DetectedAt: time.Now(),
	})
	tx.Commit()

	consequences, err := graph.GetConsequences(decision.ID, 5)
	if err != nil {
		t.Fatalf("Failed to get consequences: %v", err)
	}

	if len(consequences) != 2 {
		t.Errorf("Expected 2 consequences, got %d", len(consequences))
	}
}

func TestGetParents(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	// Create: parent -> child
	parent := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "fact",
		Summary:   "Parent node",
		Timestamp: time.Now(),
		Wing:      "test",
	}
	child := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "decision",
		Summary:   "Child node",
		Timestamp: time.Now().Add(time.Hour),
		Wing:      "test",
	}

	tx, _ := graph.db.Begin()
	graph.AddNodeTx(tx, parent)
	graph.AddNodeTx(tx, child)
	graph.AddEdgeTx(tx, &types.CausalEdge{
		FromID:     parent.ID,
		ToID:       child.ID,
		Relation:   types.RelBecause,
		DetectedAt: time.Now(),
	})
	tx.Commit()

	parents, err := graph.GetParents(child.ID, types.RelBecause)
	if err != nil {
		t.Fatalf("Failed to get parents: %v", err)
	}

	if len(parents) != 1 {
		t.Errorf("Expected 1 parent, got %d", len(parents))
	}
	if parents[0].ID != parent.ID {
		t.Error("Wrong parent ID")
	}
}

func TestCreateCausalNodeFromFingerprint(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	fp := &types.Fingerprint{
		ID: uuid.New(),
		Data: types.FingerprintData{
			Decision: "Use Redis",
			Subject:  []string{"caching"},
		},
		ExtractedAt: time.Now(),
	}

	node := graph.CreateCausalNodeFromFingerprint(fp, "backend", nil)

	if node.ID != fp.ID {
		t.Error("Node ID should match fingerprint ID")
	}
	if node.Summary != "Use Redis" {
		t.Errorf("Expected summary 'Use Redis', got '%s'", node.Summary)
	}
	if node.Wing != "backend" {
		t.Error("Wing mismatch")
	}
}

func BenchmarkAddNode(b *testing.B) {
	graph, _, cleanup := setupTestGraph(&testing.T{})
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node := &types.CausalNode{
			ID:        uuid.New(),
			Type:      "decision",
			Summary:   "Benchmark decision",
			Timestamp: time.Now(),
			Wing:      "benchmark",
		}
		tx, _ := graph.db.Begin()
		graph.AddNodeTx(tx, node)
		tx.Commit()
	}
}

func BenchmarkHasEdge(b *testing.B) {
	graph, _, cleanup := setupTestGraph(&testing.T{})
	defer cleanup()

	// Setup nodes and edge
	node1 := &types.CausalNode{
		ID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Type:      "fact",
		Summary:   "Fact 1",
		Timestamp: time.Now(),
		Wing:      "test",
	}
	node2 := &types.CausalNode{
		ID:        uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Type:      "decision",
		Summary:   "Decision 1",
		Timestamp: time.Now(),
		Wing:      "test",
	}

	tx, _ := graph.db.Begin()
	graph.AddNodeTx(tx, node1)
	graph.AddNodeTx(tx, node2)
	graph.AddEdgeTx(tx, &types.CausalEdge{
		FromID:     node1.ID,
		ToID:       node2.ID,
		Relation:   types.RelBecause,
		DetectedAt: time.Now(),
	})
	tx.Commit()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		graph.HasEdge(node1.ID, node2.ID)
	}
}

func BenchmarkGetChain(b *testing.B) {
	graph, _, cleanup := setupTestGraph(&testing.T{})
	defer cleanup()

	// Create a chain of 5 nodes
	nodes := make([]*types.CausalNode, 5)
	for i := 0; i < 5; i++ {
		nodes[i] = &types.CausalNode{
			ID:        uuid.New(),
			Type:      "decision",
			Summary:   "Node " + string(rune('0'+i)),
			Timestamp: time.Now().Add(time.Duration(i) * time.Hour),
			Wing:      "test",
		}
	}

	tx, _ := graph.db.Begin()
	for _, n := range nodes {
		graph.AddNodeTx(tx, n)
	}
	for i := 0; i < 4; i++ {
		graph.AddEdgeTx(tx, &types.CausalEdge{
			FromID:     nodes[i].ID,
			ToID:       nodes[i+1].ID,
			Relation:   types.RelTriggered,
			DetectedAt: time.Now(),
		})
	}
	tx.Commit()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		graph.GetChain(nodes[4].ID, 10)
	}
}
