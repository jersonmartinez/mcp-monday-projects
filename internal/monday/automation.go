package monday

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jersonmartinez/mcp-monday-projects/internal/domain"
)

const (
	// AutomationListVersion is the first monday API version exposing board_automations.
	AutomationListVersion = "2026-10"
	// WorkflowPreviewVersion exposes workflow lifecycle mutations while monday keeps them in preview.
	WorkflowPreviewVersion = "dev"
	// AutomationStatisticsVersion is the stable schema used for Int-based statistics filters.
	AutomationStatisticsVersion = "2026-07"
	// AutomationRunsVersion is the stable schema whose trigger-event filters use automationIds.
	AutomationRunsVersion = "2026-07"
)

const boardAutomationsQuery = `query BoardAutomations($boardIDs: [ID!], $limit: Int, $cursor: String, $includeLegacy: Boolean!) {
  board_automations(board_ids: $boardIDs, limit: $limit, cursor: $cursor) {
    cursor
    legacy_automations @include(if: $includeLegacy)
    items {
      id user_id active title description created_at updated_at
      workflow_host_data workflow_blocks workflow_variables
      importance notice_message template_reference_id
    }
  }
}`

const automationStatisticsQuery = `query AutomationStatistics($filters: AccountTriggerStatisticsFiltersInput) {
  account_trigger_statistics(filters: $filters) { id success failure total }
}`

const automationEntityStatisticsQuery = `query AutomationEntityStatistics($runStatus: TriggerEventState!, $filters: AccountTriggersByEntityIdFiltersInput) {
  account_triggers_statistics_by_entity_id(run_status: $runStatus, filters: $filters) {
    id automation_statistics workflow_statistics
  }
}`

const automationRunsQuery = `query AutomationRuns($nextPageOffset: Int, $filters: TriggerEventsFiltersInput) {
  trigger_events(nextPageOffset: $nextPageOffset, filters: $filters) {
    triggerEvents {
      triggerUuid eventState eventKind triggerStartedAt createdAt triggerDuration errorReason
      entityKind hostType hostInstanceId billingActionsCount creatorAppFeatureReferenceId
      waitingForTriggerName reignitionSubscriptionId
    }
  }
}`

// ListAutomations lists board automations. It deliberately requests the API
// version where board_automations exists instead of requiring the whole server
// to move away from its stable default.
func (c *Client) ListAutomations(ctx context.Context, boardID string, limit int, cursor string, includeLegacy bool) (domain.AutomationPage, error) {
	var data struct {
		Page struct {
			Cursor *string             `json:"cursor"`
			Legacy json.RawMessage     `json:"legacy_automations"`
			Items  []domain.Automation `json:"items"`
		} `json:"board_automations"`
	}
	err := c.DoWithVersion(ctx, boardAutomationsQuery, map[string]any{
		"boardIDs": []string{boardID}, "limit": limit, "cursor": cursor, "includeLegacy": includeLegacy,
	}, &data, AutomationListVersion)
	if err != nil {
		return domain.AutomationPage{}, err
	}
	page := domain.AutomationPage{Automations: data.Page.Items}
	if data.Page.Cursor != nil {
		page.Cursor = *data.Page.Cursor
		page.HasMore = *data.Page.Cursor != ""
	}
	if includeLegacy && len(data.Page.Legacy) > 0 && string(data.Page.Legacy) != "null" {
		var legacy any
		if err := json.Unmarshal(data.Page.Legacy, &legacy); err == nil {
			page.Legacy = legacy
		}
	}
	return page, nil
}

// intFilter converts monday's numeric string IDs to GraphQL Int input values.
// The automation statistics and trigger-event filters are Int-based in the
// stable schema, unlike most modern monday ID arguments.
func intFilter(name, value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive numeric monday ID", name)
	}
	return parsed, nil
}

func intFilterList(name string, values []string) ([]int, error) {
	out := make([]int, 0, len(values))
	for _, value := range values {
		parsed, err := intFilter(name, value)
		if err != nil {
			return nil, err
		}
		out = append(out, parsed)
	}
	return out, nil
}

// CreateAutomationInput is the provider-shaped input for preview automation creation.
// Workflow fields are deliberately JSON-valued because monday's block schema is
// versioned independently and is validated authoritatively by the provider.
type CreateAutomationInput struct {
	BoardID                 string
	Title                   string
	Description             string
	TemplateReferenceID     string
	WorkflowBlocks          any
	WorkflowVariables       any
	WorkflowVariablesValues any
}

// CreateAutomation creates a native board automation through monday's preview
// workflow API. Template-based creation needs template_reference_id; direct
// creation needs workflow_blocks plus workflow_variables.
func (c *Client) CreateAutomation(ctx context.Context, input CreateAutomationInput) (string, error) {
	payload := map[string]any{
		"board_id": input.BoardID,
		"title":    input.Title,
	}
	if input.Description != "" {
		payload["description"] = input.Description
	}
	if input.TemplateReferenceID != "" {
		payload["template_reference_id"] = input.TemplateReferenceID
		if input.WorkflowVariablesValues != nil {
			payload["workflow_variables_values"] = input.WorkflowVariablesValues
		}
	} else {
		payload["workflow_blocks"] = input.WorkflowBlocks
		payload["workflow_variables"] = input.WorkflowVariables
	}
	var data struct {
		Result struct {
			WorkflowID string `json:"workflow_id"`
		} `json:"create_board_automation"`
	}
	err := c.DoWithVersion(ctx, `mutation CreateBoardAutomation($input: BoardAutomationCreateInput!) {
  create_board_automation(input: $input) { workflow_id }
}`, map[string]any{"input": payload}, &data, WorkflowPreviewVersion)
	if err != nil {
		return "", err
	}
	if data.Result.WorkflowID == "" {
		return "", fmt.Errorf("monday returned an incomplete create_board_automation response")
	}
	return data.Result.WorkflowID, nil
}

// ManageAutomation activates or deactivates a live workflow through monday's
// preview workflow mutations. Delete uses the stable board-automation mutation
// introduced with board_automations and requires the owning board ID.
func (c *Client) ManageAutomation(ctx context.Context, action, boardID, automationID string) (bool, error) {
	if action == "delete" {
		var data struct {
			Result struct {
				IsSuccess bool `json:"is_success"`
			} `json:"delete_board_automation"`
		}
		err := c.DoWithVersion(ctx, `mutation DeleteBoardAutomation($id: ID!, $boardID: ID!) {
  delete_board_automation(id: $id, board_id: $boardID) { is_success }
}`, map[string]any{"id": automationID, "boardID": boardID}, &data, AutomationListVersion)
		if err != nil {
			return false, err
		}
		return data.Result.IsSuccess, nil
	}
	mutationName := map[string]string{
		"activate":   "activate_live_workflow",
		"deactivate": "deactivate_live_workflow",
	}[action]
	if mutationName == "" {
		return false, fmt.Errorf("unsupported automation action %q", action)
	}
	query := fmt.Sprintf(`mutation ManageAutomation($id: ID!) {
  %s(id: $id) { is_success }
}`, mutationName)
	var data map[string]struct {
		IsSuccess bool `json:"is_success"`
	}
	if err := c.DoWithVersion(ctx, query, map[string]any{"id": automationID}, &data, WorkflowPreviewVersion); err != nil {
		return false, err
	}
	result, ok := data[mutationName]
	if !ok {
		return false, fmt.Errorf("monday returned incomplete %s response", mutationName)
	}
	return result.IsSuccess, nil
}

// AutomationTotals returns aggregate automation outcomes for a board or account.
func (c *Client) AutomationTotals(ctx context.Context, boardID string, userIDs []string) (domain.AutomationStatistics, error) {
	filters := map[string]any{}
	if boardID != "" {
		id, err := intFilter("board_id", boardID)
		if err != nil {
			return domain.AutomationStatistics{}, err
		}
		filters["board_id"] = id
	}
	if len(userIDs) > 0 {
		ids, err := intFilterList("user_ids", userIDs)
		if err != nil {
			return domain.AutomationStatistics{}, err
		}
		filters["user_ids"] = ids
	}
	var data struct {
		Statistics domain.AutomationStatistics `json:"account_trigger_statistics"`
	}
	if err := c.DoWithVersion(ctx, automationStatisticsQuery, map[string]any{"filters": filters}, &data, AutomationStatisticsVersion); err != nil {
		return domain.AutomationStatistics{}, err
	}
	return data.Statistics, nil
}

// AutomationEntityTotals returns per-entity automation outcomes for one run state.
func (c *Client) AutomationEntityTotals(ctx context.Context, runStatus, boardID string, userIDs, excludedAutomationIDs []string) (domain.AutomationEntityStatistics, error) {
	filters := map[string]any{}
	if boardID != "" {
		id, err := intFilter("board_id", boardID)
		if err != nil {
			return domain.AutomationEntityStatistics{}, err
		}
		filters["board_id"] = id
	}
	if len(userIDs) > 0 {
		ids, err := intFilterList("user_ids", userIDs)
		if err != nil {
			return domain.AutomationEntityStatistics{}, err
		}
		filters["user_ids"] = ids
	}
	if len(excludedAutomationIDs) > 0 {
		ids, err := intFilterList("exclude_automation_ids", excludedAutomationIDs)
		if err != nil {
			return domain.AutomationEntityStatistics{}, err
		}
		filters["automation_ids"] = ids
	}
	var data struct {
		Statistics domain.AutomationEntityStatistics `json:"account_triggers_statistics_by_entity_id"`
	}
	err := c.DoWithVersion(ctx, automationEntityStatisticsQuery, map[string]any{"runStatus": runStatus, "filters": filters}, &data, AutomationStatisticsVersion)
	if err != nil {
		return domain.AutomationEntityStatistics{}, err
	}
	return data.Statistics, nil
}

// ListAutomationRuns returns one offset page of workflow/automation trigger events.
func (c *Client) ListAutomationRuns(ctx context.Context, boardID string, offset int, states []string, automationIDs []string, itemID string) (domain.AutomationRunPage, error) {
	filters := map[string]any{}
	if boardID != "" {
		filters["boardId"] = boardID
	}
	if len(states) > 0 {
		filters["stateFilter"] = states
	}
	if len(automationIDs) > 0 {
		ids, err := intFilterList("automation_ids", automationIDs)
		if err != nil {
			return domain.AutomationRunPage{}, err
		}
		// The Int-based camelCase field is the one present in the stable 2026-07
		// schema. snake_case replacements arrive only in later versions.
		filters["automationIds"] = ids
	}
	if itemID != "" {
		filters["itemId"] = itemID
	}
	var data struct {
		Events struct {
			Runs []domain.AutomationRun `json:"triggerEvents"`
		} `json:"trigger_events"`
	}
	err := c.DoWithVersion(ctx, automationRunsQuery, map[string]any{"nextPageOffset": offset, "filters": filters}, &data, AutomationRunsVersion)
	if err != nil {
		return domain.AutomationRunPage{}, err
	}
	counts := map[string]int{}
	for _, run := range data.Events.Runs {
		if run.EventState != "" {
			counts[run.EventState]++
		}
	}
	return domain.AutomationRunPage{Runs: data.Events.Runs, Offset: offset, Count: len(data.Events.Runs), StateCounts: counts}, nil
}
