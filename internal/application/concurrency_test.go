package application_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jersonmartinez/mcp-monday-projects/internal/application"
	"github.com/jersonmartinez/mcp-monday-projects/internal/application/applicationtest"
	"github.com/jersonmartinez/mcp-monday-projects/internal/domain"
)

type slowUpdatePort struct {
	*applicationtest.FakePort
	mu     sync.Mutex
	active int
	max    int
}

func (p *slowUpdatePort) UpdateItemValues(ctx context.Context, boardID, itemID string, values map[string]any) (*domain.Item, error) {
	p.mu.Lock()
	p.active++
	if p.active > p.max {
		p.max = p.active
	}
	p.mu.Unlock()
	time.Sleep(20 * time.Millisecond)
	defer func() {
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
	}()
	return p.FakePort.UpdateItemValues(ctx, boardID, itemID, values)
}

func TestBulkUpdateUsesBoundedConcurrency(t *testing.T) {
	port := &slowUpdatePort{FakePort: applicationtest.NewFakePort()}
	svc := application.NewService(port, application.Options{BulkMaxConcurrency: 4})
	updates := []application.BulkUpdate{
		{ItemID: "500", ColumnValues: map[string]any{"est": 1}},
		{ItemID: "501", ColumnValues: map[string]any{"est": 1}},
		{ItemID: "502", ColumnValues: map[string]any{"est": 1}},
		{ItemID: "503", ColumnValues: map[string]any{"est": 1}},
	}
	result, err := svc.BulkUpdateItems(context.Background(), "100", updates, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Succeeded != 4 {
		t.Fatalf("result = %+v", result)
	}
	port.mu.Lock()
	max := port.max
	port.mu.Unlock()
	if max < 2 || max > 4 {
		t.Fatalf("max active updates = %d, want bounded parallelism in [2,4]", max)
	}
}
