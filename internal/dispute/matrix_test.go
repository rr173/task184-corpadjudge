package dispute

import (
	"reflect"
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

// TestSummarizeTieBreakStableOrder 验证票数并列时的稳定排序行为：
// 并列标签按字母升序返回，且首位（leading_label）始终为字母序最小者，
// 不论输入顺序如何。
func TestSummarizeTieBreakStableOrder(t *testing.T) {
	// 两组输入标签顺序不同，但票数分布一致：B/C 并列最高(2)，A/D 并列次高(1)。
	cases := [][]model.MatrixEntry{
		{
			{Label: "C", VoteCount: 2, Annotators: []string{"a", "b"}},
			{Label: "A", VoteCount: 1, Annotators: []string{"c"}},
			{Label: "B", VoteCount: 2, Annotators: []string{"d", "e"}},
			{Label: "D", VoteCount: 1, Annotators: []string{"f"}},
		},
		{
			{Label: "B", VoteCount: 2, Annotators: []string{"d", "e"}},
			{Label: "D", VoteCount: 1, Annotators: []string{"f"}},
			{Label: "C", VoteCount: 2, Annotators: []string{"a", "b"}},
			{Label: "A", VoteCount: 1, Annotators: []string{"c"}},
		},
	}
	// 期望顺序：票数降序 → 同票数按标签字母升序。
	wantOrder := []string{"B", "C", "A", "D"}
	wantVotes := map[string]int{"B": 2, "C": 2, "A": 1, "D": 1}
	wantAnnot := map[string][]string{
		"A": {"c"}, "B": {"d", "e"}, "C": {"a", "b"}, "D": {"f"},
	}
	for i, entries := range cases {
		sum := Summarize(entries)
		if sum.LeadingLabel != "B" {
			t.Fatalf("case %d: leading=%q want B", i, sum.LeadingLabel)
		}
		if len(sum.Ranks) != len(wantOrder) {
			t.Fatalf("case %d: ranks len=%d want %d", i, len(sum.Ranks), len(wantOrder))
		}
		for j, r := range sum.Ranks {
			if r.Label != wantOrder[j] {
				t.Fatalf("case %d rank %d: label=%q want %q", i, j, r.Label, wantOrder[j])
			}
			if r.Votes != wantVotes[r.Label] {
				t.Fatalf("case %d rank %d (%s): votes=%d want %d", i, j, r.Label, r.Votes, wantVotes[r.Label])
			}
			if !reflect.DeepEqual(r.Annotators, wantAnnot[r.Label]) {
				t.Fatalf("case %d rank %d (%s): annotators=%v want %v", i, j, r.Label, r.Annotators, wantAnnot[r.Label])
			}
		}
		// 票数并列且无一过半 → Majority 应为 false。
		if sum.Majority {
			t.Fatalf("case %d: tie should not yield majority", i)
		}
		if _, ok := DecideMajorityLabel(sum); ok {
			t.Fatalf("case %d: tie should not decide majority label", i)
		}
	}
}

