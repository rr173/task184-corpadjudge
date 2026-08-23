package dispute

import (
	"testing"

	"task184-corpadjudge/internal/model"
)

func TestMatrixSummaryCountsDistinctAnnotatorsAcrossLabels(t *testing.T) {
	sum := Summarize([]model.MatrixEntry{
		{Label: "A", VoteCount: 2, Annotators: []string{"ann-1", "ann-2"}},
		{Label: "B", VoteCount: 2, Annotators: []string{"ann-1", "ann-3"}},
	})
	if sum.DistinctAnnotators != 3 {
		t.Fatalf("distinct annotators=%d, want 3 for ann-1/ann-2/ann-3", sum.DistinctAnnotators)
	}
}
