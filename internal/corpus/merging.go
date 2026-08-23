package corpus

import (
	"strings"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

// MergeResult 是片段上多人标注的归并结果：一致或分歧。
type MergeResult struct {
	SpanID      int64          `json:"span_id"`
	Layer       model.Layer    `json:"layer"`
	Consistent  bool           `json:"consistent"`
	LabelGroups map[string]int `json:"label_groups"` // 标签 → 投票数
	Annotators  []string       `json:"annotators"`
	DisputeID   int64          `json:"dispute_id,omitempty"`
}

// Merger 按片段归并标注：同层标签一致则片段标记 consistent，
// 出现多个标签则创建分歧并标记 disputed。
type Merger struct {
	spans    *store.CorpusStore
	anns     *store.AnnotationStore
	disputes *store.DisputeStore
}

// NewMerger 创建归并器。
func NewMerger(sp *store.CorpusStore, an *store.AnnotationStore, ds *store.DisputeStore) *Merger {
	return &Merger{spans: sp, anns: an, disputes: ds}
}

// MergeSpan 对片段某层执行归并。
func (m *Merger) MergeSpan(spanID int64, layer model.Layer) (*MergeResult, error) {
	anns, err := m.anns.ListSubmitted(spanID, string(layer))
	if err != nil {
		return nil, err
	}
	labelGroups := map[string]int{} // 标签 → 去重后的标注员数
	labelVoters := map[string]map[string]struct{}{}
	annotators := []string{}
	for _, a := range anns {
		voters, ok := labelVoters[a.Label]
		if !ok {
			voters = map[string]struct{}{}
			labelVoters[a.Label] = voters
		}
		if _, dup := voters[a.Annotator]; !dup {
			voters[a.Annotator] = struct{}{}
			labelGroups[a.Label]++
		}
		annotators = appendUnique(annotators, a.Annotator)
	}
	res := &MergeResult{
		SpanID: spanID, Layer: layer,
		LabelGroups: labelGroups, Annotators: annotators,
	}

	// 无标注 → 维持 pending。
	if len(anns) == 0 {
		res.Consistent = false
		return res, nil
	}
	// 单标签 → 一致。
	if len(labelGroups) == 1 {
		res.Consistent = true
		_ = m.spans.UpdateSpanStatus(spanID, model.SpanConsistent, store.Now())
		return res, nil
	}
	// 多标签 → 分歧。
	res.Consistent = false
	_ = m.spans.UpdateSpanStatus(spanID, model.SpanDisputed, store.Now())

	dispute, err := m.disputes.GetBySpanLayer(spanID, string(layer))
	if err == model.ErrNotFound {
		dispute = &model.Dispute{
			SpanID: spanID, Layer: layer, LabelCount: len(labelGroups), CreatedAt: store.Now(),
		}
		id, cerr := m.disputes.Create(dispute)
		if cerr != nil {
			return nil, cerr
		}
		dispute.ID = id
	} else if err != nil {
		return nil, err
	}
	res.DisputeID = dispute.ID

	// 重建矩阵条目。每个标签按标注员去重计票，避免同一标注员跨版本/重审
	// 产生多条标注行时被重复计入。
	for label := range labelGroups {
		who := []string{}
		for _, a := range anns {
			if a.Label == label {
				who = appendUnique(who, a.Annotator)
			}
		}
		if err := m.disputes.UpsertMatrixEntry(model.MatrixEntry{
			Label: label, Annotators: who, VoteCount: len(who),
		}, dispute.ID); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func appendUnique(list []string, v string) []string {
	for _, e := range list {
		if e == v {
			return list
		}
	}
	return append(list, v)
}

// LabelsOf 返回某个分歧下所有候选标签。
func LabelsOf(s string) []string {
	parts := strings.Split(s, ",")
	out := []string{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
