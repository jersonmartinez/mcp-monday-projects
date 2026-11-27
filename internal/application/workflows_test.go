package application_test

import (
	"strings"
	"testing"

	"github.com/jersonmartinez/mcp-monday-projects/internal/application"
	"github.com/jersonmartinez/mcp-monday-projects/internal/domain"
)

func TestBulkCreateAndImportValidateBeforeWriting(t *testing.T) {
	svc, port := newService(application.NewWriteGuard(false, nil, nil))
	plan, err := svc.BulkCreateItems(ctx, "100", []application.BulkCreateRow{{Name: "One", GroupID: "todo", ColumnValues: map[string]any{"status": "Done"}}}, true)
	if err != nil || !plan.DryRun || plan.Planned != 1 {
		t.Fatalf("dry-run plan = %+v, err = %v", plan, err)
	}
	if port.MutationCount() != 0 {
		t.Fatalf("dry-run wrote mutations: %v", port.Mutations)
	}
	applied, err := svc.BulkCreateItems(ctx, "100", []application.BulkCreateRow{{Name: "One", GroupID: "To Do", ColumnValues: map[string]any{"status": "Done"}}}, false)
	if err != nil || applied.Succeeded != 1 || applied.Rows[0].ItemID == "" {
		t.Fatalf("applied = %+v, err = %v", applied, err)
	}
	invalid, err := svc.ImportBoardTable(ctx, "100", "csv", "name,status\nA,Done\nA,Done", true)
	if err != nil {
		t.Fatal(err)
	}
	if invalid.Failed != 2 || !strings.Contains(invalid.Rows[0].Error, "duplicate") {
		t.Fatalf("duplicate import result = %+v", invalid)
	}
}

func TestCreateAutomationIsDryRunFirstAndValidatesModes(t *testing.T) {
	svc, port := newService(application.NewWriteGuard(false, nil, nil))
	input := application.AutomationCreationInput{BoardID: "100", Title: "Close completed items", TemplateReferenceID: "123", WorkflowVariablesValues: map[string]any{"status": "Done"}}
	plan, err := svc.CreateAutomation(ctx, input, true)
	if err != nil || !plan.DryRun || plan.Created || plan.Mode != "template" {
		t.Fatalf("creation plan = %+v, err = %v", plan, err)
	}
	if port.MutationCount() != 0 {
		t.Fatalf("creation dry-run wrote mutations: %v", port.Mutations)
	}
	created, err := svc.CreateAutomation(ctx, input, false)
	if err != nil || !created.Created || created.WorkflowID != "88" {
		t.Fatalf("created automation = %+v, err = %v", created, err)
	}
	if len(port.Mutations) != 1 || port.Mutations[0] != "create_automation" {
		t.Fatalf("creation mutations = %v", port.Mutations)
	}
	if _, err := svc.CreateAutomation(ctx, application.AutomationCreationInput{BoardID: "100", Title: "bad", TemplateReferenceID: "123", WorkflowBlocks: map[string]any{"trigger": true}}, true); err == nil {
		t.Fatal("template creation combined with workflow blocks must be rejected")
	}
	if _, err := svc.CreateAutomation(ctx, application.AutomationCreationInput{BoardID: "100", Title: "bad", WorkflowBlocks: map[string]any{"trigger": true}}, true); err == nil {
		t.Fatal("direct creation without workflow variables must be rejected")
	}
	if _, err := svc.CreateAutomation(ctx, application.AutomationCreationInput{BoardID: "100", Title: "bad", TemplateReferenceID: "abc"}, true); err == nil {
		t.Fatal("non-numeric template reference must be rejected")
	}
}

func TestCreateAutomationRejectsEmbeddedCredentialMaterial(t *testing.T) {
	svc, _ := newService(application.NewWriteGuard(false, nil, nil))
	validDirect := application.AutomationCreationInput{
		BoardID: "100",
		Title:   "Credential references",
		WorkflowBlocks: []any{map[string]any{"config": map[string]any{
			"token":         "123456",
			"authorization": "Bearer {{variables.api_token}}",
		}}},
		WorkflowVariables: map[string]any{"api_token": map[string]any{"type": "credential", "required": true}},
	}
	if _, err := svc.CreateAutomation(ctx, validDirect, true); err != nil {
		t.Fatalf("credential definitions and references must be accepted: %v", err)
	}
	badValues := application.AutomationCreationInput{BoardID: "100", Title: "bad", TemplateReferenceID: "123", WorkflowVariablesValues: map[string]any{"api_token": "opaque-secret-value"}}
	if _, err := svc.CreateAutomation(ctx, badValues, true); err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("opaque credential assignment err = %v", err)
	}
	badBlocks := validDirect
	badBlocks.WorkflowBlocks = []any{map[string]any{"config": map[string]any{"authorization": "Bearer opaque-secret-value"}}}
	if _, err := svc.CreateAutomation(ctx, badBlocks, true); err == nil || !strings.Contains(err.Error(), "authorization") {
		t.Fatalf("raw authorization header err = %v", err)
	}
	badKnownSecret := validDirect
	badKnownSecret.WorkflowVariables = map[string]any{"example": "ghp_" + strings.Repeat("x", 20)}
	if _, err := svc.CreateAutomation(ctx, badKnownSecret, true); err == nil || !strings.Contains(err.Error(), "embedded credential material") {
		t.Fatalf("known token pattern err = %v", err)
	}
	badPrivateKey := validDirect
	badPrivateKey.WorkflowVariables = map[string]any{"example": "-----BEGIN " + "PRIVATE KEY-----\nabc"}
	if _, err := svc.CreateAutomation(ctx, badPrivateKey, true); err == nil {
		t.Fatal("private key material must be rejected")
	}
}

func TestRunBoardAutomationIsDryRunFirstAndRowAudited(t *testing.T) {
	svc, port := newService(application.NewWriteGuard(false, nil, nil))
	rules := []application.AutomationRule{{Name: "empty status", ColumnID: "status", Operator: "empty", SetColumnValues: map[string]any{"status": "Done"}}}
	plan, err := svc.RunBoardAutomation(ctx, "100", rules, 10, true)
	if err != nil || plan.Matched != 1 || port.MutationCount() != 0 {
		t.Fatalf("plan = %+v, mutations = %v, err = %v", plan, port.Mutations, err)
	}
	applied, err := svc.RunBoardAutomation(ctx, "100", rules, 10, false)
	if err != nil || applied.Succeeded != 1 || applied.Rows[0].Status != "applied" {
		t.Fatalf("applied = %+v, err = %v", applied, err)
	}
	if port.MutationCount() != 1 || !strings.HasPrefix(port.Mutations[0], "update_item_values:500") {
		t.Fatalf("mutations = %v", port.Mutations)
	}
	if _, err := svc.RunBoardAutomation(ctx, "100", []application.AutomationRule{{ColumnID: "missing", Operator: "empty", SetColumnValues: map[string]any{"status": "Done"}}}, 10, true); err == nil {
		t.Fatal("unknown rule column must be rejected before scanning")
	}
}

func TestCompleteAndHandoffWorkflows(t *testing.T) {
	svc, port := newService(application.NewWriteGuard(false, nil, nil))
	preview, err := svc.CompleteItem(ctx, "500", "", "", "", "done", true)
	if err != nil || !preview.DryRun || !preview.Updated || port.MutationCount() != 0 {
		t.Fatalf("complete preview = %+v, mutations = %v, err = %v", preview, port.Mutations, err)
	}
	done, err := svc.CompleteItem(ctx, "500", "", "", "todo", "done", false)
	if err != nil || !done.Updated || done.MovedGroupID != "todo" || !done.UpdatePosted {
		t.Fatalf("complete result = %+v, err = %v", done, err)
	}
	handoff, err := svc.HandoffItem(ctx, "500", "", []int64{1}, "status", "Working on it", "handoff", false)
	if err != nil || !handoff.StatusSet || !handoff.UpdatePosted {
		t.Fatalf("handoff = %+v, err = %v", handoff, err)
	}
	if port.MutationCount() != 5 {
		t.Fatalf("complete+handoff mutations = %v", port.Mutations)
	}
}

func TestBoardFlowMetricsReportsExplicitProxies(t *testing.T) {
	svc, _ := newService(nil)
	metrics, err := svc.BoardFlowMetrics(ctx, "100", 30, domain.ReportColumns{})
	if err != nil {
		t.Fatal(err)
	}
	if metrics.TotalItems != 1 || metrics.OpenItems != 1 || metrics.BottleneckStatus == "" {
		t.Fatalf("metrics = %+v", metrics)
	}
	joined := strings.Join(metrics.Warnings, " ")
	if !strings.Contains(joined, "proxy") || !strings.Contains(joined, "approximations") {
		t.Fatalf("proxy warnings = %v", metrics.Warnings)
	}
}

func TestAssetsRedactURLsAndAutomationIsOwnershipChecked(t *testing.T) {
	svc, port := newService(application.NewWriteGuard(false, nil, nil))
	assets, err := svc.ListItemAssets(ctx, "500", "all", nil, false)
	if err != nil || len(assets) != 1 || assets[0].PublicURL != "" {
		t.Fatalf("assets = %+v, err = %v", assets, err)
	}
	if ok, err := svc.ManageAutomation(ctx, "100", "deactivate", "77"); err != nil || !ok {
		t.Fatalf("deactivate = %v, err = %v", ok, err)
	}
	if _, err := svc.ManageAutomation(ctx, "100", "deactivate", "999"); err == nil || !strings.Contains(err.Error(), "list_automations") {
		t.Fatalf("foreign automation err = %v", err)
	}
	if _, err := svc.DeleteAutomation(ctx, "100", "77", true); err == nil || !strings.Contains(err.Error(), "MCP_ACCESS_LEVEL=full") {
		t.Fatalf("delete without full access err = %v", err)
	}
	if len(port.Mutations) != 1 || port.Mutations[0] != "manage_automation_deactivate" {
		t.Fatalf("mutations = %v", port.Mutations)
	}
}

func TestDeleteAutomationRequiresConfirmationAndFullAccess(t *testing.T) {
	svc, port := deleteService(t, application.Options{AllowDelete: true})
	if _, err := svc.DeleteAutomation(ctx, "100", "77", false); err == nil || !strings.Contains(err.Error(), "confirm=true") {
		t.Fatalf("unconfirmed automation delete err = %v", err)
	}
	ok, err := svc.DeleteAutomation(ctx, "100", "77", true)
	if err != nil || !ok {
		t.Fatalf("automation delete = %v, err = %v", ok, err)
	}
	if len(port.Mutations) != 1 || port.Mutations[0] != "manage_automation_delete" {
		t.Fatalf("delete mutations = %v", port.Mutations)
	}
}

func TestManageAutomationFindsAutomationOnLaterPage(t *testing.T) {
	svc, port := newService(nil)
	port.AutomationPages = map[string]domain.AutomationPage{
		"":     {Automations: []domain.Automation{{ID: "70", Title: "first page"}}, Cursor: "next", HasMore: true},
		"next": {Automations: []domain.Automation{{ID: "77", Title: "target", Active: true}}},
	}
	ok, err := svc.ManageAutomation(ctx, "100", "deactivate", "77")
	if err != nil || !ok {
		t.Fatalf("paged deactivate = %v, err = %v", ok, err)
	}
	if len(port.Mutations) != 1 || port.Mutations[0] != "manage_automation_deactivate" {
		t.Fatalf("mutations = %v", port.Mutations)
	}
}

func TestAutomationStatisticsRunsAndUpdateAssets(t *testing.T) {
	svc, _ := newService(nil)
	page, err := svc.ListAutomations(ctx, "100", 25, "")
	if err != nil || len(page.Automations) != 1 {
		t.Fatalf("automations = %+v, err = %v", page, err)
	}
	totals, err := svc.AutomationStatistics(ctx, "100", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	stats, ok := totals.(domain.AutomationStatistics)
	if !ok || stats.Total != 10 {
		t.Fatalf("totals = %#v", totals)
	}
	entity, err := svc.AutomationStatistics(ctx, "100", "failure", "by_entity", []string{"1"}, []string{"88"})
	if err != nil || entity == nil {
		t.Fatalf("entity totals = %#v, err = %v", entity, err)
	}
	runs, err := svc.ListAutomationRuns(ctx, "100", 0, []string{"success", "failure"}, []string{"77"}, "500")
	if err != nil || runs.Count != 2 || runs.StateCounts["failure"] != 1 {
		t.Fatalf("runs = %+v, err = %v", runs, err)
	}
	assets, err := svc.ListUpdateAssets(ctx, "77", "500", false)
	if err != nil || len(assets) != 1 || assets[0].PublicURL != "" {
		t.Fatalf("update assets = %+v, err = %v", assets, err)
	}
	if _, err := svc.ListUpdateAssets(ctx, "999", "500", false); err == nil {
		t.Fatal("foreign update id must be rejected")
	}
}

func TestMarkdownImportRowsAreParsedThroughBulkPipeline(t *testing.T) {
	svc, port := newService(application.NewWriteGuard(false, nil, nil))
	markdown := "| name | group | status |\n|---|---|---|\n| Alpha | To Do | Done |\n| Beta | To Do | Working on it |\n"
	plan, err := svc.ImportBoardTable(ctx, "100", "markdown", markdown, true)
	if err != nil || plan.Planned != 2 || port.MutationCount() != 0 {
		t.Fatalf("markdown plan = %+v, mutations = %v, err = %v", plan, port.Mutations, err)
	}
	applied, err := svc.ImportBoardTable(ctx, "100", "markdown", markdown, false)
	if err != nil || applied.Succeeded != 2 {
		t.Fatalf("markdown apply = %+v, err = %v", applied, err)
	}
	if _, err := svc.ImportBoardTable(ctx, "100", "markdown", "| name |\n|---|\n", true); err == nil {
		t.Fatal("markdown without data rows must be rejected")
	}
}

func TestRunBoardAutomationOperatorsAndCompositeActions(t *testing.T) {
	svc, port := newService(application.NewWriteGuard(false, nil, nil))
	cell := func(id, text string) domain.ColumnValue {
		return domain.ColumnValue{ID: id, Text: text, Type: "status"}
	}
	port.Items["501"] = domain.Item{ID: "501", Name: "Critical", BoardID: "100", Group: &domain.Group{ID: "todo", Title: "To Do"}, ColumnValues: []domain.ColumnValue{cell("status", "Blocked critical")}}
	port.Items["502"] = domain.Item{ID: "502", Name: "Empty", BoardID: "100", Group: &domain.Group{ID: "todo", Title: "To Do"}, ColumnValues: []domain.ColumnValue{cell("status", "")}}
	port.Items["503"] = domain.Item{ID: "503", Name: "Ready", BoardID: "100", Group: &domain.Group{ID: "todo", Title: "To Do"}, ColumnValues: []domain.ColumnValue{cell("status", "Ready")}}
	port.Order = []string{"501", "502", "503"}
	rules := []application.AutomationRule{
		{Name: "critical", ColumnID: "status", Operator: "contains", Value: "critical", SetColumnValues: map[string]any{"status": "Done"}, MoveGroupID: "To Do", UpdateBody: "closed by automation"},
		{Name: "empty", ColumnID: "status", Operator: "empty", SetColumnValues: map[string]any{"status": "Done"}},
		{Name: "ready", ColumnID: "status", Operator: "not_empty", SetColumnValues: map[string]any{"status": "Done"}},
	}
	result, err := svc.RunBoardAutomation(ctx, "100", rules, 10, false)
	if err != nil || result.Matched != 3 || result.Succeeded != 3 || result.Failed != 0 {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if port.MutationCount() != 5 { // first rule writes values + move + update; the other two write values
		t.Fatalf("mutations = %v", port.Mutations)
	}
}

func TestAutomationAndImportValidationBranches(t *testing.T) {
	svc, port := newService(application.NewWriteGuard(false, nil, nil))
	if _, err := svc.ListAutomations(ctx, "100", 101, ""); err == nil {
		t.Fatal("automation limit must be bounded")
	}
	if _, err := svc.AutomationStatistics(ctx, "100", "", "by_entity", nil, nil); err == nil {
		t.Fatal("by_entity statistics require a run status")
	}
	if _, err := svc.AutomationStatistics(ctx, "100", "", "invalid", nil, nil); err == nil {
		t.Fatal("unknown statistics breakdown must be rejected")
	}
	if _, err := svc.ListAutomationRuns(ctx, "100", -1, nil, nil, ""); err == nil {
		t.Fatal("negative run offset must be rejected")
	}
	if _, err := svc.ListItemAssets(ctx, "500", "unknown", nil, false); err == nil {
		t.Fatal("unknown asset source must be rejected")
	}
	if _, err := svc.RunBoardAutomation(ctx, "100", nil, 10, true); err == nil {
		t.Fatal("empty automation rules must be rejected")
	}
	if _, err := svc.RunBoardAutomation(ctx, "100", []application.AutomationRule{{ColumnID: "status", Operator: "invalid", SetColumnValues: map[string]any{"status": "Done"}}}, 10, true); err == nil {
		t.Fatal("invalid automation operator must be rejected")
	}
	if _, err := svc.RunBoardAutomation(ctx, "100", []application.AutomationRule{{ColumnID: "status", Operator: "empty"}}, 10, true); err == nil {
		t.Fatal("actionless automation rule must be rejected")
	}
	if _, err := svc.ImportBoardTable(ctx, "100", "xml", "name", true); err == nil {
		t.Fatal("unknown import format must be rejected")
	}
	if _, err := svc.BulkCreateItems(ctx, "100", nil, true); err == nil {
		t.Fatal("empty bulk rows must be rejected")
	}
	if port.MutationCount() != 0 {
		t.Fatalf("validation wrote mutations: %v", port.Mutations)
	}
}

func TestBoardFlowMetricsDateAndCycleBranches(t *testing.T) {
	svc, port := newService(nil)
	port.Items["500"] = domain.Item{ID: "500", Name: "Done", BoardID: "100", CreatedAt: "2026-09-20T00:00:00Z", UpdatedAt: "2026-09-25T00:00:00Z", Group: &domain.Group{ID: "todo", Title: "To Do"}, ColumnValues: []domain.ColumnValue{{ID: "status", Text: "Done", Type: "status"}}}
	port.Items["501"] = domain.Item{ID: "501", Name: "Open", BoardID: "100", CreatedAt: "2026-09-01T00:00:00Z", UpdatedAt: "2026-09-24T00:00:00Z", Group: &domain.Group{ID: "todo", Title: "To Do"}, ColumnValues: []domain.ColumnValue{{ID: "status", Text: "Working on it", Type: "status"}, {ID: "due", Text: "2026-09-01", Type: "date"}}}
	port.Items["502"] = domain.Item{ID: "502", Name: "Blocked", BoardID: "100", CreatedAt: "bad-date", UpdatedAt: "bad-date", ColumnValues: []domain.ColumnValue{{ID: "status", Text: "Stuck", Type: "status"}}}
	port.Order = []string{"500", "501", "502"}
	metrics, err := svc.BoardFlowMetrics(ctx, "100", 30, domain.ReportColumns{StatusColumnID: "status", DateColumnID: "due", PeopleColumnID: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if metrics.TotalItems != 3 || metrics.DoneItems == 0 || metrics.OpenItems == 0 || metrics.AverageCycleTimeProxy == 0 {
		t.Fatalf("metrics = %+v", metrics)
	}
	if metrics.CreatedInWindow == 0 || metrics.UpdatedInWindow == 0 || len(metrics.WIPByStatus) == 0 || len(metrics.WIPByGroup) == 0 {
		t.Fatalf("date/wip branches not exercised: %+v", metrics)
	}
	assets, err := svc.ListItemAssets(ctx, "500", "all", []string{"files"}, true)
	if err != nil || len(assets) != 1 || assets[0].PublicURL == "" {
		t.Fatalf("include URLs assets = %+v, err = %v", assets, err)
	}
}

func TestWorkflowInputValidationErrorBranches(t *testing.T) {
	svc, _ := newService(nil)
	if _, err := svc.ManageAutomation(ctx, "100", "pause", "77"); err == nil {
		t.Fatal("invalid automation action must be rejected")
	}
	if _, err := svc.ManageAutomation(ctx, "100", "deactivate", ""); err == nil {
		t.Fatal("empty automation id must be rejected")
	}
	if _, err := svc.ListAutomationRuns(ctx, "100", 0, []string{"bad"}, nil, ""); err == nil {
		t.Fatal("invalid automation run state must be rejected")
	}
	if _, err := svc.ListAutomationRuns(ctx, "100", 0, nil, nil, "abc"); err == nil {
		t.Fatal("non-numeric automation run item id must be rejected")
	}
	if _, err := svc.ListItemAssets(ctx, "500", "all", []string{""}, false); err == nil {
		t.Fatal("empty asset column id must be rejected")
	}
	if _, err := svc.ImportBoardTable(ctx, "100", "csv", "name,status\n,Done\n", true); err == nil {
		t.Fatal("import rows without names must be rejected")
	}
	rows := make([]application.BulkCreateRow, 51)
	for index := range rows {
		rows[index].Name = "row"
	}
	if _, err := svc.BulkCreateItems(ctx, "100", rows, true); err == nil {
		t.Fatal("more than 50 bulk create rows must be rejected")
	}
	if _, err := svc.CompleteItem(ctx, "", "", "", "", "", true); err == nil {
		t.Fatal("empty completion item id must be rejected")
	}
	if _, err := svc.HandoffItem(ctx, "500", "", nil, "", "", "", true); err == nil {
		t.Fatal("handoff without assignees must be rejected")
	}
	if _, err := svc.HandoffItem(ctx, "500", "", []int64{-1}, "", "", "", true); err == nil {
		t.Fatal("negative assignee IDs must be rejected")
	}
	if _, err := svc.HandoffItem(ctx, "500", "", []int64{1}, "status", "", "", true); err == nil {
		t.Fatal("status column without value must be rejected")
	}
	if _, err := svc.BoardFlowMetrics(ctx, "100", 366, domain.ReportColumns{}); err == nil {
		t.Fatal("window over 365 days must be rejected")
	}
}
