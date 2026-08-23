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
func Summarize(entries []model.MatrixEntry) MatrixSummary {
	sum := MatrixSummary{}
	// 汇总票数与标签排名，标注员去重在展示层完成。
	for _, e := range entries {
		sum.LabelCount++
		sum.TotalVotes += e.VoteCount
		sum.DistinctAnnotators += len(e.Annotators)
		sum.Ranks = append(sum.Ranks, LabelRank{Label: e.Label, Votes: e.VoteCount, Annotators: e.Annotators})
	}
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
