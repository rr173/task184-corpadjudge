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

// TestSummarizeDedupsAnnotatorsAcrossEntries 验证同一标注员跨版本/重审重复
// 出现时按标注员去重：条目内重复行不重复计票，跨条目人头只计一次。
func TestSummarizeDedupsAnnotatorsAcrossEntries(t *testing.T) {
	// V 条目里 ann-a 重复两次（跨版本重复提交同标签）→ 条目内去重，计 2 票。
	// N 条目里 ann-b 出现两次 → 条目内去重，计 1 票。
	// 全局标注员：ann-a, ann-b, ann-c 共 3 人（即便有人只在一个标签里）。
	sum := Summarize([]model.MatrixEntry{
		{Label: "V", VoteCount: 5, Annotators: []string{"ann-a", "ann-a", "ann-c"}},
		{Label: "N", VoteCount: 4, Annotators: []string{"ann-b", "ann-b"}},
	})
	// 条目内去重后 V=2, N=1，合计 3 票。
	if sum.TotalVotes != 3 {
		t.Fatalf("TotalVotes=%d want 3", sum.TotalVotes)
	}
	if sum.DistinctAnnotators != 3 {
		t.Fatalf("DistinctAnnotators=%d want 3", sum.DistinctAnnotators)
	}
	if sum.Threshold != 2 { // 3/2+1
		t.Fatalf("Threshold=%d want 2", sum.Threshold)
	}
	if sum.LeadVotes != 2 || !sum.Majority { // V 2 票过半
		t.Fatalf("LeadVotes=%d Majority=%v want 2,true", sum.LeadVotes, sum.Majority)
	}
	label, ok := DecideMajorityLabel(sum)
	if !ok || label != "V" {
		t.Fatalf("majority=%q ok=%v want V", label, ok)
	}
}
