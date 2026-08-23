package dispute

import (
	"testing"

	"task184-corpadjudge/internal/model"
)

func TestTiedMatrixRankingUsesStableLabelOrder(t *testing.T) {
	sum := Summarize([]model.MatrixEntry{
		{Label: "Z", VoteCount: 2, Annotators: []string{"a", "b"}},
		{Label: "A", VoteCount: 2, Annotators: []string{"c", "d"}},
	})
	if sum.LeadingLabel != "A" || len(sum.Ranks) != 2 || sum.Ranks[0].Label != "A" {
		t.Fatalf("tie ranking = %+v, want A first with deterministic ascending label order", sum)
	}
}
