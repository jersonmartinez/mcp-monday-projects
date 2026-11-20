package application

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jersonmartinez/mcp-monday-projects/internal/domain"
	"github.com/jersonmartinez/mcp-monday-projects/internal/monday"
)

// Subitems returns one parent's subitems, optionally projecting selected columns.
func (s *Service) Subitems(ctx context.Context, parentID string, columnIDs []string) ([]domain.Subitem, error) {
	if err := requireID("item_id", parentID); err != nil {
		return nil, err
	}
	item, err := s.GetItem(ctx, parentID)
	if err != nil {
		return nil, err
	}
	if len(columnIDs) == 0 {
		return item.Subitems, nil
	}
	wanted := make(map[string]bool, len(columnIDs))
	for _, id := range columnIDs {
		if strings.TrimSpace(id) != "" {
			wanted[id] = true
		}
	}
	for i := range item.Subitems {
		values := item.Subitems[i].ColumnValues[:0]
		for _, value := range item.Subitems[i].ColumnValues {
			if wanted[value.ID] {
				values = append(values, value)
			}
		}
		item.Subitems[i].ColumnValues = values
	}
	return item.Subitems, nil
}

// TextMatch loads a bounded board snapshot and searches names and visible
// column text/raw values. It is deliberately read-only and provider-neutral.
func (s *Service) TextMatch(ctx context.Context, boardID, text string, columnIDs []string, maxItems int) (domain.ItemPage, error) {
	if err := requireID("board_id", boardID); err != nil {
		return domain.ItemPage{}, err
	}
	if err := requireText("text", text); err != nil {
		return domain.ItemPage{}, err
	}
	if maxItems <= 0 || maxItems > s.reportMaxItems {
		maxItems = s.reportMaxItems
	}
	snapshot, err := s.Snapshot(ctx, boardID, maxItems)
	if err != nil {
		return domain.ItemPage{}, err
	}
	needle := strings.ToLower(text)
	wanted := make(map[string]bool, len(columnIDs))
	for _, id := range columnIDs {
		wanted[id] = true
	}
	var result domain.ItemPage
	for _, item := range snapshot.Items {
		matched := strings.Contains(strings.ToLower(item.Name), needle)
		if !matched {
			for _, value := range item.ColumnValues {
				if len(wanted) > 0 && !wanted[value.ID] {
					continue
				}
				if strings.Contains(strings.ToLower(value.Text), needle) || strings.Contains(strings.ToLower(value.Value), needle) {
					matched = true
					break
				}
			}
		}
		if matched {
			result.Items = append(result.Items, item)
		}
	}
	return result, nil
}

// ColumnVerification reports differences between expected friendly values and
// values read back from the board.
type ColumnVerification struct {
	ItemID      string   `json:"item_id"`
	Correct     bool     `json:"correct"`
	Differences []string `json:"differences,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
}

type VerifyResult struct {
	Checked   int                  `json:"checked"`
	Correct   int                  `json:"correct"`
	Different int                  `json:"different"`
	Rows      []ColumnVerification `json:"rows"`
	Warnings  []string             `json:"warnings,omitempty"`
}

// VerifyColumns compares expected values against the persisted item cells.
// Expected values are friendly values, with date/timeline and numeric values
// compared semantically rather than by provider JSON formatting.
func (s *Service) VerifyColumns(ctx context.Context, expected map[string]map[string]any) (VerifyResult, error) {
	if len(expected) == 0 || len(expected) > MaxBulkItems {
		return VerifyResult{}, invalid("expected must contain between 1 and %d items", MaxBulkItems)
	}
	ids := make([]string, 0, len(expected))
	for id := range expected {
		if err := requireID("item_id", id); err != nil {
			return VerifyResult{}, err
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	items, err := s.port.GetItems(ctx, ids)
	if err != nil {
		return VerifyResult{}, err
	}
	byID := make(map[string]domain.Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	out := VerifyResult{Rows: make([]ColumnVerification, 0, len(ids))}
	for _, id := range ids {
		row := ColumnVerification{ItemID: id, Correct: true}
		item, ok := byID[id]
		if !ok {
			row.Correct = false
			row.Differences = []string{"item is not visible or does not exist"}
		}
		if ok {
			for columnID, want := range expected[id] {
				got := itemCell(item, columnID)
				if !equalExpected(got, want) {
					row.Correct = false
					row.Differences = append(row.Differences, fmt.Sprintf("%s: expected %v, stored %s", columnID, want, got.Value))
				}
			}
			if warning := domain.UnreadableColumnsWarning(domain.UnreadableColumns([]domain.Item{item}, mapKeys(expected[id])...)); warning != "" {
				row.Warnings = append(row.Warnings, warning)
			}
		}
		out.Checked++
		if row.Correct {
			out.Correct++
		} else {
			out.Different++
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

func mapKeys(values map[string]any) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	return result
}
func itemCell(item domain.Item, id string) domain.ColumnValue {
	for _, value := range item.ColumnValues {
		if value.ID == id {
			return value
		}
	}
	return domain.ColumnValue{ID: id}
}
func equalExpected(got domain.ColumnValue, want any) bool {
	if want == nil {
		return got.Text == "" && got.Value == ""
	}
	if m, ok := want.(map[string]any); ok {
		var stored map[string]any
		if json.Unmarshal([]byte(got.Value), &stored) != nil {
			return false
		}
		for _, key := range []string{"date", "from", "to"} {
			if expected, exists := m[key]; exists && fmt.Sprint(stored[key]) != fmt.Sprint(expected) {
				return false
			}
		}
		if len(m) > 0 {
			return true
		}
	}
	if s, ok := want.(string); ok {
		return strings.EqualFold(strings.TrimSpace(got.Text), strings.TrimSpace(s)) || strings.TrimSpace(got.Value) == s
	}
	return strings.Contains(got.Text, fmt.Sprint(want)) || strings.Contains(got.Value, fmt.Sprint(want))
}

// PersistedValueWarnings re-reads a just-written cell and warns if Monday
// altered it (notably timeline/date values adjusted by dependencies).
func (s *Service) PersistedValueWarnings(ctx context.Context, itemID, columnID string, requested any) []string {
	if _, ok := requested.(map[string]any); !ok {
		return nil
	}
	item, err := s.GetItem(ctx, itemID)
	if err != nil {
		return []string{fmt.Sprintf("could not verify persisted column %s: %v", columnID, err)}
	}
	cell := itemCell(*item, columnID)
	if warning := domain.UnreadableColumnsWarning(domain.UnreadableColumns([]domain.Item{*item}, columnID)); warning != "" {
		return []string{warning}
	}
	if !equalExpected(cell, requested) {
		return []string{fmt.Sprintf("persisted value differs from requested value for %s: requested=%v stored=%s", columnID, requested, cell.Value)}
	}
	return nil
}

func (s *Service) persistedWarnings(ctx context.Context, itemID string, values map[string]any) []string {
	var warnings []string
	for columnID, value := range values {
		if _, ok := value.(map[string]any); !ok {
			continue
		}
		warnings = append(warnings, s.PersistedValueWarnings(ctx, itemID, columnID, value)...)
	}
	return warnings
}

type ProgressBucket struct {
	Status     string  `json:"status"`
	Phase      string  `json:"phase,omitempty"`
	Items      int     `json:"items"`
	ItemPct    float64 `json:"item_pct"`
	Planned    float64 `json:"planned"`
	Spent      float64 `json:"spent"`
	PlannedPct float64 `json:"planned_pct"`
	SpentPct   float64 `json:"spent_pct"`
}
type ProgressReport struct {
	Scope        string           `json:"scope"`
	TotalItems   int              `json:"total_items"`
	TotalPlanned float64          `json:"total_planned"`
	TotalSpent   float64          `json:"total_spent"`
	Buckets      []ProgressBucket `json:"buckets"`
	Truncated    bool             `json:"truncated"`
	Warnings     []string         `json:"warnings,omitempty"`
}

// Progress computes effort and item rollups grouped by status and optional phase.
func (s *Service) Progress(ctx context.Context, boardID, statusID, phaseID, plannedID, spentID string, maxItems int) (ProgressReport, error) {
	if err := requireID("board_id", boardID); err != nil {
		return ProgressReport{}, err
	}
	for name, id := range map[string]string{"status_column_id": statusID, "planned_column_id": plannedID} {
		if err := requireText(name, id); err != nil {
			return ProgressReport{}, err
		}
	}
	snap, err := s.Snapshot(ctx, boardID, maxItems)
	if err != nil {
		return ProgressReport{}, err
	}
	groups := map[string]*ProgressBucket{}
	report := ProgressReport{Scope: boardID, TotalItems: len(snap.Items), Truncated: snap.Truncated}
	for _, item := range snap.Items {
		status, phase := item.CellText(statusID), item.CellText(phaseID)
		key := status + "\x00" + phase
		bucket := groups[key]
		if bucket == nil {
			bucket = &ProgressBucket{Status: status, Phase: phase}
			groups[key] = bucket
		}
		bucket.Items++
		planned := numericCell(item, plannedID)
		spent := numericCell(item, spentID)
		bucket.Planned += planned
		bucket.Spent += spent
		report.TotalPlanned += planned
		report.TotalSpent += spent
	}
	for _, bucket := range groups {
		if report.TotalItems > 0 {
			bucket.ItemPct = roundPercent(float64(bucket.Items) * 100 / float64(report.TotalItems))
		}
		if report.TotalPlanned > 0 {
			bucket.PlannedPct = roundPercent(bucket.Planned * 100 / report.TotalPlanned)
		}
		if report.TotalSpent > 0 {
			bucket.SpentPct = roundPercent(bucket.Spent * 100 / report.TotalSpent)
		}
		report.Buckets = append(report.Buckets, *bucket)
	}
	sort.Slice(report.Buckets, func(i, j int) bool {
		if report.Buckets[i].Items != report.Buckets[j].Items {
			return report.Buckets[i].Items > report.Buckets[j].Items
		}
		return report.Buckets[i].Status < report.Buckets[j].Status
	})
	if warning := domain.UnreadableColumnsWarning(domain.UnreadableColumns(snap.Items, statusID, phaseID, plannedID, spentID)); warning != "" {
		report.Warnings = append(report.Warnings, warning)
	}
	return report, nil
}
func numericCell(item domain.Item, id string) float64 {
	if id == "" {
		return 0
	}
	cell := itemCell(item, id)
	if n, err := strconv.ParseFloat(strings.TrimSpace(cell.Text), 64); err == nil {
		return n
	}
	if n, err := strconv.ParseFloat(strings.TrimSpace(cell.Value), 64); err == nil {
		return n
	}
	var m map[string]any
	if json.Unmarshal([]byte(cell.Value), &m) == nil {
		for _, key := range []string{"number", "value"} {
			if n, ok := m[key].(float64); ok {
				return n
			}
		}
	}
	return 0
}

func roundPercent(value float64) float64 { return math.Round(value*10) / 10 }

// Dependencies describes both directions of a dependency column.
type Dependencies struct {
	ItemID     string   `json:"item_id"`
	ColumnID   string   `json:"column_id"`
	DependsOn  []string `json:"depends_on"`
	Dependents []string `json:"dependents"`
}

func (s *Service) Dependencies(ctx context.Context, boardID, itemID, columnID string) (Dependencies, error) {
	if err := requireID("board_id", boardID); err != nil {
		return Dependencies{}, err
	}
	if err := requireID("item_id", itemID); err != nil {
		return Dependencies{}, err
	}
	if err := requireText("column_id", columnID); err != nil {
		return Dependencies{}, err
	}
	item, err := s.GetItem(ctx, itemID)
	if err != nil {
		return Dependencies{}, err
	}
	if item.BoardID != "" && item.BoardID != boardID {
		return Dependencies{}, invalid("item %s belongs to board %s, not %s", itemID, item.BoardID, boardID)
	}
	cell := itemCell(*item, columnID)
	result := Dependencies{ItemID: itemID, ColumnID: columnID, DependsOn: append([]string(nil), cell.LinkedItemIDs...)}
	snap, err := s.Snapshot(ctx, boardID, 0)
	if err != nil {
		return Dependencies{}, err
	}
	for _, candidate := range snap.Items {
		for _, linked := range itemCell(candidate, columnID).LinkedItemIDs {
			if linked == itemID {
				result.Dependents = append(result.Dependents, candidate.ID)
				break
			}
		}
	}
	sort.Strings(result.DependsOn)
	sort.Strings(result.Dependents)
	return result, nil
}
func (s *Service) SetDependencies(ctx context.Context, boardID, itemID, columnID, mode string, ids []string) (domain.Item, []string, error) {
	if err := requireID("board_id", boardID); err != nil {
		return domain.Item{}, nil, err
	}
	if err := requireID("item_id", itemID); err != nil {
		return domain.Item{}, nil, err
	}
	if err := requireText("column_id", columnID); err != nil {
		return domain.Item{}, nil, err
	}
	if mode == "" {
		mode = "replace"
	}
	if err := oneOf("mode", mode, "add", "remove", "replace"); err != nil {
		return domain.Item{}, nil, err
	}
	for _, id := range ids {
		if err := requireID("item_ids", id); err != nil {
			return domain.Item{}, nil, err
		}
	}
	item, err := s.GetItem(ctx, itemID)
	if err != nil {
		return domain.Item{}, nil, err
	}
	if item.BoardID != "" && item.BoardID != boardID {
		return domain.Item{}, nil, invalid("item %s belongs to board %s, not %s", itemID, item.BoardID, boardID)
	}
	current := append([]string(nil), itemCell(*item, columnID).LinkedItemIDs...)
	if mode == "replace" {
		current = append([]string(nil), ids...)
	} else {
		set := map[string]bool{}
		for _, id := range current {
			set[id] = true
		}
		if mode == "add" {
			for _, id := range ids {
				set[id] = true
			}
		} else {
			for _, id := range ids {
				delete(set, id)
			}
		}
		current = current[:0]
		for id := range set {
			current = append(current, id)
		}
		sort.Strings(current)
	}
	if len(current) > 0 {
		found, err := s.port.GetItems(ctx, current)
		if err != nil {
			return domain.Item{}, nil, err
		}
		if len(found) != len(current) {
			return domain.Item{}, nil, invalid("all dependency item IDs must exist and be visible on the board")
		}
		for _, candidate := range found {
			if candidate.BoardID != "" && candidate.BoardID != boardID {
				return domain.Item{}, nil, invalid("dependency item %s belongs to another board", candidate.ID)
			}
		}
	}
	updated, err := s.UpdateItemValues(ctx, boardID, itemID, map[string]any{columnID: current})
	if err != nil {
		return domain.Item{}, nil, err
	}
	warnings := s.PersistedValueWarnings(ctx, itemID, columnID, current)
	return *updated, warnings, nil
}

// BulkUpdateUpdates posts updates one item at a time and returns every outcome.
type UpdateOutcome struct {
	ItemID string         `json:"item_id"`
	Status string         `json:"status"`
	Update *domain.Update `json:"update,omitempty"`
	Error  string         `json:"error,omitempty"`
}
type BulkUpdatesResult struct {
	Planned   int             `json:"planned"`
	Succeeded int             `json:"succeeded"`
	Failed    int             `json:"failed"`
	Rows      []UpdateOutcome `json:"rows"`
}

func (s *Service) BulkCreateUpdates(ctx context.Context, updates map[string]string, common string, dryRun bool) (BulkUpdatesResult, error) {
	if len(updates) == 0 || len(updates) > MaxBulkItems {
		return BulkUpdatesResult{}, invalid("updates must contain between 1 and %d items", MaxBulkItems)
	}
	ids := make([]string, 0, len(updates))
	for id := range updates {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := BulkUpdatesResult{Planned: len(ids), Rows: make([]UpdateOutcome, 0, len(ids))}
	for _, id := range ids {
		body := updates[id]
		if body == "" {
			body = common
		}
		row := UpdateOutcome{ItemID: id, Status: "planned"}
		if err := requireID("item_id", id); err != nil {
			row.Status = "failed"
			row.Error = err.Error()
		} else if err := requireText("body", body); err != nil {
			row.Status = "failed"
			row.Error = err.Error()
		} else if !dryRun {
			u, err := s.CreateUpdate(ctx, id, body)
			if err != nil {
				row.Status = "failed"
				row.Error = err.Error()
			} else {
				row.Status = "created"
				row.Update = u
			}
		}
		if row.Status == "failed" {
			out.Failed++
		} else if row.Status == "created" {
			out.Succeeded++
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

// SubitemsRollup summarizes status counts for several parent items.
type SubitemRollup struct {
	ItemID   string         `json:"item_id"`
	Total    int            `json:"total"`
	ByStatus []domain.Count `json:"by_status"`
}

func (s *Service) SubitemsRollup(ctx context.Context, ids []string, statusID string) ([]SubitemRollup, error) {
	if err := requireIDs("item_ids", ids, MaxBulkItems); err != nil {
		return nil, err
	}
	if err := requireText("status_column_id", statusID); err != nil {
		return nil, err
	}
	items, err := s.port.GetItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]SubitemRollup, 0, len(items))
	for _, item := range items {
		counts := map[string]int{}
		for _, sub := range item.Subitems {
			counts[orDash(subCellText(sub, statusID))]++
		}
		result = append(result, SubitemRollup{ItemID: item.ID, Total: len(item.Subitems), ByStatus: countsList(counts)})
	}
	return result, nil
}
func subCellText(item domain.Subitem, id string) string {
	for _, v := range item.ColumnValues {
		if v.ID == id {
			return v.Text
		}
	}
	return ""
}
func countsList(m map[string]int) []domain.Count {
	r := make([]domain.Count, 0, len(m))
	for k, v := range m {
		r = append(r, domain.Count{Key: k, Count: v})
	}
	sort.Slice(r, func(i, j int) bool {
		if r[i].Count != r[j].Count {
			return r[i].Count > r[j].Count
		}
		return r[i].Key < r[j].Key
	})
	return r
}
func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// PreviewDependencyMove computes a bounded dependency cascade without writing.
type DependencyPreview struct {
	ItemID        string `json:"item_id"`
	CurrentFrom   string `json:"current_from,omitempty"`
	CurrentTo     string `json:"current_to,omitempty"`
	ProjectedFrom string `json:"projected_from,omitempty"`
	ProjectedTo   string `json:"projected_to,omitempty"`
	ShiftDays     int    `json:"shift_days"`
}

func (s *Service) PreviewDependencyMove(ctx context.Context, boardID, itemID, timelineColumnID, dependencyColumnID, newFrom, newTo string, maxItems int) ([]DependencyPreview, error) {
	if err := requireID("board_id", boardID); err != nil {
		return nil, err
	}
	if err := requireID("item_id", itemID); err != nil {
		return nil, err
	}
	if err := requireText("timeline_column_id", timelineColumnID); err != nil {
		return nil, err
	}
	if err := requireText("dependency_column_id", dependencyColumnID); err != nil {
		return nil, err
	}
	for name, value := range map[string]string{"from": newFrom, "to": newTo} {
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return nil, invalid("%s must be YYYY-MM-DD", name)
		}
	}
	if newTo < newFrom {
		return nil, invalid("to must not be before from")
	}
	snap, err := s.Snapshot(ctx, boardID, maxItems)
	if err != nil {
		return nil, err
	}
	target := domain.Item{}
	for _, item := range snap.Items {
		if item.ID == itemID {
			target = item
			break
		}
	}
	if target.ID == "" {
		return nil, &monday.NotFoundError{Resource: "item", ID: itemID}
	}
	raw := itemCell(target, timelineColumnID).Value
	var old map[string]any
	_ = json.Unmarshal([]byte(raw), &old)
	oldFrom, _ := old["from"].(string)
	shift := 0
	if oldFrom != "" {
		a, _ := time.Parse("2006-01-02", oldFrom)
		b, _ := time.Parse("2006-01-02", newFrom)
		shift = int(b.Sub(a).Hours() / 24)
	}
	var result []DependencyPreview
	for _, item := range snap.Items {
		if item.ID == itemID {
			continue
		}
		linked := false
		for _, id := range itemCell(item, dependencyColumnID).LinkedItemIDs {
			if id == itemID {
				linked = true
				break
			}
		}
		if !linked {
			continue
		}
		cell := itemCell(item, timelineColumnID)
		var value map[string]any
		_ = json.Unmarshal([]byte(cell.Value), &value)
		from, _ := value["from"].(string)
		to, _ := value["to"].(string)
		projectedFrom, projectedTo := from, to
		if shift != 0 {
			if d, e := time.Parse("2006-01-02", from); e == nil {
				projectedFrom = d.AddDate(0, 0, shift).Format("2006-01-02")
			}
			if d, e := time.Parse("2006-01-02", to); e == nil {
				projectedTo = d.AddDate(0, 0, shift).Format("2006-01-02")
			}
		}
		result = append(result, DependencyPreview{ItemID: item.ID, CurrentFrom: from, CurrentTo: to, ProjectedFrom: projectedFrom, ProjectedTo: projectedTo, ShiftDays: shift})
	}
	return result, nil
}
