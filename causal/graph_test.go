package causal

import (
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/benoitpetit/mira/types"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
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

// beginTxHelper starts a transaction and fails the test if it cannot
func beginTxHelper(t *testing.T, db *sql.DB) *sql.Tx {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	return tx
}

// commitTxHelper commits a transaction and fails the test if it cannot
func commitTxHelper(t *testing.T, tx *sql.Tx) {
	t.Helper()
	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}
}

// beginTxHelperBench starts a transaction for benchmarks
func beginTxHelperBench(b *testing.B, db *sql.DB) *sql.Tx {
	tx, err := db.Begin()
	if err != nil {
		b.Fatalf("Failed to begin transaction: %v", err)
	}
	return tx
}

// commitTxHelperBench commits a transaction for benchmarks
func commitTxHelperBench(b *testing.B, tx *sql.Tx) {
	if err := tx.Commit(); err != nil {
		b.Fatalf("Failed to commit transaction: %v", err)
	}
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

	tx := beginTxHelper(t, graph.db)
	if err := graph.AddNodeTx(tx, node); err != nil {
		tx.Rollback()
		t.Fatalf("Failed to add node: %v", err)
	}
	commitTxHelper(t, tx)

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

	tx := beginTxHelper(t, graph.db)
	graph.AddNodeTx(tx, node1)
	graph.AddNodeTx(tx, node2)
	commitTxHelper(t, tx)

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
	commitTxHelper(t, tx)

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

	tx := beginTxHelper(t, graph.db)
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
	commitTxHelper(t, tx)

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

	tx := beginTxHelper(t, graph.db)
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
	commitTxHelper(t, tx)

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

	tx := beginTxHelper(t, graph.db)
	graph.AddNodeTx(tx, parent)
	graph.AddNodeTx(tx, child)
	graph.AddEdgeTx(tx, &types.CausalEdge{
		FromID:     parent.ID,
		ToID:       child.ID,
		Relation:   types.RelBecause,
		DetectedAt: time.Now(),
	})
	commitTxHelper(t, tx)

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

func TestCreateCausalNodeFromFingerprint_SubjectFallback(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	fp := &types.Fingerprint{
		ID: uuid.New(),
		Data: types.FingerprintData{
			Decision: "", // empty decision
			Subject:  []string{"database migration"},
		},
		ExtractedAt: time.Now(),
	}

	node := graph.CreateCausalNodeFromFingerprint(fp, "data", nil)

	if node.Summary != "database migration" {
		t.Errorf("Expected subject fallback 'database migration', got '%s'", node.Summary)
	}
}

func TestCreateCausalNodeFromFingerprint_IDFallback(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	fp := &types.Fingerprint{
		ID: uuid.New(),
		Data: types.FingerprintData{
			Decision: "",
			Subject:  []string{},
		},
		ExtractedAt: time.Now(),
	}

	node := graph.CreateCausalNodeFromFingerprint(fp, "misc", nil)

	expected := "Memory " + fp.ID.String()[:8]
	if node.Summary != expected {
		t.Errorf("Expected ID fallback '%s', got '%s'", expected, node.Summary)
	}
}

func TestCreateCausalNodeFromFingerprint_LongSummaryTruncation(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	longDecision := strings.Repeat("x", 300)
	fp := &types.Fingerprint{
		ID: uuid.New(),
		Data: types.FingerprintData{
			Decision: longDecision,
		},
		ExtractedAt: time.Now(),
	}

	node := graph.CreateCausalNodeFromFingerprint(fp, "test", nil)

	if len(node.Summary) != 200 {
		t.Errorf("Expected truncated summary of 200 chars, got %d", len(node.Summary))
	}
}

func TestCreateCausalNodeFromFingerprint_WithRoom(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	room := "auth"
	fp := &types.Fingerprint{
		ID: uuid.New(),
		Data: types.FingerprintData{
			Decision: "Use JWT",
		},
		Type:        types.TypeDecision,
		ExtractedAt: time.Now(),
	}

	node := graph.CreateCausalNodeFromFingerprint(fp, "backend", &room)

	if node.Room == nil || *node.Room != "auth" {
		t.Errorf("Expected room 'auth', got %v", node.Room)
	}
	if node.Type != string(types.TypeDecision) {
		t.Errorf("Expected type '%s', got '%s'", types.TypeDecision, node.Type)
	}
}

func TestGetChildren(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	parent := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "decision",
		Summary:   "Parent decision",
		Timestamp: time.Now(),
		Wing:      "test",
	}
	child1 := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "action",
		Summary:   "Child action 1",
		Timestamp: time.Now().Add(time.Hour),
		Wing:      "test",
	}
	child2 := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "action",
		Summary:   "Child action 2",
		Timestamp: time.Now().Add(2 * time.Hour),
		Wing:      "test",
	}

	tx := beginTxHelper(t, graph.db)
	graph.AddNodeTx(tx, parent)
	graph.AddNodeTx(tx, child1)
	graph.AddNodeTx(tx, child2)
	graph.AddEdgeTx(tx, &types.CausalEdge{
		FromID: parent.ID, ToID: child1.ID,
		Relation: types.RelTriggered, Weight: 0.9, DetectedAt: time.Now(),
	})
	graph.AddEdgeTx(tx, &types.CausalEdge{
		FromID: parent.ID, ToID: child2.ID,
		Relation: types.RelBecause, Weight: 0.7, DetectedAt: time.Now(),
	})
	commitTxHelper(t, tx)

	children, err := graph.GetChildren(parent.ID, types.RelTriggered, types.RelBecause)
	if err != nil {
		t.Fatalf("GetChildren() error: %v", err)
	}
	if len(children) != 2 {
		t.Errorf("Expected 2 children, got %d", len(children))
	}
}

func TestGetChildren_NoRelations(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	children, err := graph.GetChildren(uuid.New())
	if err != nil {
		t.Fatalf("GetChildren() error: %v", err)
	}
	if children != nil {
		t.Errorf("Expected nil children with no relations, got %v", children)
	}
}

func TestGetParents_NoRelations(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	parents, err := graph.GetParents(uuid.New())
	if err != nil {
		t.Fatalf("GetParents() error: %v", err)
	}
	if parents != nil {
		t.Errorf("Expected nil parents with no relations, got %v", parents)
	}
}

func TestGetNode_NotFound(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	_, err := graph.GetNode(uuid.New())
	if err == nil {
		t.Fatal("Expected error for non-existent node")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestGetNode_WithRoom(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	room := "auth"
	node := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "decision",
		Summary:   "Use OAuth2",
		Timestamp: time.Now(),
		Wing:      "backend",
		Room:      &room,
	}

	tx := beginTxHelper(t, graph.db)
	graph.AddNodeTx(tx, node)
	commitTxHelper(t, tx)

	retrieved, err := graph.GetNode(node.ID)
	if err != nil {
		t.Fatalf("GetNode() error: %v", err)
	}
	if retrieved.Room == nil || *retrieved.Room != "auth" {
		t.Errorf("Expected room 'auth', got %v", retrieved.Room)
	}
}

func TestHasEdge_NonExistent(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	if graph.HasEdge(uuid.New(), uuid.New()) {
		t.Error("Expected false for non-existent edge")
	}
}

func TestGetChain_DepthLimit(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	// Create a chain of 10 nodes
	nodes := make([]*types.CausalNode, 10)
	tx := beginTxHelper(t, graph.db)
	for i := 0; i < 10; i++ {
		nodes[i] = &types.CausalNode{
			ID:        uuid.New(),
			Type:      "decision",
			Summary:   "Node " + strings.Repeat("x", i),
			Timestamp: time.Now().Add(time.Duration(i) * time.Hour),
			Wing:      "test",
		}
		graph.AddNodeTx(tx, nodes[i])
	}
	for i := 0; i < 9; i++ {
		graph.AddEdgeTx(tx, &types.CausalEdge{
			FromID: nodes[i].ID, ToID: nodes[i+1].ID,
			Relation: types.RelTriggered, DetectedAt: time.Now(),
		})
	}
	commitTxHelper(t, tx)

	// Get chain with depth limit of 2 from the last node
	chain, err := graph.GetChain(nodes[9].ID, 2)
	if err != nil {
		t.Fatalf("GetChain() error: %v", err)
	}

	// Should be limited (node9 + node8 + node7 = 3 nodes at most)
	if len(chain) > 3 {
		t.Errorf("Expected at most 3 nodes with depth limit 2, got %d", len(chain))
	}
}

func TestGetConsequences_Empty(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	node := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "fact",
		Summary:   "Isolated node",
		Timestamp: time.Now(),
		Wing:      "test",
	}
	tx := beginTxHelper(t, graph.db)
	graph.AddNodeTx(tx, node)
	commitTxHelper(t, tx)

	consequences, err := graph.GetConsequences(node.ID, 5)
	if err != nil {
		t.Fatalf("GetConsequences() error: %v", err)
	}
	if len(consequences) != 0 {
		t.Errorf("Expected 0 consequences for isolated node, got %d", len(consequences))
	}
}

func TestAddNodeTx_Duplicate(t *testing.T) {
	graph, _, cleanup := setupTestGraph(t)
	defer cleanup()

	node := &types.CausalNode{
		ID:        uuid.New(),
		Type:      "decision",
		Summary:   "Dup node",
		Timestamp: time.Now(),
		Wing:      "test",
	}

	tx := beginTxHelper(t, graph.db)
	if err := graph.AddNodeTx(tx, node); err != nil {
		tx.Rollback()
		t.Fatalf("First AddNodeTx() error: %v", err)
	}
	// INSERT OR IGNORE — second insert should not error
	if err := graph.AddNodeTx(tx, node); err != nil {
		tx.Rollback()
		t.Fatalf("Duplicate AddNodeTx() should not error: %v", err)
	}
	commitTxHelper(t, tx)
}

// --- GetRecentForCausalDetection (requires verbatim + fingerprint tables) ---

func setupTestGraphWithFullSchema(t *testing.T) (*Graph, *sql.DB, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "mira-causal-full-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := tmpDir + "/test.db"
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to open db: %v", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS verbatim (
		id BLOB PRIMARY KEY,
		content TEXT NOT NULL,
		token_count INTEGER NOT NULL,
		created_at REAL NOT NULL,
		wing TEXT NOT NULL,
		room TEXT,
		metadata TEXT
	);
	CREATE TABLE IF NOT EXISTS fingerprints (
		id BLOB PRIMARY KEY,
		verbatim_id BLOB NOT NULL REFERENCES verbatim(id),
		ftype TEXT NOT NULL,
		extracted_at REAL NOT NULL,
		entities TEXT,
		subjects TEXT,
		decision TEXT,
		data TEXT NOT NULL,
		fact_count INTEGER DEFAULT 0,
		token_estimate INTEGER DEFAULT 0,
		model_hash TEXT NOT NULL
	);
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

func insertVerbatimAndFingerprint(t *testing.T, db *sql.DB, wing string, fpType types.MemoryType) uuid.UUID {
	t.Helper()
	id := uuid.New()
	now := float64(time.Now().Unix())

	entities, _ := json.Marshal([]string{"test"})
	subjects, _ := json.Marshal([]string{"testing"})
	data, _ := json.Marshal(types.FingerprintData{ID: id.String(), Type: string(fpType)})

	_, err := db.Exec(`INSERT INTO verbatim (id, content, token_count, created_at, wing) VALUES (?, ?, ?, ?, ?)`,
		id[:], "Content for "+id.String()[:8], 10, now, wing)
	if err != nil {
		t.Fatalf("Insert verbatim error: %v", err)
	}

	_, err = db.Exec(`INSERT INTO fingerprints (id, verbatim_id, ftype, extracted_at, entities, subjects, data, fact_count, token_estimate, model_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id[:], id[:], string(fpType), now, entities, subjects, data, 1, 5, "test-hash")
	if err != nil {
		t.Fatalf("Insert fingerprint error: %v", err)
	}

	return id
}

func TestGetRecentForCausalDetection(t *testing.T) {
	graph, db, cleanup := setupTestGraphWithFullSchema(t)
	defer cleanup()

	// Insert 5 fingerprints in "backend" wing
	ids := make([]uuid.UUID, 5)
	for i := 0; i < 5; i++ {
		ids[i] = insertVerbatimAndFingerprint(t, db, "backend", types.TypeFact)
	}
	// Insert 2 in "frontend" wing
	insertVerbatimAndFingerprint(t, db, "frontend", types.TypeDecision)
	insertVerbatimAndFingerprint(t, db, "frontend", types.TypeDecision)

	// Get recent for backend, excluding the last one
	fps, err := graph.GetRecentForCausalDetection("backend", ids[4], 50)
	if err != nil {
		t.Fatalf("GetRecentForCausalDetection() error: %v", err)
	}

	if len(fps) != 4 {
		t.Errorf("Expected 4 fingerprints (5 backend - 1 excluded), got %d", len(fps))
	}

	// Verify excluded ID is not in results
	for _, fp := range fps {
		if fp.ID == ids[4] {
			t.Error("Excluded ID should not be in results")
		}
	}
}

func TestGetRecentForCausalDetectionTx(t *testing.T) {
	graph, db, cleanup := setupTestGraphWithFullSchema(t)
	defer cleanup()

	id1 := insertVerbatimAndFingerprint(t, db, "backend", types.TypeFact)
	id2 := insertVerbatimAndFingerprint(t, db, "backend", types.TypeDecision)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin tx error: %v", err)
	}
	defer tx.Rollback()

	fps, err := graph.GetRecentForCausalDetectionTx(tx, "backend", id2, 50)
	if err != nil {
		t.Fatalf("GetRecentForCausalDetectionTx() error: %v", err)
	}

	if len(fps) != 1 {
		t.Errorf("Expected 1 fingerprint, got %d", len(fps))
	}
	if len(fps) > 0 && fps[0].ID != id1 {
		t.Errorf("Expected id1, got %v", fps[0].ID)
	}
}

func TestGetRecentForCausalDetection_EmptyWing(t *testing.T) {
	graph, _, cleanup := setupTestGraphWithFullSchema(t)
	defer cleanup()

	fps, err := graph.GetRecentForCausalDetection("nonexistent", uuid.New(), 50)
	if err != nil {
		t.Fatalf("GetRecentForCausalDetection() error: %v", err)
	}
	if len(fps) != 0 {
		t.Errorf("Expected 0 results for empty wing, got %d", len(fps))
	}
}

func TestGetRecentForCausalDetection_Limit(t *testing.T) {
	graph, db, cleanup := setupTestGraphWithFullSchema(t)
	defer cleanup()

	for i := 0; i < 10; i++ {
		insertVerbatimAndFingerprint(t, db, "test", types.TypeFact)
	}

	fps, err := graph.GetRecentForCausalDetection("test", uuid.New(), 3)
	if err != nil {
		t.Fatalf("GetRecentForCausalDetection() error: %v", err)
	}
	if len(fps) > 3 {
		t.Errorf("Expected at most 3 results, got %d", len(fps))
	}
}

func TestGetRecentForCausalDetection_WithDecision(t *testing.T) {
	graph, db, cleanup := setupTestGraphWithFullSchema(t)
	defer cleanup()

	id := uuid.New()
	now := float64(time.Now().Unix())
	entities, _ := json.Marshal([]string{"Redis"})
	subjects, _ := json.Marshal([]string{"caching"})
	data, _ := json.Marshal(types.FingerprintData{ID: id.String(), Type: "decision", Decision: "Use Redis"})

	_, err := db.Exec(`INSERT INTO verbatim (id, content, token_count, created_at, wing) VALUES (?, ?, ?, ?, ?)`,
		id[:], "We decided to use Redis.", 10, now, "backend")
	if err != nil {
		t.Fatalf("Insert verbatim error: %v", err)
	}

	_, err = db.Exec(`INSERT INTO fingerprints (id, verbatim_id, ftype, extracted_at, entities, subjects, decision, data, fact_count, token_estimate, model_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id[:], id[:], "decision", now, entities, subjects, "Use Redis", data, 2, 8, "test-hash")
	if err != nil {
		t.Fatalf("Insert fingerprint error: %v", err)
	}

	fps, err := graph.GetRecentForCausalDetection("backend", uuid.New(), 50)
	if err != nil {
		t.Fatalf("GetRecentForCausalDetection() error: %v", err)
	}

	if len(fps) != 1 {
		t.Fatalf("Expected 1 fingerprint, got %d", len(fps))
	}
	if fps[0].Decision == nil || *fps[0].Decision != "Use Redis" {
		t.Errorf("Expected decision 'Use Redis', got %v", fps[0].Decision)
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
		tx := beginTxHelperBench(b, graph.db)
		graph.AddNodeTx(tx, node)
		commitTxHelperBench(b, tx)
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

	tx := beginTxHelperBench(b, graph.db)
	graph.AddNodeTx(tx, node1)
	graph.AddNodeTx(tx, node2)
	graph.AddEdgeTx(tx, &types.CausalEdge{
		FromID:     node1.ID,
		ToID:       node2.ID,
		Relation:   types.RelBecause,
		DetectedAt: time.Now(),
	})
	commitTxHelperBench(b, tx)

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

	tx := beginTxHelperBench(b, graph.db)
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
	commitTxHelperBench(b, tx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		graph.GetChain(nodes[4].ID, 10)
	}
}
