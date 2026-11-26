package application

import (
	"context"
	"strings"

	"github.com/jersonmartinez/mcp-monday-projects/internal/domain"
)

func redactAssetURLs(assets []domain.Asset, includeURLs bool) []domain.Asset {
	out := append([]domain.Asset(nil), assets...)
	for index := range out {
		if !includeURLs {
			out[index].PublicURL = ""
			out[index].ThumbnailURL = ""
		}
	}
	return out
}

// ListItemAssets returns file metadata for an item. Temporary public URLs are
// omitted unless includeURLs is explicitly true because they grant time-limited
// access to the underlying file.
func (s *Service) ListItemAssets(ctx context.Context, itemID, source string, columnIDs []string, includeURLs bool) ([]domain.Asset, error) {
	if err := requireID("item_id", itemID); err != nil {
		return nil, err
	}
	if err := oneOf("source", source, "all", "columns", "gallery"); err != nil {
		return nil, err
	}
	for _, columnID := range columnIDs {
		if strings.TrimSpace(columnID) == "" {
			return nil, invalid("column_ids cannot contain empty values")
		}
	}
	// Resolve the item first so workspace scope and visibility are enforced even
	// when the provider's asset projection is account-wide.
	if _, err := s.GetItem(ctx, itemID); err != nil {
		return nil, err
	}
	assets, err := s.port.ListItemAssets(ctx, itemID, source, columnIDs)
	if err != nil {
		return nil, err
	}
	return redactAssetURLs(assets, includeURLs), nil
}

// ListUpdateAssets returns file metadata for one update after verifying the
// owning item remains visible inside the configured scope.
func (s *Service) ListUpdateAssets(ctx context.Context, updateID, itemID string, includeURLs bool) ([]domain.Asset, error) {
	if err := requireID("update_id", updateID); err != nil {
		return nil, err
	}
	if s.scope != "" && itemID == "" {
		return nil, invalid("item_id is required when MONDAY_WORKSPACE_ID is configured so update ownership can be verified")
	}
	if itemID != "" {
		if err := requireID("item_id", itemID); err != nil {
			return nil, err
		}
		updates, err := s.port.ListItemUpdates(ctx, itemID, 100)
		if err != nil {
			return nil, err
		}
		found := false
		for _, update := range updates {
			if update.ID == updateID {
				found = true
				break
			}
		}
		if !found {
			return nil, invalid("update_id %s does not belong to item_id %s or is outside the first 100 updates", updateID, itemID)
		}
	}
	assets, err := s.port.ListUpdateAssets(ctx, updateID)
	if err != nil {
		return nil, err
	}
	return redactAssetURLs(assets, includeURLs), nil
}
