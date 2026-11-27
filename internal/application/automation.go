package application

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/jersonmartinez/mcp-monday-projects/internal/domain"
	"github.com/jersonmartinez/mcp-monday-projects/internal/monday"
)

// AutomationCreationInput configures native automation creation. The workflow
// fields accept decoded JSON values because monday's preview block schema is
// provider-versioned; the application layer validates shape combinations and
// bounded size, while monday remains the authoritative schema validator.
type AutomationCreationInput struct {
	BoardID                 string `json:"board_id"`
	Title                   string `json:"title"`
	Description             string `json:"description,omitempty"`
	TemplateReferenceID     string `json:"template_reference_id,omitempty"`
	WorkflowBlocks          any    `json:"workflow_blocks,omitempty"`
	WorkflowVariables       any    `json:"workflow_variables,omitempty"`
	WorkflowVariablesValues any    `json:"workflow_variables_values,omitempty"`
}

// AutomationCreationResult reports a native automation creation or dry-run plan.
type AutomationCreationResult struct {
	DryRun     bool     `json:"dry_run"`
	Created    bool     `json:"created"`
	Mode       string   `json:"mode"`
	BoardID    string   `json:"board_id"`
	Title      string   `json:"title"`
	WorkflowID string   `json:"workflow_id,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
}

func emptyAutomationJSON(value any) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	}
	return false
}

func automationPayloadBytes(values ...any) (int, error) {
	total := 0
	for _, value := range values {
		if value == nil {
			continue
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return 0, invalid("workflow JSON values must be serializable: %v", err)
		}
		total += len(raw)
	}
	return total, nil
}

var embeddedAutomationSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`\b(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
}

func containsEmbeddedAutomationSecret(value string) bool {
	for _, pattern := range embeddedAutomationSecretPatterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func sensitiveAutomationKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(key))
	for _, marker := range []string{"password", "passwd", "secret", "token", "api_key", "apikey", "access_key", "private_key", "authorization"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func automationReferenceValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}
	digitsOnly := true
	for _, char := range trimmed {
		if char < '0' || char > '9' {
			digitsOnly = false
			break
		}
	}
	if digitsOnly || strings.HasPrefix(trimmed, "$") || strings.HasPrefix(trimmed, "ref:") || strings.HasPrefix(trimmed, "connection:") {
		return true
	}
	if strings.Contains(trimmed, "{{") && strings.Contains(trimmed, "}}") {
		return true
	}
	if len(trimmed) == 36 && trimmed[8] == '-' && trimmed[13] == '-' && trimmed[18] == '-' && trimmed[23] == '-' {
		hexOnly := true
		for index, char := range trimmed {
			if index == 8 || index == 13 || index == 18 || index == 23 {
				continue
			}
			if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
				hexOnly = false
				break
			}
		}
		return hexOnly
	}
	return false
}

func scanAutomationJSONSecrets(value any, key string, rejectAssignments bool) error {
	switch typed := value.(type) {
	case string:
		if containsEmbeddedAutomationSecret(typed) {
			return invalid("workflow JSON contains embedded credential material; use monday workflow variables or credential references instead")
		}
		if rejectAssignments && sensitiveAutomationKey(key) && !automationReferenceValue(typed) {
			return invalid("workflow JSON field %q appears to embed a credential value; use a monday workflow variable or credential reference instead", key)
		}
	case []any:
		for _, child := range typed {
			if err := scanAutomationJSONSecrets(child, key, rejectAssignments); err != nil {
				return err
			}
		}
	case map[string]any:
		for childKey, child := range typed {
			if err := scanAutomationJSONSecrets(child, childKey, rejectAssignments); err != nil {
				return err
			}
		}
	}
	return nil
}

// CreateAutomation validates and creates a native board automation. Creation
// uses monday's preview workflow API, so it is dry-run-first and deliberately
// requires callers to set dry_run=false before activating a rule that can cause
// future unattended writes.
func (s *Service) CreateAutomation(ctx context.Context, input AutomationCreationInput, dryRun bool) (AutomationCreationResult, error) {
	if err := requireID("board_id", input.BoardID); err != nil {
		return AutomationCreationResult{}, err
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if input.Title == "" || len(input.Title) > 160 {
		return AutomationCreationResult{}, invalid("title is required and must be at most 160 characters")
	}
	if len(input.Description) > 1000 {
		return AutomationCreationResult{}, invalid("description must be at most 1000 characters")
	}
	hasTemplate := strings.TrimSpace(input.TemplateReferenceID) != ""
	hasBlocks := !emptyAutomationJSON(input.WorkflowBlocks)
	hasVariables := !emptyAutomationJSON(input.WorkflowVariables)
	hasValues := !emptyAutomationJSON(input.WorkflowVariablesValues)
	mode := "direct"
	if hasTemplate {
		mode = "template"
		if err := requireID("template_reference_id", input.TemplateReferenceID); err != nil {
			return AutomationCreationResult{}, err
		}
		if hasBlocks || hasVariables {
			return AutomationCreationResult{}, invalid("template_reference_id cannot be combined with workflow_blocks or workflow_variables")
		}
	} else {
		if !hasBlocks || !hasVariables {
			return AutomationCreationResult{}, invalid("direct creation requires non-empty workflow_blocks and workflow_variables; template creation requires template_reference_id")
		}
		if hasValues {
			return AutomationCreationResult{}, invalid("workflow_variables_values applies only to template-based creation")
		}
	}
	size, err := automationPayloadBytes(input.WorkflowBlocks, input.WorkflowVariables, input.WorkflowVariablesValues)
	if err != nil {
		return AutomationCreationResult{}, err
	}
	if size > 64*1024 {
		return AutomationCreationResult{}, invalid("workflow definition is too large; keep workflow JSON under 64 KiB")
	}
	for _, workflowJSON := range []any{input.WorkflowBlocks, input.WorkflowVariables, input.WorkflowVariablesValues} {
		if err := scanAutomationJSONSecrets(workflowJSON, "", false); err != nil {
			return AutomationCreationResult{}, err
		}
	}
	// Variable definitions may legitimately declare credential variables, but
	// executable blocks and template variable assignments must not carry raw
	// credential values. Numeric/UUID strings and explicit reference syntax are
	// treated as references; opaque strings are refused conservatively.
	for _, workflowJSON := range []any{input.WorkflowBlocks, input.WorkflowVariablesValues} {
		if err := scanAutomationJSONSecrets(workflowJSON, "", true); err != nil {
			return AutomationCreationResult{}, err
		}
	}
	if _, err := s.port.GetBoard(ctx, input.BoardID); err != nil {
		return AutomationCreationResult{}, err
	}
	result := AutomationCreationResult{
		DryRun:  dryRun,
		Mode:    mode,
		BoardID: input.BoardID,
		Title:   input.Title,
		Warnings: []string{
			"native automation creation uses monday's preview workflow API; verify the created rule with list_automations",
			"workflow definitions must reference credentials through monday workflow variables; never embed secret values",
		},
	}
	if dryRun {
		return result, nil
	}
	if err := s.guard.CheckBoard(input.BoardID); err != nil {
		return AutomationCreationResult{}, err
	}
	workflowID, err := s.port.CreateAutomation(ctx, monday.CreateAutomationInput{
		BoardID:                 input.BoardID,
		Title:                   input.Title,
		Description:             input.Description,
		TemplateReferenceID:     input.TemplateReferenceID,
		WorkflowBlocks:          input.WorkflowBlocks,
		WorkflowVariables:       input.WorkflowVariables,
		WorkflowVariablesValues: input.WorkflowVariablesValues,
	})
	if err != nil {
		return AutomationCreationResult{}, err
	}
	if workflowID == "" {
		return AutomationCreationResult{}, invalid("monday returned an empty workflow_id")
	}
	result.Created = true
	result.WorkflowID = workflowID
	return result, nil
}

// ListAutomations returns a page of native board automations. monday exposes
// this query from API version 2026-10; the transport requests that version for
// this operation only. Some legacy automations are read-only provider data.
func (s *Service) ListAutomations(ctx context.Context, boardID string, limit int, cursor string) (domain.AutomationPage, error) {
	if err := requireID("board_id", boardID); err != nil {
		return domain.AutomationPage{}, err
	}
	limit, err := BoundLimit(limit, 100, 100)
	if err != nil {
		return domain.AutomationPage{}, err
	}
	return s.port.ListAutomations(ctx, boardID, limit, cursor, cursor == "")
}

// ManageAutomation controls a native live workflow. Activate/deactivate are
// write operations; delete is permanent, requires full access through
// RequireDelete, and also requires confirm in the MCP input.
func (s *Service) verifyAutomationOwnership(ctx context.Context, boardID, automationID string) error {
	// Verify the automation belongs to the requested board before mutating it;
	// automation IDs are otherwise account-wide opaque identifiers. Walk the
	// bounded cursor chain because boards can have more than one automation page.
	cursor := ""
	for pageNumber := 0; pageNumber < 10; pageNumber++ {
		page, err := s.port.ListAutomations(ctx, boardID, 100, cursor, false)
		if err != nil {
			return err
		}
		for _, automation := range page.Automations {
			if automation.ID == automationID {
				return nil
			}
		}
		if !page.HasMore || page.Cursor == "" || page.Cursor == cursor {
			break
		}
		cursor = page.Cursor
	}
	return invalid("automation_id %s was not found on board %s; call list_automations first and do not guess IDs", automationID, boardID)
}

// ManageAutomation activates or deactivates a native live workflow after
// verifying board ownership. Permanent deletion is intentionally a separate
// full-access tool, mirroring the rest of the delete contract.
func (s *Service) ManageAutomation(ctx context.Context, boardID, action, automationID string) (bool, error) {
	if err := requireID("board_id", boardID); err != nil {
		return false, err
	}
	if err := requireID("automation_id", automationID); err != nil {
		return false, err
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if err := oneOf("action", action, "activate", "deactivate"); err != nil || action == "" {
		return false, invalid("action must be activate or deactivate")
	}
	if err := s.guard.CheckBoard(boardID); err != nil {
		return false, err
	}
	if err := s.verifyAutomationOwnership(ctx, boardID, automationID); err != nil {
		return false, err
	}
	return s.port.ManageAutomation(ctx, action, boardID, automationID)
}

// DeleteAutomation permanently deletes a native automation only after the
// generic full-access/confirm gate and board-ownership proof. Deactivation is
// the preferred reversible control.
func (s *Service) DeleteAutomation(ctx context.Context, boardID, automationID string, confirm bool) (bool, error) {
	if err := requireID("board_id", boardID); err != nil {
		return false, err
	}
	if err := requireID("automation_id", automationID); err != nil {
		return false, err
	}
	if err := s.guard.CheckBoard(boardID); err != nil {
		return false, err
	}
	if err := s.requireDelete("automation", "deactivate", confirm); err != nil {
		return false, err
	}
	if err := s.verifyAutomationOwnership(ctx, boardID, automationID); err != nil {
		return false, err
	}
	return s.port.ManageAutomation(ctx, "delete", boardID, automationID)
}

// AutomationStatistics returns aggregate native automation outcomes. monday's
// statistics API is versioned independently and may be unavailable on accounts
// without the current workflow platform; provider errors are surfaced typed.
func (s *Service) AutomationStatistics(ctx context.Context, boardID, runStatus, breakdown string, userIDs, excludedAutomationIDs []string) (any, error) {
	if boardID != "" {
		if err := requireID("board_id", boardID); err != nil {
			return nil, err
		}
	}
	breakdown = strings.ToLower(strings.TrimSpace(breakdown))
	if breakdown == "" {
		breakdown = "totals"
	}
	if err := oneOf("breakdown", breakdown, "totals", "by_entity"); err != nil {
		return nil, err
	}
	if breakdown == "by_entity" {
		if err := oneOf("run_status", runStatus, "success", "failure", "exhausted"); err != nil || runStatus == "" {
			return nil, invalid("run_status is required for by_entity and must be success, failure, or exhausted")
		}
		return s.port.AutomationEntityTotals(ctx, runStatus, boardID, userIDs, excludedAutomationIDs)
	}
	return s.port.AutomationTotals(ctx, boardID, userIDs)
}

// ListAutomationRuns returns one offset page of native workflow execution history.
func (s *Service) ListAutomationRuns(ctx context.Context, boardID string, offset int, states, automationIDs []string, itemID string) (domain.AutomationRunPage, error) {
	if boardID != "" {
		if err := requireID("board_id", boardID); err != nil {
			return domain.AutomationRunPage{}, err
		}
	}
	if offset < 0 || offset > 100000 {
		return domain.AutomationRunPage{}, invalid("offset must be between 0 and 100000")
	}
	for _, state := range states {
		if err := oneOf("states", state, "success", "failure", "exhausted"); err != nil {
			return domain.AutomationRunPage{}, err
		}
	}
	if itemID != "" {
		if err := requireID("item_id", itemID); err != nil {
			return domain.AutomationRunPage{}, err
		}
	}
	return s.port.ListAutomationRuns(ctx, boardID, offset, states, automationIDs, itemID)
}
