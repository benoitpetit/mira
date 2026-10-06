package agentinstall

import (
	"strings"
	"testing"
)

func TestNativeSkillListsOnlyMCPTools(t *testing.T) {
	skill := NativeSkill("api")
	for _, tool := range []string{"mira_recall", "mira_store", "mira_load", "mira_consolidate"} {
		if !strings.Contains(skill, tool) {
			t.Errorf("skill missing %s", tool)
		}
	}
	for _, unavailable := range []string{"soul_capture", "soul_recall"} {
		if strings.Contains(skill, unavailable) {
			t.Errorf("skill advertises unavailable tool %s", unavailable)
		}
	}
}

func TestMergeManagedSkillKeepsFrontMatterAndUserContent(t *testing.T) {
	merged, err := MergeManagedSkill("---\nname: mira\n---\nUser note.\n", NativeSkill("api"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(merged, "---\nname: mira\n") || !strings.Contains(merged, "User note.") || !strings.Contains(merged, "MIRA:BEGIN") {
		t.Fatalf("unexpected skill content: %q", merged)
	}
}
