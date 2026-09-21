package extraction

import "testing"

func TestEstimateTokenCountIsLocalAndDeterministic(t *testing.T) {
	const input = "MIRA keeps memory local."
	first := estimateTokenCount(input)
	second := estimateTokenCount(input)
	if first <= 0 {
		t.Fatal("expected a positive token estimate")
	}
	if first != second {
		t.Fatalf("estimate changed between calls: %d != %d", first, second)
	}
	if estimateTokenCount(" ") != 0 {
		t.Fatal("whitespace should have zero tokens")
	}
}
