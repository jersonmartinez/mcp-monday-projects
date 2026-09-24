package monday

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jersonmartinez/mcp-monday-projects/internal/config"
)

func TestListItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"boards":[{"items_page":{"items":[{"id":"10","name":"Task","state":"active","group":{"id":"topics","title":"Topics"},"column_values":[{"id":"status","text":"Todo","value":"{\"index\":0}","type":"status"}]}]}}]}}`))
	}))
	defer server.Close()

	client := NewClient(config.Config{APIToken: "token", APIVersion: "2026-07", APIURL: server.URL, HTTPTimeout: time.Second, MaxResponseBytes: 4096})
	items, err := client.ListItems(context.Background(), "123", 10)
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}
	if len(items) != 1 || items[0].Group == nil || items[0].Group.Title != "Topics" {
		t.Fatalf("items = %+v", items)
	}
}
