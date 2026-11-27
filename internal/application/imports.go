package application

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/jersonmartinez/mcp-monday-projects/internal/domain"
)

// BulkCreateRow is one item creation row.
type BulkCreateRow struct {
	Name         string         `json:"name" jsonschema:"item name"`
	GroupID      string         `json:"group_id,omitempty" jsonschema:"target group ID; omit for monday's default group"`
	ColumnValues map[string]any `json:"column_values,omitempty" jsonschema:"column ID to friendly value mapping"`
}

func resolveColumnIDs(columns []domain.Column, values map[string]any) (map[string]any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	byName := map[string]string{}
	byID := map[string]bool{}
	for _, column := range columns {
		byID[column.ID] = true
		byName[strings.ToLower(column.Title)] = column.ID
	}
	out := map[string]any{}
	for key, value := range values {
		id := key
		if !byID[key] {
			resolved, ok := byName[strings.ToLower(key)]
			if !ok {
				return nil, invalid("unknown column %q; use a column ID or exact column title", key)
			}
			id = resolved
		}
		out[id] = value
	}
	return out, nil
}

func resolveGroupFromList(groups []domain.Group, boardID, group string) (string, error) {
	for _, candidate := range groups {
		if candidate.ID == group || strings.EqualFold(candidate.Title, group) {
			if candidate.Archived {
				return "", invalid("group %q is archived", group)
			}
			return candidate.ID, nil
		}
	}
	return "", invalid("group %q was not found on board %s", group, boardID)
}

func (s *Service) resolveGroupInput(ctx context.Context, boardID, group string) (string, error) {
	if strings.TrimSpace(group) == "" {
		return "", nil
	}
	groups, err := s.port.ListGroups(ctx, boardID)
	if err != nil {
		return "", fmt.Errorf("load board groups: %w", err)
	}
	return resolveGroupFromList(groups, boardID, group)
}

func duplicateNames(rows []BulkCreateRow) map[string]bool {
	seen := map[string]int{}
	duplicates := map[string]bool{}
	for _, row := range rows {
		key := strings.ToLower(strings.TrimSpace(row.Name))
		if key == "" {
			continue
		}
		seen[key]++
		if seen[key] > 1 {
			duplicates[key] = true
		}
	}
	return duplicates
}

// BulkCreateItems validates every row before writing anything. With dryRun it
// returns the normalized execution plan only; with dryRun=false it creates rows
// with bounded concurrency and preserves input order in the results.
func (s *Service) BulkCreateItems(ctx context.Context, boardID string, rows []BulkCreateRow, dryRun bool) (BulkResult, error) {
	if err := requireID("board_id", boardID); err != nil {
		return BulkResult{}, err
	}
	if len(rows) == 0 || len(rows) > MaxBulkItems {
		return BulkResult{}, invalid("rows must contain between 1 and %d entries", MaxBulkItems)
	}
	if !dryRun {
		if err := s.guard.CheckBoard(boardID); err != nil {
			return BulkResult{}, err
		}
	}
	columns, err := s.port.ListColumns(ctx, boardID)
	if err != nil {
		return BulkResult{}, fmt.Errorf("load board columns: %w", err)
	}
	var groups []domain.Group
	groupsLoaded := false
	loadGroups := func() ([]domain.Group, error) {
		if !groupsLoaded {
			loaded, err := s.port.ListGroups(ctx, boardID)
			if err != nil {
				return nil, fmt.Errorf("load board groups: %w", err)
			}
			groups = loaded
			groupsLoaded = true
		}
		return groups, nil
	}
	duplicates := duplicateNames(rows)
	result := BulkResult{DryRun: dryRun, Planned: len(rows)}
	valid := true
	for index, input := range rows {
		row := BulkRowOutcome{Row: index + 1, Name: input.Name, Status: "planned"}
		switch {
		case strings.TrimSpace(input.Name) == "":
			row.Status, row.Error, valid = "invalid", "name is required", false
		case duplicates[strings.ToLower(strings.TrimSpace(input.Name))]:
			row.Status, row.Error, valid = "invalid", "duplicate item name inside the import batch", false
		default:
			groupID := ""
			if strings.TrimSpace(input.GroupID) != "" {
				availableGroups, err := loadGroups()
				if err == nil {
					groupID, err = resolveGroupFromList(availableGroups, boardID, input.GroupID)
				}
				if err != nil {
					row.Status, row.Error, valid = "invalid", err.Error(), false
					break
				}
			}
			input.GroupID = groupID
			mapped, err := resolveColumnIDs(columns, input.ColumnValues)
			if err != nil {
				row.Status, row.Error, valid = "invalid", err.Error(), false
				break
			}
			plan := domain.PlanColumnValues(columns, mapped)
			row.Normalized, row.Issues = plan.Normalized, plan.Issues
			if !plan.Valid {
				row.Status, valid = "invalid", false
			}
		}
		rows[index] = input
		result.Rows = append(result.Rows, row)
	}
	if dryRun || !valid {
		if !valid {
			result.Failed = countStatus(result.Rows, "invalid")
		}
		return result, nil
	}
	var mu sync.Mutex
	runBounded(ctx, s.bulkMaxConcurrency, len(result.Rows), func(ctx context.Context, index int) {
		row := &result.Rows[index]
		input := rows[index]
		item, err := s.port.CreateItem(ctx, boardID, input.GroupID, strings.TrimSpace(input.Name), row.Normalized)
		if err != nil {
			row.Status, row.Error = "failed", err.Error()
		} else if item == nil || item.ID == "" {
			row.Status, row.Error = "failed", "monday returned an incomplete created item"
		} else {
			row.Status, row.ItemID = "created", item.ID
		}
		mu.Lock()
		if row.Status == "created" {
			result.Succeeded++
		} else {
			result.Failed++
		}
		mu.Unlock()
	})
	return result, nil
}

func parseCSVRows(text string) ([]map[string]string, error) {
	reader := csv.NewReader(strings.NewReader(text))
	reader.TrimLeadingSpace = true
	headers, err := reader.Read()
	if err != nil {
		return nil, invalid("csv_text must include a header row: %v", err)
	}
	for index := range headers {
		headers[index] = strings.TrimSpace(headers[index])
	}
	rows := []map[string]string{}
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, invalid("invalid csv_text: %v", err)
		}
		row := map[string]string{}
		for index, header := range headers {
			if index < len(record) {
				row[header] = strings.TrimSpace(record[index])
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func parseMarkdownRows(text string) ([]map[string]string, error) {
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "|") && strings.HasSuffix(line, "|") && !strings.Contains(line, "---") {
			lines = append(lines, line)
		}
	}
	if len(lines) < 2 {
		return nil, invalid("markdown_text must contain a header row and at least one data row")
	}
	split := func(line string) []string {
		parts := strings.Split(strings.Trim(line, "|"), "|")
		for index := range parts {
			parts[index] = strings.TrimSpace(parts[index])
		}
		return parts
	}
	headers := split(lines[0])
	rows := []map[string]string{}
	for _, line := range lines[1:] {
		parts := split(line)
		row := map[string]string{}
		for index, header := range headers {
			if index < len(parts) {
				row[header] = parts[index]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func importRowsFromTable(rows []map[string]string) ([]BulkCreateRow, error) {
	out := make([]BulkCreateRow, 0, len(rows))
	for index, row := range rows {
		name := row["name"]
		if name == "" {
			name = row["Name"]
		}
		if strings.TrimSpace(name) == "" {
			return nil, invalid("row %d is missing the required name column", index+1)
		}
		values := map[string]any{}
		group := row["group"]
		if group == "" {
			group = row["Group"]
		}
		for key, value := range row {
			lower := strings.ToLower(key)
			if lower == "name" || lower == "group" || strings.TrimSpace(value) == "" {
				continue
			}
			values[key] = value
		}
		out = append(out, BulkCreateRow{Name: name, GroupID: group, ColumnValues: values})
	}
	return out, nil
}

// ImportBoardTable parses CSV or Markdown rows and delegates to the governed
// bulk-create pipeline. Required columns: name; optional: group. Every other
// header is resolved as a column ID or exact column title.
func (s *Service) ImportBoardTable(ctx context.Context, boardID, format, text string, dryRun bool) (BulkResult, error) {
	var raw []map[string]string
	var err error
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "csv":
		raw, err = parseCSVRows(text)
	case "markdown":
		raw, err = parseMarkdownRows(text)
	default:
		return BulkResult{}, invalid("format must be csv or markdown")
	}
	if err != nil {
		return BulkResult{}, err
	}
	rows, err := importRowsFromTable(raw)
	if err != nil {
		return BulkResult{}, err
	}
	return s.BulkCreateItems(ctx, boardID, rows, dryRun)
}
