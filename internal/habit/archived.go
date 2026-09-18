package habit

import (
	"context"

	"traya-bah-service/internal/common"
)

// ArchiveResult is the archive/unarchive response.
type ArchiveResult struct {
	ArchivedProductIDs []string `json:"archivedProductIds"`
	ActiveProductIDs   []string `json:"activeProductIds"`
}

// GetArchivedProductIDs returns the user's archived set.
func (s *Service) GetArchivedProductIDs(ctx context.Context, userID string) (map[string]bool, error) {
	return s.Store.ArchivedProductIDs(ctx, userID)
}

// GetUserProductIDs mirrors bahArchivedProductsService.getUserProductIds:
// the distinct product ids on the user's most recent active log.
func (s *Service) GetUserProductIDs(ctx context.Context, userID string) ([]string, error) {
	pp, err := s.Store.LatestActivityLogPrescriptions(ctx, userID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []string{}
	for _, p := range pp {
		id, _ := p["product_id"].(string)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

func without(all []string, drop map[string]bool) []string {
	out := []string{}
	for _, id := range all {
		if !drop[id] {
			out = append(out, id)
		}
	}
	return out
}

// ArchiveProduct mirrors bahArchivedProductsService.archiveProduct (last-product rule).
func (s *Service) ArchiveProduct(ctx context.Context, userID, productID string) (*ArchiveResult, error) {
	if productID == "" {
		return nil, common.BadRequest(MsgProductIDRequired)
	}
	universe, err := s.GetUserProductIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	archived, err := s.GetArchivedProductIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	active := without(universe, archived)
	for _, id := range active {
		if id == productID && len(active) <= 1 {
			return nil, common.BadRequest(MsgCannotRemoveLast)
		}
	}
	ids, err := s.Store.AddArchivedProduct(ctx, userID, productID, s.now())
	if err != nil {
		return nil, err
	}
	newSet := map[string]bool{}
	for _, id := range ids {
		newSet[id] = true
	}
	return &ArchiveResult{ArchivedProductIDs: ids, ActiveProductIDs: without(universe, newSet)}, nil
}

// UnarchiveProduct mirrors bahArchivedProductsService.unarchiveProduct.
func (s *Service) UnarchiveProduct(ctx context.Context, userID, productID string) (*ArchiveResult, error) {
	if productID == "" {
		return nil, common.BadRequest(MsgProductIDRequired)
	}
	universe, err := s.GetUserProductIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids, err := s.Store.RemoveArchivedProduct(ctx, userID, productID, s.now())
	if err != nil {
		return nil, err
	}
	newSet := map[string]bool{}
	for _, id := range ids {
		newSet[id] = true
	}
	return &ArchiveResult{ArchivedProductIDs: ids, ActiveProductIDs: without(universe, newSet)}, nil
}

// FilterArchivedProducts is the pure list filter (kept for parity with the JS helper).
func FilterArchivedProducts(list []map[string]any, archived map[string]bool) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		id, _ := p["product_id"].(string)
		if !archived[id] {
			out = append(out, p)
		}
	}
	return out
}
