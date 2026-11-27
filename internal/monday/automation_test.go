package monday

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jersonmartinez/mcp-monday-projects/internal/config"
)

func automationContract(t *testing.T, wantVersion, reply string) (*Client, *string, *map[string]any) {
	t.Helper()
	var query string
	variables := map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("API-Version"); got != wantVersion {
			t.Errorf("API-Version = %q, want %q", got, wantVersion)
		}
		body, _ := io.ReadAll(r.Body)
		var request GraphQLRequest
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("request JSON: %v", err)
		}
		query = request.Query
		variables = request.Variables
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	client := NewClient(config.Config{APIToken: "token", APIVersion: "2026-07", APIURL: server.URL, HTTPTimeout: time.Second, MaxResponseBytes: 1 << 20})
	return client, &query, &variables
}

func TestListAutomationsUsesVersionedAPIAndParsesPage(t *testing.T) {
	client, query, variables := automationContract(t, AutomationListVersion, `{"data":{"board_automations":{"cursor":"next","legacy_automations":[{"id":"legacy"}],"items":[{"id":"77","title":"When done close","active":true,"workflow_blocks":{"trigger":"status"}}]}}}`)
	page, err := client.ListAutomations(context.Background(), "100", 25, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Automations) != 1 || page.Automations[0].ID != "77" || !page.Automations[0].Active || page.Cursor != "next" || !page.HasMore {
		t.Fatalf("page = %+v", page)
	}
	if !strings.Contains(*query, "board_automations") || !strings.Contains(*query, "workflow_blocks") {
		t.Fatalf("query = %s", *query)
	}
	boardIDs := (*variables)["boardIDs"].([]any)
	if boardIDs[0] != "100" || (*variables)["limit"].(float64) != 25 {
		t.Fatalf("variables = %+v", *variables)
	}
}

func TestManageAutomationUsesPreviewMutationAndParsesSuccess(t *testing.T) {
	client, query, variables := automationContract(t, WorkflowPreviewVersion, `{"data":{"deactivate_live_workflow":{"is_success":true}}}`)
	ok, err := client.ManageAutomation(context.Background(), "deactivate", "100", "77")
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if !strings.Contains(*query, "deactivate_live_workflow") || (*variables)["id"] != "77" {
		t.Fatalf("query/variables = %s %+v", *query, *variables)
	}
}

func TestDeleteAutomationUsesStableMutationAndParsesSuccess(t *testing.T) {
	client, query, variables := automationContract(t, AutomationListVersion, `{"data":{"delete_board_automation":{"is_success":true}}}`)
	ok, err := client.ManageAutomation(context.Background(), "delete", "100", "77")
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if !strings.Contains(*query, "delete_board_automation") || (*variables)["id"] != "77" || (*variables)["boardID"] != "100" {
		t.Fatalf("query/variables = %s %+v", *query, *variables)
	}
}

func TestCreateAutomationUsesPreviewMutationAndInput(t *testing.T) {
	client, query, variables := automationContract(t, WorkflowPreviewVersion, `{"data":{"create_board_automation":{"workflow_id":"88"}}}`)
	workflowID, err := client.CreateAutomation(context.Background(), CreateAutomationInput{
		BoardID:                 "100",
		Title:                   "Close completed items",
		Description:             "Created from a template",
		TemplateReferenceID:     "123",
		WorkflowVariablesValues: map[string]any{"status": "Done"},
	})
	if err != nil || workflowID != "88" {
		t.Fatalf("workflowID = %q, err = %v", workflowID, err)
	}
	if !strings.Contains(*query, "create_board_automation") {
		t.Fatalf("query = %s", *query)
	}
	input := (*variables)["input"].(map[string]any)
	if input["board_id"] != "100" || input["template_reference_id"] != "123" || input["workflow_variables_values"].(map[string]any)["status"] != "Done" {
		t.Fatalf("input variables = %+v", input)
	}
	if _, ok := input["workflow_blocks"]; ok {
		t.Fatalf("template creation must not send workflow_blocks: %+v", input)
	}
}

func TestCreateAutomationDirectModeSendsWorkflowJSON(t *testing.T) {
	client, _, variables := automationContract(t, WorkflowPreviewVersion, `{"data":{"create_board_automation":{"workflow_id":"89"}}}`)
	workflowID, err := client.CreateAutomation(context.Background(), CreateAutomationInput{
		BoardID:           "100",
		Title:             "Direct workflow",
		WorkflowBlocks:    []any{map[string]any{"workflowNodeId": 1, "blockReferenceId": 2}},
		WorkflowVariables: map[string]any{"1": map[string]any{"type": "text"}},
	})
	if err != nil || workflowID != "89" {
		t.Fatalf("workflowID = %q, err = %v", workflowID, err)
	}
	input := (*variables)["input"].(map[string]any)
	if input["workflow_variables"].(map[string]any)["1"].(map[string]any)["type"] != "text" {
		t.Fatalf("direct input variables = %+v", input)
	}
	blocks, ok := input["workflow_blocks"].([]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("direct workflow_blocks = %#v", input["workflow_blocks"])
	}
	if _, exists := input["template_reference_id"]; exists {
		t.Fatalf("direct creation must not send template_reference_id: %+v", input)
	}
}

func TestAutomationStatisticsAndRunsDecodeProviderShapes(t *testing.T) {
	client, _, _ := automationContract(t, AutomationStatisticsVersion, `{"data":{"account_trigger_statistics":{"success":8,"failure":2,"total":10}}}`)
	stats, err := client.AutomationTotals(context.Background(), "100", []string{"1"})
	if err != nil || stats.Success != 8 || stats.Total != 10 {
		t.Fatalf("stats = %+v, err = %v", stats, err)
	}

	client2, query, _ := automationContract(t, "2026-07", `{"data":{"trigger_events":{"triggerEvents":[{"triggerUuid":"run-1","eventState":"success"},{"triggerUuid":"run-2","eventState":"failure","errorReason":"example"}]}}}`)
	runs, err := client2.ListAutomationRuns(context.Background(), "100", 0, []string{"success"}, nil, "500")
	if err != nil || runs.Count != 2 || runs.StateCounts["failure"] != 1 {
		t.Fatalf("runs = %+v, err = %v", runs, err)
	}
	if !strings.Contains(*query, "trigger_events") {
		t.Fatalf("runs query = %s", *query)
	}
}

func TestAssetQueriesDecodeMetadata(t *testing.T) {
	client, query, _ := automationContract(t, "2026-07", `{"data":{"items":[{"id":"500","assets":[{"id":"a1","name":"spec.pdf","public_url":"https://public","url_thumbnail":"https://thumb","file_extension":"pdf","file_size":123,"uploaded_by":{"id":"1","name":"Ana"}}]}]}}`)
	assets, err := client.ListItemAssets(context.Background(), "500", "all", nil)
	if err != nil || len(assets) != 1 || assets[0].FileSize != 123 || assets[0].UploadedBy.Name != "Ana" {
		t.Fatalf("assets = %+v, err = %v", assets, err)
	}
	if !strings.Contains(*query, "assets(assets_source") {
		t.Fatalf("asset query = %s", *query)
	}
}

func TestUpdateAssetsAndAutomationEntityStatistics(t *testing.T) {
	client, query, variables := automationContract(t, "2026-07", `{"data":{"updates":[{"id":"77","assets":[{"id":"a2","name":"trace.log","file_extension":"log","file_size":42}]}]}}`)
	assets, err := client.ListUpdateAssets(context.Background(), "77")
	if err != nil || len(assets) != 1 || assets[0].Name != "trace.log" {
		t.Fatalf("update assets = %+v, err = %v", assets, err)
	}
	if !strings.Contains(*query, "updates(ids: $ids)") {
		t.Fatalf("update asset query = %s", *query)
	}
	_ = variables

	client2, query2, variables2 := automationContract(t, AutomationStatisticsVersion, `{"data":{"account_triggers_statistics_by_entity_id":{"id":"77","automation_statistics":{"77":2}}}}`)
	entity, err := client2.AutomationEntityTotals(context.Background(), "failure", "100", []string{"1"}, []string{"88"})
	if err != nil || entity.ID != "77" {
		t.Fatalf("entity stats = %+v, err = %v", entity, err)
	}
	if !strings.Contains(*query2, "account_triggers_statistics_by_entity_id") {
		t.Fatalf("entity query = %s", *query2)
	}
	filters := (*variables2)["filters"].(map[string]any)
	if filters["board_id"] != float64(100) || filters["automation_ids"].([]any)[0] != float64(88) {
		t.Fatalf("entity variables = %+v", *variables2)
	}
}
