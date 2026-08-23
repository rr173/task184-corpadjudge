package annotation

import (
	"fmt"

	"task184-corpadjudge/internal/model"
)

// 层级混淆防护：某些标签组合在语言层上互斥（例如在命名实体层标注 "B-PER"
// 时，语义角色层不应标注 "AGENT" 于同一范围）。此处提供静态规则集，供校验与
// 自检引用，属于领域约束而非数据录入判断。

// layerConflict 返回两个层的互斥规则；无冲突返回 nil。
type conflictRule struct {
	otherLayer model.Layer
	reason     string
}

// conflictsOf 返回指定层的全部互斥规则。
func conflictsOf(l model.Layer) []conflictRule {
	switch l {
	case model.LayerNamedEnt:
		return []conflictRule{
			{otherLayer: model.LayerCoref, reason: "命名实体层与指代层互斥：同一范围不允许既标实体又标指代"},
		}
	case model.LayerCoref:
		return []conflictRule{
			{otherLayer: model.LayerNamedEnt, reason: "指代层与命名实体层互斥"},
		}
	case model.LayerSentiment:
		return []conflictRule{
			{otherLayer: model.LayerSemRole, reason: "情感极性层与语义角色层互斥：情感标签不承载论元角色"},
		}
	case model.LayerSemRole:
		return []conflictRule{
			{otherLayer: model.LayerSentiment, reason: "语义角色层与情感极性层互斥"},
		}
	}
	return nil
}

// CrossLayerGuard 检测同一标注员对同一片段是否提交了互斥层的标注。
// 命中互斥规则时返回错误，防止不同层级混淆。
type CrossLayerGuard struct {
	annotations AnnotationLister
}

// AnnotationLister 抽象标注查询，便于测试替身。
type AnnotationLister interface {
	ListBySpan(spanID int64, layer string) ([]model.Annotation, error)
}

// NewCrossLayerGuard 创建层级混淆守卫。
func NewCrossLayerGuard(a AnnotationLister) *CrossLayerGuard { return &CrossLayerGuard{annotations: a} }

// Check 检查标注员在片段上的既有标注是否与新层互斥。
func (g *CrossLayerGuard) Check(annotator string, spanID int64, newLayer model.Layer) error {
	for _, rule := range conflictsOf(newLayer) {
		existing, err := g.annotations.ListBySpan(spanID, string(rule.otherLayer))
		if err != nil {
			return err
		}
		for _, a := range existing {
			if a.Annotator == annotator {
				return fmt.Errorf("%w: %s（标注员 %s 已在 %s 层标注 %q）",
					model.ErrInvalidInput, rule.reason, annotator, rule.otherLayer, a.Label)
			}
		}
	}
	return nil
}
