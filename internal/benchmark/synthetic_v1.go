package benchmark

import "math/rand"

const SyntheticV1GeneratorVersion = "synthetic-v1"

func GenerateSyntheticV1(seed int64) (Dataset, error) {
	rng := rand.New(rand.NewSource(seed))
	d := Dataset{SchemaVersion: DatasetSchemaVersion, Name: "mira-synthetic", Version: "1.0.0"}
	memories := []Memory{
		{ID: "m-apartment", SessionID: "session-1", Wing: "personal", Room: "home", Content: "Nora moved to Cedar House, apartment 4B, on March 12."},
		{ID: "m-desk", SessionID: "session-1", Wing: "personal", Room: "home", Content: "The desk for Nora's new apartment is 140 centimeters wide."},
		{ID: "m-coffee-old", SessionID: "session-2", Wing: "personal", Room: "preferences", Content: "Nora used to prefer dark roast coffee."},
		{ID: "m-coffee-new", SessionID: "session-3", Wing: "personal", Room: "preferences", Content: "Nora now prefers light roast coffee, especially Ethiopian beans."},
		{ID: "m-trip-train", SessionID: "session-2", Wing: "personal", Room: "travel", Content: "Nora booked a train from Lyon to Turin for June 18."},
		{ID: "m-trip-hotel", SessionID: "session-3", Wing: "personal", Room: "travel", Content: "The Turin hotel reservation is for two nights, June 18 through June 20."},
		{ID: "m-project-owner", SessionID: "session-1", Wing: "work", Room: "project", Content: "The Atlas migration is owned by Ravi."},
		{ID: "m-project-date", SessionID: "session-3", Wing: "work", Room: "project", Content: "The Atlas migration review is scheduled for September 9."},
	}
	for i := 0; i < 12; i++ {
		memories = append(memories, Memory{ID: "m-distractor-" + twoDigits(i), SessionID: "session-distractors", Wing: "personal", Room: "notes", Content: distractors[rng.Intn(len(distractors))]})
	}
	d.Memories = memories
	d.Queries = []Query{
		{ID: "q-address", Text: "Where does Nora live now?", Wing: "personal", Room: "home", Budget: 256},
		{ID: "q-desk-paraphrase", Text: "How wide is the work table in the new flat?", Wing: "personal", Room: "home", Budget: 256},
		{ID: "q-coffee-update", Text: "Which coffee roast does Nora currently like?", Wing: "personal", Room: "preferences", Budget: 256},
		{ID: "q-turin-dates", Text: "When is Nora's trip to Turin and how long is the hotel stay?", Wing: "personal", Room: "travel", Budget: 384},
		{ID: "q-atlas-owner", Text: "Who is responsible for the Atlas project migration?", Wing: "work", Room: "project", Budget: 256},
		{ID: "q-atlas-review", Text: "When will the Atlas migration be reviewed?", Wing: "work", Room: "project", Budget: 256},
	}
	d.Judgments = []Judgment{
		{QueryID: "q-address", MemoryID: "m-apartment", Grade: 2},
		{QueryID: "q-desk-paraphrase", MemoryID: "m-desk", Grade: 2},
		{QueryID: "q-coffee-update", MemoryID: "m-coffee-new", Grade: 2}, {QueryID: "q-coffee-update", MemoryID: "m-coffee-old", Grade: 0},
		{QueryID: "q-turin-dates", MemoryID: "m-trip-train", Grade: 1}, {QueryID: "q-turin-dates", MemoryID: "m-trip-hotel", Grade: 2},
		{QueryID: "q-atlas-owner", MemoryID: "m-project-owner", Grade: 2},
		{QueryID: "q-atlas-review", MemoryID: "m-project-date", Grade: 2},
	}
	return d, ValidateDataset(d)
}

var distractors = []string{
	"The community garden on Cedar Street opens at eight in the morning.",
	"A blue notebook was left beside the meeting room window.",
	"The weekend market sells pears, bread, and fresh herbs.",
	"The library closes early on the first Wednesday of each month.",
	"A parcel for the design team arrived at the north entrance.",
	"The old bridge is closed while crews repaint its railings.",
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
