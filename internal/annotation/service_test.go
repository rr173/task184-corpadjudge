package annotation

import (
	"errors"
	"testing"

	"task184-corpadjudge/internal/model"
)

type annotationListStub struct{ byLayer map[string][]model.Annotation }

func (s annotationListStub) ListBySpan(_ int64, layer string) ([]model.Annotation, error) {
	return s.byLayer[layer], nil
}

func TestVoteKeyAndCrossLayerGuard(t *testing.T) {
	key := VoteKey("ann-a", 7, model.LayerNamedEnt, "B-PER", 3)
	if key != "ann-a:7:namedent:B-PER:3" {
		t.Fatalf("vote key=%q", key)
	}
	guard := NewCrossLayerGuard(annotationListStub{byLayer: map[string][]model.Annotation{
		string(model.LayerNamedEnt): {{Annotator: "ann-a", Label: "B-PER"}},
	}})
	if err := guard.Check("ann-a", 7, model.LayerCoref); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("cross-layer check error=%v, want ErrInvalidInput", err)
	}
	if err := guard.Check("ann-b", 7, model.LayerCoref); err != nil {
		t.Fatalf("different annotator should pass: %v", err)
	}
}
