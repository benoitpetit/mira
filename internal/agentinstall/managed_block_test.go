package agentinstall

import "testing"

func TestMergeManagedBlockInsertsAndReplacesOnlyMiraBlock(t *testing.T) {
	const body = "Use MIRA for durable project continuity."
	input := "# Project rules\nKeep this text.\n"
	merged, err := MergeManagedBlock(input, body)
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	if merged == input || !containsManagedBody(merged, body) || !containsManagedBody(merged, managedBegin) || !containsManagedBody(merged, managedEnd) {
		t.Fatalf("managed block missing from %q", merged)
	}

	replaced, err := MergeManagedBlock("prefix\n"+merged+"\nsuffix\n", "Updated instructions.")
	if err != nil {
		t.Fatalf("replace failed: %v", err)
	}
	if !containsManagedBody(replaced, "prefix\n") || !containsManagedBody(replaced, "\nsuffix\n") || containsManagedBody(replaced, body) || !containsManagedBody(replaced, "Updated instructions.") {
		t.Fatalf("replacement damaged surrounding content: %q", replaced)
	}
}

func TestManagedBlockRejectsAmbiguousMarkersAndRemovesOnlyManagedContent(t *testing.T) {
	if _, err := MergeManagedBlock(managedBegin+"\n"+managedBegin+"\n"+managedEnd, "body"); err == nil {
		t.Fatal("duplicate managed blocks were accepted")
	}
	if _, err := RemoveManagedBlock(managedBegin + "\nopen"); err == nil {
		t.Fatal("unclosed managed block was accepted")
	}
	cleaned, err := RemoveManagedBlock("before\n" + managedBegin + "\nbody\n" + managedEnd + "\nafter\n")
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if cleaned != "before\nafter\n" {
		t.Fatalf("unexpected removal result %q", cleaned)
	}
}

func containsManagedBody(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
