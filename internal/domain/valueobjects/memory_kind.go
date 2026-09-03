package valueobjects

// MemoryKind describes the business role of a memory. It is deliberately
// independent from MemoryType, which describes how the extractor classified
// its content (fact, decision, preference, etc.).
type MemoryKind string

const (
	KindIdentity  MemoryKind = "identity"
	KindUser      MemoryKind = "user"
	KindProject   MemoryKind = "project"
	KindTask      MemoryKind = "task"
	KindKnowledge MemoryKind = "knowledge"
	KindHistory   MemoryKind = "history"
)

// IsValid reports whether the kind is one of MIRA's supported business roles.
func (mk MemoryKind) IsValid() bool {
	switch mk {
	case KindIdentity, KindUser, KindProject, KindTask, KindKnowledge, KindHistory:
		return true
	}
	return false
}

func (mk MemoryKind) String() string { return string(mk) }

// DefaultMemoryKindForType provides a useful role when callers do not choose
// one explicitly. Explicit kinds always take precedence.
func DefaultMemoryKindForType(memType MemoryType) MemoryKind {
	switch memType {
	case TypePreference:
		return KindUser
	case TypeDecision, TypeDebugLog:
		return KindProject
	case TypeSessionNote:
		return KindHistory
	default:
		return KindKnowledge
	}
}
