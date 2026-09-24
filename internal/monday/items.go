package monday

import "context"

const listGroupsQuery = `query ListGroups($boardID: ID!) {
  boards(ids: [$boardID]) { groups { id title color position } }
}`

const listColumnsQuery = `query ListColumns($boardID: ID!) {
  boards(ids: [$boardID]) { columns { id title type settings_str } }
}`

const listItemsQuery = `query ListItems($boardID: ID!, $limit: Int!) {
  boards(ids: [$boardID]) {
    items_page(limit: $limit) {
      cursor
      items { id name state group { id title } column_values { id text value type } }
    }
  }
}`

const createItemMutation = `mutation CreateItem($boardID: ID!, $groupID: String, $itemName: String!, $columnValues: JSON) {
  create_item(board_id: $boardID, group_id: $groupID, item_name: $itemName, column_values: $columnValues) { id name state }
}`

const updateItemValuesMutation = `mutation UpdateItemValues($boardID: ID!, $itemID: ID!, $columnValues: JSON!) {
  change_multiple_column_values(board_id: $boardID, item_id: $itemID, column_values: $columnValues) { id name state }
}`

const moveItemMutation = `mutation MoveItem($itemID: ID!, $groupID: String!) {
  move_item_to_group(item_id: $itemID, group_id: $groupID) { id name state }
}`

const archiveItemMutation = `mutation ArchiveItem($itemID: ID!) {
  archive_item(item_id: $itemID) { id name state }
}`

// Group is the provider-neutral representation of a monday board group.
type Group struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Color    string `json:"color,omitempty"`
	Position string `json:"position,omitempty"`
}

// Column is the provider-neutral representation of a monday board column.
type Column struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Settings string `json:"settings_str,omitempty"`
}

// ColumnValue contains the display and raw value returned by monday.
type ColumnValue struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

// Item is the provider-neutral representation of a board item.
type Item struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	State        string        `json:"state,omitempty"`
	Group        *Group        `json:"group,omitempty"`
	ColumnValues []ColumnValue `json:"column_values,omitempty"`
}

func (c *Client) ListGroups(ctx context.Context, boardID string) ([]Group, error) {
	var data struct {
		Boards []struct {
			Groups []Group `json:"groups"`
		} `json:"boards"`
	}
	if err := c.Do(ctx, listGroupsQuery, map[string]any{"boardID": boardID}, &data); err != nil {
		return nil, err
	}
	if len(data.Boards) == 0 {
		return nil, &NotFoundError{Resource: "board", ID: boardID}
	}
	return data.Boards[0].Groups, nil
}

func (c *Client) ListColumns(ctx context.Context, boardID string) ([]Column, error) {
	var data struct {
		Boards []struct {
			Columns []Column `json:"columns"`
		} `json:"boards"`
	}
	if err := c.Do(ctx, listColumnsQuery, map[string]any{"boardID": boardID}, &data); err != nil {
		return nil, err
	}
	if len(data.Boards) == 0 {
		return nil, &NotFoundError{Resource: "board", ID: boardID}
	}
	return data.Boards[0].Columns, nil
}

func (c *Client) ListItems(ctx context.Context, boardID string, limit int) ([]Item, error) {
	var data struct {
		Boards []struct {
			ItemsPage struct {
				Items []Item `json:"items"`
			} `json:"items_page"`
		} `json:"boards"`
	}
	if err := c.Do(ctx, listItemsQuery, map[string]any{"boardID": boardID, "limit": limit}, &data); err != nil {
		return nil, err
	}
	if len(data.Boards) == 0 {
		return nil, &NotFoundError{Resource: "board", ID: boardID}
	}
	return data.Boards[0].ItemsPage.Items, nil
}

func (c *Client) CreateItem(ctx context.Context, boardID, groupID, name string, values map[string]any) (*Item, error) {
	var data struct {
		Item Item `json:"create_item"`
	}
	if err := c.Do(ctx, createItemMutation, map[string]any{"boardID": boardID, "groupID": groupID, "itemName": name, "columnValues": values}, &data); err != nil {
		return nil, err
	}
	return &data.Item, nil
}

func (c *Client) UpdateItemValues(ctx context.Context, boardID, itemID string, values map[string]any) (*Item, error) {
	var data struct {
		Item Item `json:"change_multiple_column_values"`
	}
	if err := c.Do(ctx, updateItemValuesMutation, map[string]any{"boardID": boardID, "itemID": itemID, "columnValues": values}, &data); err != nil {
		return nil, err
	}
	return &data.Item, nil
}

func (c *Client) MoveItem(ctx context.Context, itemID, groupID string) (*Item, error) {
	var data struct {
		Item Item `json:"move_item_to_group"`
	}
	if err := c.Do(ctx, moveItemMutation, map[string]any{"itemID": itemID, "groupID": groupID}, &data); err != nil {
		return nil, err
	}
	return &data.Item, nil
}

func (c *Client) ArchiveItem(ctx context.Context, itemID string) (*Item, error) {
	var data struct {
		Item Item `json:"archive_item"`
	}
	if err := c.Do(ctx, archiveItemMutation, map[string]any{"itemID": itemID}, &data); err != nil {
		return nil, err
	}
	return &data.Item, nil
}
