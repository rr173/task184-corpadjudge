package dispute

import (
	"sort"

	"task184-corpadjudge/internal/model"
)

// MatrixSummary 汇总分歧矩阵的关键统计，供工作台与自检使用。
type MatrixSummary struct {
	DisputeID          int64       `json:"dispute_id"`
	LabelCount         int         `json:"label_count"`
	TotalVotes         int         `json:"total_votes"`
	DistinctAnnotators int         `json:"distinct_annotators"`
	LeadingLabel       string      `json:"leading_label"` // 票数最高的标签
	LeadVotes          int         `json:"lead_votes"`
	Majority           bool        `json:"majority"`  // 是否有标签过半
	Threshold          int         `json:"threshold"` // 过半阈值（总票数/2+1）
	Ranks              []LabelRank `json:"ranks"`
}

// LabelRank 是单标签的排序结果。
type LabelRank struct {
	Label      string   `json:"label"`
	Votes      int      `json:"votes"`
	Annotators []string `json:"annotators"`
}

// Summarize 从矩阵条目计算汇总统计。
//
// 统计一律按标注员去重：同一标注员可能在跨准则版本、重审等场景下产生多条
// 标注行。这里以标注员为统计单位——每个标签内同一标注员只计一票（条目内去重），
// 跨标签也只计一次人头（DistinctAnnotators 全局去重）。TotalVotes 取各标签
// 去重票数之和，与 LeadVotes 同口径，保证多数判定阈值一致。
func Summarize(entries []model.MatrixEntry) MatrixSummary {
	sum := MatrixSummary{}
	seen := map[string]struct{}{}
	for _, e := range entries {
		sum.LabelCount++
		entryAnnotators := dedupStrings(e.Annotators) // 条目内去重，防重复行
		for _, a := range entryAnnotators {
			seen[a] = struct{}{} // 跨条目去重人头
		}
		votes := len(entryAnnotators)
		sum.TotalVotes += votes
		sum.Ranks = append(sum.Ranks, LabelRank{Label: e.Label, Votes: votes, Annotators: entryAnnotators})
	}
	sum.DistinctAnnotators = len(seen)
	sort.Slice(sum.Ranks, func(i, j int) bool { return sum.Ranks[i].Votes > sum.Ranks[j].Votes })
	if len(sum.Ranks) > 0 {
		sum.LeadingLabel = sum.Ranks[0].Label
		sum.LeadVotes = sum.Ranks[0].Votes
	}
	if sum.TotalVotes > 0 {
		sum.Threshold = sum.TotalVotes/2 + 1
		sum.Majority = sum.LeadVotes >= sum.Threshold
	}
	return sum
}

// dedupStrings 去重并保留首次出现顺序。
func dedupStrings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// DecideMajorityLabel 返回多数标签；若无一过半则返回空串与 false。
func DecideMajorityLabel(sum MatrixSummary) (string, bool) {
	if sum.Majority && sum.LeadingLabel != "" {
		return sum.LeadingLabel, true
	}
	return "", false
}

// ByLayer 按层拆分矩阵条目（用于跨层展示）。
func ByLayer(entries []model.MatrixEntry, layer model.Layer) []model.MatrixEntry {
	out := []model.MatrixEntry{}
	for _, e := range entries {
		// 矩阵条目本身不携带层信息，此处依赖调用方传入的过滤逻辑；
		// 保留此函数以维持跨层工作台的扩展点。
		out = append(out, e)
	}
	return out
}
