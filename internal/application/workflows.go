package application

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jersonmartinez/mcp-monday-projects/internal/domain"
	"github.com/jersonmartinez/mcp-monday-projects/internal/monday"
)

// AutomationRule is a deterministic, server-side workflow rule. It is not a
// native monday automation; it is an auditable one-shot transition pipeline
// executed through the same guarded write path as every other mutation.
type AutomationRule struct {
	Name            string         `json:"name,omitempty" jsonschema:"human-readable rule name"`
	ColumnID        string         `json:"column_id" jsonschema:"column evaluated for matching"`
	Operator        string         `json:"operator,omitempty" jsonschema:"equals (default), not_equals, empty, not_empty, contains"`
	Value           string         `json:"value,omitempty" jsonschema:"case-insensitive comparison text"`
	SetColumnValues map[string]any `json:"set_column_values,omitempty" jsonschema:"friendly column values applied to matching items"`
	MoveGroupID     string         `json:"move_group_id,omitempty" jsonschema:"optional destination group"`
	UpdateBody      string         `json:"update_body,omitempty" jsonschema:"optional update posted to matching items"`
}

// AutomationRunRow reports one evaluated/mutated item.
type AutomationRunRow struct {
	ItemID string `json:"item_id"`
	Name   string `json:"name"`
	Rule   string `json:"rule"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// AutomationRunResult summarizes a custom automation execution.
type AutomationRunResult struct {
	DryRun    bool               `json:"dry_run"`
	BoardID   string             `json:"board_id"`
	Scanned   int                `json:"scanned"`
	Matched   int                `json:"matched"`
	Succeeded int                `json:"succeeded"`
	Failed    int                `json:"failed"`
	Rows      []AutomationRunRow `json:"rows"`
	Warnings  []string           `json:"warnings,omitempty"`
}

func automationMatch(item domain.Item, rule AutomationRule) bool {
	cell := itemCell(item, rule.ColumnID)
	text := strings.ToLower(strings.TrimSpace(cell.Text))
	want := strings.ToLower(strings.TrimSpace(rule.Value))
	switch rule.Operator {
	case "", "equals":
		return text == want
	case "not_equals":
		return text != want
	case "empty":
		return text == ""
	case "not_empty":
		return text != ""
	case "contains":
		return want != "" && strings.Contains(text, want)
	}
	return false
}

// RunBoardAutomation evaluates deterministic rules against a bounded board
// snapshot and optionally applies their transitions. dryRun must be true for a
// preview; writes validate the complete rule set before any mutation.
func (s *Service) RunBoardAutomation(ctx context.Context, boardID string, rules []AutomationRule, limit int, dryRun bool) (AutomationRunResult, error) {
	if err := requireID("board_id", boardID); err != nil {
		return AutomationRunResult{}, err
	}
	if len(rules) == 0 || len(rules) > 10 {
		return AutomationRunResult{}, invalid("rules must contain between 1 and 10 entries")
	}
	limit, err := BoundLimit(limit, 100, 200)
	if err != nil {
		return AutomationRunResult{}, err
	}
	if !dryRun {
		if err := s.guard.CheckBoard(boardID); err != nil {
			return AutomationRunResult{}, err
		}
	}
	columns, err := s.port.ListColumns(ctx, boardID)
	if err != nil {
		return AutomationRunResult{}, err
	}
	validColumns := map[string]bool{"name": true}
	for _, column := range columns {
		validColumns[column.ID] = true
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
	normalizedRules := make([]map[string]any, len(rules))
	for index, rule := range rules {
		if rule.Name == "" {
			rule.Name = fmt.Sprintf("rule_%d", index+1)
			rules[index].Name = rule.Name
		}
		if strings.TrimSpace(rule.ColumnID) == "" || !validColumns[rule.ColumnID] {
			return AutomationRunResult{}, invalid("rules[%d].column_id is required and must exist on the board", index)
		}
		if err := oneOf(fmt.Sprintf("rules[%d].operator", index), rule.Operator, "equals", "not_equals", "empty", "not_empty", "contains"); err != nil {
			return AutomationRunResult{}, err
		}
		if rule.Operator == "" {
			rules[index].Operator = "equals"
		}
		if len(rule.SetColumnValues) == 0 && rule.MoveGroupID == "" && rule.UpdateBody == "" {
			return AutomationRunResult{}, invalid("rules[%d] has no action; provide set_column_values, move_group_id, or update_body", index)
		}
		mapped, err := resolveColumnIDs(columns, rule.SetColumnValues)
		if err != nil {
			return AutomationRunResult{}, err
		}
		plan := domain.PlanColumnValues(columns, mapped)
		if err := plan.Err(); err != nil {
			return AutomationRunResult{}, invalid("rules[%d].set_column_values is invalid: %v", index, err)
		}
		normalizedRules[index] = plan.Normalized
		if rule.MoveGroupID != "" {
			availableGroups, err := loadGroups()
			if err != nil {
				return AutomationRunResult{}, err
			}
			groupID, err := resolveGroupFromList(availableGroups, boardID, rule.MoveGroupID)
			if err != nil {
				return AutomationRunResult{}, err
			}
			rules[index].MoveGroupID = groupID
		}
	}
	snapshot, err := s.Snapshot(ctx, boardID, limit)
	if err != nil {
		return AutomationRunResult{}, err
	}
	result := AutomationRunResult{DryRun: dryRun, BoardID: boardID, Scanned: len(snapshot.Items)}
	if snapshot.Truncated {
		result.Warnings = append(result.Warnings, "board snapshot truncated; automation evaluated only the bounded item set")
	}
	type task struct {
		rowIndex int
		itemID   string
		rule     int
	}
	tasks := []task{}
	for _, item := range snapshot.Items {
		matched := false
		for ruleIndex, rule := range rules {
			if !automationMatch(item, rule) {
				continue
			}
			matched = true
			result.Rows = append(result.Rows, AutomationRunRow{ItemID: item.ID, Name: item.Name, Rule: rule.Name, Status: "planned"})
			tasks = append(tasks, task{rowIndex: len(result.Rows) - 1, itemID: item.ID, rule: ruleIndex})
			break // first matching rule wins, making execution deterministic
		}
		_ = matched
	}
	result.Matched = len(result.Rows)
	if dryRun || len(tasks) == 0 {
		return result, nil
	}
	var mu sync.Mutex
	runBounded(ctx, s.bulkMaxConcurrency, len(tasks), func(ctx context.Context, index int) {
		task := tasks[index]
		row := &result.Rows[task.rowIndex]
		ruleIndex := task.rule
		rule := rules[ruleIndex]
		status := "applied"
		if len(normalizedRules[ruleIndex]) > 0 {
			if _, err := s.port.UpdateItemValues(ctx, boardID, task.itemID, normalizedRules[ruleIndex]); err != nil {
				row.Status, row.Error = "failed", err.Error()
				status = "failed"
			}
		}
		if status != "failed" && rule.MoveGroupID != "" {
			if _, err := s.port.MoveItem(ctx, task.itemID, rule.MoveGroupID); err != nil {
				row.Status, row.Error = "failed", err.Error()
				status = "failed"
			}
		}
		if status != "failed" && strings.TrimSpace(rule.UpdateBody) != "" {
			if _, err := s.port.CreateUpdate(ctx, task.itemID, monday.FormatUpdateBody(rule.UpdateBody), ""); err != nil {
				row.Status, row.Error = "failed", err.Error()
				status = "failed"
			}
		}
		if status != "failed" {
			row.Status = status
		}
		mu.Lock()
		if status == "failed" {
			result.Failed++
		} else {
			result.Succeeded++
		}
		mu.Unlock()
	})
	return result, nil
}

// CompleteItemResult reports the deterministic completion workflow.
type CompleteItemResult struct {
	DryRun       bool     `json:"dry_run"`
	ItemID       string   `json:"item_id"`
	Name         string   `json:"name"`
	StatusColumn string   `json:"status_column_id,omitempty"`
	DoneValue    string   `json:"done_value"`
	AlreadyDone  bool     `json:"already_done"`
	Updated      bool     `json:"updated"`
	MovedGroupID string   `json:"moved_group_id,omitempty"`
	UpdatePosted bool     `json:"update_posted"`
	Warnings     []string `json:"warnings,omitempty"`
}

func isDoneText(text string, doneValue string) bool {
	value := strings.ToLower(strings.TrimSpace(text))
	if doneValue != "" {
		return value == strings.ToLower(strings.TrimSpace(doneValue))
	}
	for _, done := range []string{"done", "listo", "hecho", "completado", "completed", "terminado", "closed", "cerrado"} {
		if value == done {
			return true
		}
	}
	return false
}

// CompleteItem performs a small, auditable completion workflow: validate the
// target item, move its status to a Done-like value, optionally move it to a
// closing group, and optionally post a completion update. It is dry-run by
// default because it composes multiple writes.
func (s *Service) CompleteItem(ctx context.Context, itemID, statusColumnID, doneValue, moveGroupID, updateBody string, dryRun bool) (CompleteItemResult, error) {
	if err := requireID("item_id", itemID); err != nil {
		return CompleteItemResult{}, err
	}
	item, err := s.GetItem(ctx, itemID)
	if err != nil {
		return CompleteItemResult{}, err
	}
	boardID := item.BoardID
	columns, err := s.port.ListColumns(ctx, boardID)
	if err != nil {
		return CompleteItemResult{}, err
	}
	if statusColumnID == "" {
		statusColumnID = domain.DetectReportColumns(columns, domain.ReportColumns{}).StatusColumnID
	}
	if statusColumnID == "" {
		return CompleteItemResult{}, invalid("no status column found; provide status_column_id")
	}
	if doneValue == "" {
		doneValue = "Done"
	}
	if moveGroupID != "" {
		resolvedGroupID, err := s.resolveGroupInput(ctx, boardID, moveGroupID)
		if err != nil {
			return CompleteItemResult{}, err
		}
		moveGroupID = resolvedGroupID
	}
	plan := domain.PlanColumnValues(columns, map[string]any{statusColumnID: doneValue})
	if err := plan.Err(); err != nil {
		return CompleteItemResult{}, invalid("done value is invalid for the status column: %v", err)
	}
	result := CompleteItemResult{DryRun: dryRun, ItemID: item.ID, Name: item.Name, StatusColumn: statusColumnID, DoneValue: doneValue, AlreadyDone: isDoneText(itemCell(*item, statusColumnID).Text, doneValue)}
	if dryRun {
		result.Updated = !result.AlreadyDone
		result.MovedGroupID = moveGroupID
		result.UpdatePosted = strings.TrimSpace(updateBody) != ""
		return result, nil
	}
	if err := s.guard.CheckBoard(boardID); err != nil {
		return CompleteItemResult{}, err
	}
	if !result.AlreadyDone {
		if _, err := s.port.UpdateItemValues(ctx, boardID, item.ID, plan.Normalized); err != nil {
			return CompleteItemResult{}, err
		}
		result.Updated = true
	}
	if moveGroupID != "" {
		if _, err := s.port.MoveItem(ctx, item.ID, moveGroupID); err != nil {
			return CompleteItemResult{}, err
		}
		result.MovedGroupID = moveGroupID
	}
	if strings.TrimSpace(updateBody) != "" {
		if _, err := s.port.CreateUpdate(ctx, item.ID, monday.FormatUpdateBody(updateBody), ""); err != nil {
			return CompleteItemResult{}, err
		}
		result.UpdatePosted = true
	}
	if warning := domain.UnreadableColumnsWarning(domain.UnreadableColumns([]domain.Item{*item}, statusColumnID)); warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	return result, nil
}

// HandoffItemResult reports a reassignment workflow.
type HandoffItemResult struct {
	DryRun       bool     `json:"dry_run"`
	ItemID       string   `json:"item_id"`
	Name         string   `json:"name"`
	PeopleColumn string   `json:"people_column_id,omitempty"`
	Assignees    []int64  `json:"assignees"`
	StatusSet    bool     `json:"status_set"`
	UpdatePosted bool     `json:"update_posted"`
	Warnings     []string `json:"warnings,omitempty"`
}

// HandoffItem transfers ownership by assigning people, optionally setting a
// workflow status, and posting a contextual update. Assignee IDs are monday
// user IDs; use list_users/search_users to resolve them first.
func (s *Service) HandoffItem(ctx context.Context, itemID, peopleColumnID string, assignees []int64, statusColumnID, statusValue, updateBody string, dryRun bool) (HandoffItemResult, error) {
	if err := requireID("item_id", itemID); err != nil {
		return HandoffItemResult{}, err
	}
	if len(assignees) == 0 || len(assignees) > 20 {
		return HandoffItemResult{}, invalid("assignees must contain between 1 and 20 monday user IDs")
	}
	for _, assignee := range assignees {
		if assignee <= 0 {
			return HandoffItemResult{}, invalid("assignees must be positive monday user IDs; resolve them with list_users or search_users")
		}
	}
	if (statusColumnID == "") != (statusValue == "") {
		return HandoffItemResult{}, invalid("status_column_id and status_value must be provided together")
	}
	item, err := s.GetItem(ctx, itemID)
	if err != nil {
		return HandoffItemResult{}, err
	}
	boardID := item.BoardID
	columns, err := s.port.ListColumns(ctx, boardID)
	if err != nil {
		return HandoffItemResult{}, err
	}
	if peopleColumnID == "" {
		peopleColumnID = domain.DetectReportColumns(columns, domain.ReportColumns{}).PeopleColumnID
	}
	if peopleColumnID == "" {
		return HandoffItemResult{}, invalid("no people column found; provide people_column_id")
	}
	peopleValues := make([]any, 0, len(assignees))
	for _, assignee := range assignees {
		peopleValues = append(peopleValues, assignee)
	}
	values := map[string]any{peopleColumnID: peopleValues}
	if statusColumnID != "" && statusValue != "" {
		values[statusColumnID] = statusValue
	}
	plan := domain.PlanColumnValues(columns, values)
	if err := plan.Err(); err != nil {
		return HandoffItemResult{}, invalid("handoff values are invalid: %v", err)
	}
	result := HandoffItemResult{DryRun: dryRun, ItemID: item.ID, Name: item.Name, PeopleColumn: peopleColumnID, Assignees: assignees, StatusSet: statusColumnID != "" && statusValue != "", UpdatePosted: strings.TrimSpace(updateBody) != ""}
	if dryRun {
		return result, nil
	}
	if err := s.guard.CheckBoard(boardID); err != nil {
		return HandoffItemResult{}, err
	}
	if _, err := s.port.UpdateItemValues(ctx, boardID, item.ID, plan.Normalized); err != nil {
		return HandoffItemResult{}, err
	}
	if strings.TrimSpace(updateBody) != "" {
		if _, err := s.port.CreateUpdate(ctx, item.ID, monday.FormatUpdateBody(updateBody), ""); err != nil {
			return HandoffItemResult{}, err
		}
	}
	if warning := domain.UnreadableColumnsWarning(domain.UnreadableColumns([]domain.Item{*item}, peopleColumnID, statusColumnID)); warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
	return result, nil
}
