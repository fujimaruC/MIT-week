package httpapi

import (
	"context"

	"github.com/amba/amba/backend/internal/grounding"
	"github.com/amba/amba/backend/internal/retrieval"
)

// newsSearcher lets the keyless news search (internal/retrieval) serve as the
// pipeline's grounding.Searcher when neither Google Fact Check nor Tavily is
// configured. Its items carry no fact-checker rating, so the model judges
// them like any other result, and only a fact-check article can contradict.
type newsSearcher struct{ s *retrieval.Searcher }

func (n newsSearcher) Enabled() bool { return n.s != nil }

func (n newsSearcher) Search(ctx context.Context, query string) ([]grounding.Result, error) {
	items, err := n.s.Search(ctx, query)
	if err != nil {
		return nil, grounding.ErrSearchFailed
	}
	out := make([]grounding.Result, 0, len(items))
	for _, it := range items {
		out = append(out, grounding.Result{Title: it.Title, Publisher: it.Publisher, URL: it.URL, Snippet: it.Snippet})
	}
	return out, nil
}
