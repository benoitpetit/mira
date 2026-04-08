package causal

import (
	"container/list"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"mira/types"
)

// Graph manages causal graph
type Graph struct {
	db *sql.DB
}

// New creates a new causal graph
func New(db *sql.DB) *Graph {
	return &Graph{db: db}
}

// queueItem for BFS
type queueItem struct {
	id    uuid.UUID
	depth int
}

// GetChain traces back causal chain (parents)
func (g *Graph) GetChain(nodeID uuid.UUID, maxDepth int) ([]*types.CausalNode, error) {
	visited := make(map[uuid.UUID]bool)
	var chain []*types.CausalNode

	queue := list.New()
	queue.PushBack(&queueItem{id: nodeID, depth: 0})

	for queue.Len() > 0 {
		item := queue.Remove(queue.Front()).(*queueItem)

		if visited[item.id] || item.depth > maxDepth {
			continue
		}
		visited[item.id] = true

		node, err := g.GetNode(item.id)
		if err != nil {
			continue
		}

		chain = append(chain, node)

		// Parents: TRIGGERED or BECAUSE relations (current node is consequence)
		parents, err := g.GetParents(item.id, types.RelTriggered, types.RelBecause)
		if err != nil {
			continue
		}

		for _, p := range parents {
			if !visited[p.ID] {
				queue.PushBack(&queueItem{id: p.ID, depth: item.depth + 1})
			}
		}
	}

	return chain, nil
}

// GetConsequences follows chain (children)
func (g *Graph) GetConsequences(nodeID uuid.UUID, maxDepth int) ([]*types.CausalNode, error) {
	visited := make(map[uuid.UUID]bool)
	var consequences []*types.CausalNode

	queue := list.New()
	queue.PushBack(&queueItem{id: nodeID, depth: 0})

	for queue.Len() > 0 {
		item := queue.Remove(queue.Front()).(*queueItem)

		if visited[item.id] || item.depth > maxDepth {
			continue
		}
		visited[item.id] = true

		node, err := g.GetNode(item.id)
		if err != nil {
			continue
		}

		if item.depth > 0 { // Don't include starting node
			consequences = append(consequences, node)
		}

		// Children: relations where node is the cause
		children, err := g.GetChildren(item.id, types.RelTriggered, types.RelBecause)
		if err != nil {
			continue
		}

		for _, c := range children {
			if !visited[c.ID] {
				queue.PushBack(&queueItem{id: c.ID, depth: item.depth + 1})
			}
		}
	}

	return consequences, nil
}

// GetNode retrieves a node by ID
func (g *Graph) GetNode(nodeID uuid.UUID) (*types.CausalNode, error) {
	row := g.db.QueryRow(
		`SELECT id, node_type, summary, timestamp, wing, room FROM causal_nodes WHERE id = ?`,
		nodeID[:],
	)

	var node types.CausalNode
	var idBytes []byte
	var room sql.NullString
	var timestamp float64

	err := row.Scan(&idBytes, &node.Type, &node.Summary, &timestamp, &node.Wing, &room)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("node not found")
		}
		return nil, err
	}

	node.ID, _ = uuid.FromBytes(idBytes)
	node.Timestamp = time.Unix(int64(timestamp), 0)
	if room.Valid {
		node.Room = &room.String
	}

	return &node, nil
}

// GetParents with relation filtering
func (g *Graph) GetParents(nodeID uuid.UUID, relations ...types.RelationType) ([]*types.CausalNode, error) {
	if len(relations) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(relations))
	args := make([]interface{}, len(relations)+1)
	args[0] = nodeID[:]

	for i, rel := range relations {
		placeholders[i] = "?"
		args[i+1] = string(rel)
	}

	query := `
		SELECT n.id, n.node_type, n.summary, n.timestamp, n.wing, n.room
		FROM causal_nodes n
		JOIN causal_edges e ON n.id = e.from_id
		WHERE e.to_id = ? AND e.relation IN (` + strings.Join(placeholders, ",") + `)
		ORDER BY e.weight DESC, n.timestamp DESC
	`

	rows, err := g.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*types.CausalNode
	for rows.Next() {
		var n types.CausalNode
		var idBytes []byte
		var room sql.NullString
		var timestamp float64

		err := rows.Scan(&idBytes, &n.Type, &n.Summary, &timestamp, &n.Wing, &room)
		if err != nil {
			continue
		}
		n.ID, _ = uuid.FromBytes(idBytes)
		n.Timestamp = time.Unix(int64(timestamp), 0)
		if room.Valid {
			n.Room = &room.String
		}
		nodes = append(nodes, &n)
	}

	return nodes, nil
}

// GetChildren retrieves children (consequences)
func (g *Graph) GetChildren(nodeID uuid.UUID, relations ...types.RelationType) ([]*types.CausalNode, error) {
	if len(relations) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(relations))
	args := make([]interface{}, len(relations)+1)
	args[0] = nodeID[:]

	for i, rel := range relations {
		placeholders[i] = "?"
		args[i+1] = string(rel)
	}

	query := `
		SELECT n.id, n.node_type, n.summary, n.timestamp, n.wing, n.room
		FROM causal_nodes n
		JOIN causal_edges e ON n.id = e.to_id
		WHERE e.from_id = ? AND e.relation IN (` + strings.Join(placeholders, ",") + `)
		ORDER BY e.weight DESC, n.timestamp DESC
	`

	rows, err := g.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*types.CausalNode
	for rows.Next() {
		var n types.CausalNode
		var idBytes []byte
		var room sql.NullString
		var timestamp float64

		err := rows.Scan(&idBytes, &n.Type, &n.Summary, &timestamp, &n.Wing, &room)
		if err != nil {
			continue
		}
		n.ID, _ = uuid.FromBytes(idBytes)
		n.Timestamp = time.Unix(int64(timestamp), 0)
		if room.Valid {
			n.Room = &room.String
		}
		nodes = append(nodes, &n)
	}

	return nodes, nil
}

// HasEdge checks if relation exists
func (g *Graph) HasEdge(fromID, toID uuid.UUID) bool {
	var exists bool
	err := g.db.QueryRow(
		"SELECT 1 FROM causal_edges WHERE from_id = ? AND to_id = ? LIMIT 1",
		fromID[:], toID[:],
	).Scan(&exists)
	return err == nil && exists
}

// AddNodeTx adds a node in a transaction
func (g *Graph) AddNodeTx(tx *sql.Tx, node *types.CausalNode) error {
	_, err := tx.Exec(
		`INSERT OR IGNORE INTO causal_nodes (id, node_type, summary, timestamp, wing, room) 
		 VALUES (?, ?, ?, ?, ?, ?)`,
		node.ID[:], node.Type, node.Summary, float64(node.Timestamp.Unix()), node.Wing, node.Room,
	)
	return err
}

// AddEdgeTx adds an edge in a transaction
func (g *Graph) AddEdgeTx(tx *sql.Tx, edge *types.CausalEdge) error {
	_, err := tx.Exec(
		`INSERT OR IGNORE INTO causal_edges (from_id, to_id, relation, weight, detected_at) 
		 VALUES (?, ?, ?, ?, ?)`,
		edge.FromID[:], edge.ToID[:], string(edge.Relation), edge.Weight, float64(edge.DetectedAt.Unix()),
	)
	return err
}

// GetRecentForCausalDetection retrieves N latest FPs from a wing for comparison
func (g *Graph) GetRecentForCausalDetection(wing string, excludeID uuid.UUID, limit int) ([]*types.Fingerprint, error) {
	query := `
		SELECT f.id, f.verbatim_id, f.ftype, f.extracted_at, f.entities, f.subjects,
		       f.decision, f.data, f.fact_count, f.token_estimate, f.model_hash
		FROM fingerprints f
		JOIN verbatim v ON f.verbatim_id = v.id
		WHERE v.wing = ? AND f.id != ?
		ORDER BY f.extracted_at DESC
		LIMIT ?
	`

	rows, err := g.db.Query(query, wing, excludeID[:], limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fps []*types.Fingerprint
	for rows.Next() {
		var fp types.Fingerprint
		var idBytes, verbatimIDBytes []byte
		var extractedAt float64
		var entitiesJSON, subjectsJSON []byte
		var decision sql.NullString
		var dataJSON []byte

		err := rows.Scan(
			&idBytes, &verbatimIDBytes, &fp.Type, &extractedAt,
			&entitiesJSON, &subjectsJSON, &decision, &dataJSON,
			&fp.FactCount, &fp.TokenEstimate, &fp.ModelHash,
		)
		if err != nil {
			continue
		}

		fp.ID, _ = uuid.FromBytes(idBytes)
		fp.VerbatimID, _ = uuid.FromBytes(verbatimIDBytes)
		fp.ExtractedAt = time.Unix(int64(extractedAt), 0)
		if decision.Valid {
			fp.Decision = &decision.String
		}
		if len(entitiesJSON) > 0 {
			// Simple JSON parsing - in production use json.Unmarshal
		}

		fps = append(fps, &fp)
	}

	return fps, nil
}

// CreateCausalNodeFromFingerprint creates a causal node from a fingerprint
func (g *Graph) CreateCausalNodeFromFingerprint(fp *types.Fingerprint, wing string, room *string) *types.CausalNode {
	summary := fp.Data.Decision
	if summary == "" && len(fp.Data.Subject) > 0 {
		summary = fp.Data.Subject[0]
	}
	if summary == "" {
		summary = "Memory " + fp.ID.String()[:8]
	}

	if len(summary) > 200 {
		summary = summary[:200]
	}

	return &types.CausalNode{
		ID:        fp.ID,
		Type:      string(fp.Type),
		Summary:   summary,
		Timestamp: fp.ExtractedAt,
		Wing:      wing,
		Room:      room,
	}
}
