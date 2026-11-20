package application_test

import (
	"testing"

	"github.com/jersonmartinez/mcp-monday-projects/internal/domain"
)

func TestProgramScaleReadsAndDependencyValidation(t *testing.T) {
	svc, port := newService(nil)
	port.Columns["100"] = append(port.Columns["100"], domain.Column{ID: "phase", Title: "Phase", Type: "dropdown"}, domain.Column{ID: "timeline", Title: "Timeline", Type: "timeline"})
	port.Items["500"] = domain.Item{ID: "500", Name: "External reference", BoardID: "100", ColumnValues: []domain.ColumnValue{
		{ID: "status", Text: "Done"}, {ID: "phase", Text: "Build"}, {ID: "est", Text: "5", Value: "5"},
		{ID: "dependency", LinkedItemIDs: []string{"501"}},
	}}
	port.Items["501"] = domain.Item{ID: "501", Name: "Dependent item", BoardID: "100", ColumnValues: []domain.ColumnValue{{ID: "status", Text: "Working on it"}, {ID: "est", Text: "3", Value: "3"}}}
	port.Order = []string{"500", "501"}

	page, err := svc.TextMatch(ctx, "100", "external", nil, 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "500" {
		t.Fatalf("text search = %+v, err=%v", page, err)
	}
	progress, err := svc.Progress(ctx, "100", "status", "phase", "est", "", 10)
	if err != nil || progress.TotalPlanned != 8 || len(progress.Buckets) != 2 {
		t.Fatalf("progress = %+v, err=%v", progress, err)
	}
	deps, err := svc.Dependencies(ctx, "100", "500", "dependency")
	if err != nil || len(deps.DependsOn) != 1 || deps.DependsOn[0] != "501" {
		t.Fatalf("dependencies = %+v, err=%v", deps, err)
	}
	if _, _, err := svc.SetDependencies(ctx, "100", "500", "dependency", "replace", []string{"999"}); err == nil {
		t.Fatal("missing dependency item must be rejected")
	}
	verified, err := svc.VerifyColumns(ctx, map[string]map[string]any{"500": map[string]any{"est": "5"}})
	if err != nil || verified.Correct != 1 {
		t.Fatalf("verify = %+v, err=%v", verified, err)
	}
}

func TestCreateSubitemRejectsSubitemParent(t *testing.T) {
	svc, port := newService(nil)
	port.Items["501"] = domain.Item{ID: "501", Name: "Already child", BoardID: "100", ParentItemID: "500"}
	if _, err := svc.CreateSubitem(ctx, "501", "nested", nil); err == nil {
		t.Fatal("nested subitem must be rejected")
	}
	if port.MutationCount() != 0 {
		t.Fatalf("mutation reached port: %v", port.Mutations)
	}
}

func TestBulkCreateUpdatesDryRun(t *testing.T) {
	svc, port := newService(nil)
	result, err := svc.BulkCreateUpdates(ctx, map[string]string{"500": "status update"}, "", true)
	if err != nil || result.Planned != 1 || result.Succeeded != 0 || port.MutationCount() != 0 {
		t.Fatalf("result = %+v, err=%v", result, err)
	}
	result, err = svc.BulkCreateUpdates(ctx, map[string]string{"500": "status update"}, "", false)
	if err != nil || result.Succeeded != 1 || port.MutationCount() != 1 {
		t.Fatalf("write result = %+v, err=%v", result, err)
	}
}

func TestProgramScaleBatchRollupsAndPreviews(t *testing.T) {
	svc, port := newService(nil)
	port.Columns["100"] = append(port.Columns["100"], domain.Column{ID: "timeline", Title: "Timeline", Type: "timeline"})
	port.Items["500"] = domain.Item{ID: "500", Name: "Parent", BoardID: "100", ColumnValues: []domain.ColumnValue{
		{ID: "timeline", Value: `{"from":"2026-10-01","to":"2026-10-05"}`},
	}, Subitems: []domain.Subitem{
		{ID: "501", ColumnValues: []domain.ColumnValue{{ID: "status", Text: "Done"}}},
		{ID: "502", ColumnValues: []domain.ColumnValue{{ID: "status", Text: "Working on it"}}},
	}}
	port.Items["501"] = domain.Item{ID: "501", Name: "Child", BoardID: "100", ColumnValues: []domain.ColumnValue{
		{ID: "dependency", LinkedItemIDs: []string{"500"}}, {ID: "timeline", Value: `{"from":"2026-10-06","to":"2026-10-10"}`},
	}}
	port.Order = []string{"500", "501"}

	subitems, err := svc.Subitems(ctx, "500", []string{"status"})
	if err != nil || len(subitems) != 2 || len(subitems[0].ColumnValues) != 1 {
		t.Fatalf("subitems = %+v, err=%v", subitems, err)
	}
	rollup, err := svc.SubitemsRollup(ctx, []string{"500"}, "status")
	if err != nil || rollup[0].Total != 2 || len(rollup[0].ByStatus) != 2 {
		t.Fatalf("rollup = %+v, err=%v", rollup, err)
	}
	preview, err := svc.PreviewDependencyMove(ctx, "100", "500", "timeline", "dependency", "2026-10-03", "2026-10-07", 10)
	if err != nil || len(preview) != 1 || preview[0].ShiftDays != 2 || preview[0].ProjectedFrom != "2026-10-08" {
		t.Fatalf("preview = %+v, err=%v", preview, err)
	}
	if warnings := svc.PersistedValueWarnings(ctx, "500", "timeline", map[string]any{"from": "2026-10-03", "to": "2026-10-07"}); len(warnings) != 1 {
		t.Fatalf("warnings = %v", warnings)
	}

	if _, _, err := svc.SetDependencies(ctx, "100", "500", "dependency", "add", []string{"501"}); err != nil {
		t.Fatalf("add dependencies = %v", err)
	}
	if _, _, err := svc.SetDependencies(ctx, "100", "500", "dependency", "remove", []string{"501"}); err != nil {
		t.Fatalf("remove dependencies = %v", err)
	}
	if result, err := svc.BulkCreateUpdates(ctx, map[string]string{"bad": "x"}, "", true); err != nil || result.Failed != 1 {
		t.Fatalf("invalid bulk updates = %+v, err=%v", result, err)
	}
}
