package monday

import (
	"context"

	"github.com/jersonmartinez/mcp-monday-projects/internal/domain"
)

const assetFields = `id name url public_url url_thumbnail file_extension file_size created_at uploaded_by { id name }`

const itemAssetsQuery = `query ItemAssets($ids: [ID!]!, $source: AssetsSource, $columnIDs: [String]) {
  items(ids: $ids, limit: 100) {
    id
    assets(assets_source: $source, column_ids: $columnIDs) { ` + assetFields + ` }
  }
}`

const updateAssetsQuery = `query UpdateAssets($ids: [ID!]!) {
  updates(ids: $ids) {
    id
    assets { ` + assetFields + ` }
  }
}`

// ListItemAssets returns file metadata attached to an item, its gallery, file
// columns, and — in current API versions — its updates when source is all.
func (c *Client) ListItemAssets(ctx context.Context, itemID, source string, columnIDs []string) ([]domain.Asset, error) {
	if source == "" {
		source = "all"
	}
	var data struct {
		Items []struct {
			ID     string         `json:"id"`
			Assets []domain.Asset `json:"assets"`
		} `json:"items"`
	}
	err := c.Do(ctx, itemAssetsQuery, map[string]any{"ids": []string{itemID}, "source": source, "columnIDs": columnIDs}, &data)
	if err != nil {
		return nil, err
	}
	if len(data.Items) == 0 {
		return nil, &NotFoundError{Resource: "item", ID: itemID}
	}
	return data.Items[0].Assets, nil
}

// ListUpdateAssets returns file metadata attached to one update.
func (c *Client) ListUpdateAssets(ctx context.Context, updateID string) ([]domain.Asset, error) {
	var data struct {
		Updates []struct {
			ID     string         `json:"id"`
			Assets []domain.Asset `json:"assets"`
		} `json:"updates"`
	}
	if err := c.Do(ctx, updateAssetsQuery, map[string]any{"ids": []string{updateID}}, &data); err != nil {
		return nil, err
	}
	if len(data.Updates) == 0 {
		return nil, &NotFoundError{Resource: "update", ID: updateID}
	}
	return data.Updates[0].Assets, nil
}
