package dispute

import (
	"testing"

	"task184-corpadjudge/internal/model"
)

func TestSummarizeRanksLabelsAndFindsMajority(t *testing.T) {
	sum := Summarize([]model.MatrixEntry{
		{Label: "N", VoteCount: 1, Annotators: []string{"ann-b"}},
		{Label: "V", VoteCount: 3, Annotators: []string{"ann-a", "ann-c", "ann-d"}},
	})
	if sum.TotalVotes != 4 || sum.LeadingLabel != "V" || !sum.Majority || sum.Threshold != 3 {
		t.Fatalf("summary=%+v", sum)
	}
	label, ok := DecideMajorityLabel(sum)
	if !ok || label != "V" {
		t.Fatalf("majority=%q ok=%v", label, ok)
	}
}
