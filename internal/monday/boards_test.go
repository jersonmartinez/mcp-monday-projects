package monday

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jersonmartinez/mcp-monday-projects/internal/config"
)

func TestListBoards(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if !strings.Contains(string(body), "ListBoards") {
			t.Errorf("query body does not contain ListBoards: %s", body)
		}
		_, _ = w.Write([]byte(`{"data":{"boards":[{"id":"1","name":"Roadmap","state":"active","board_kind":"public"}]}}`))
	}))
	defer server.Close()

	client := NewClient(config.Config{APIToken: "token", APIVersion: "2026-07", APIURL: server.URL, HTTPTimeout: time.Second, MaxResponseBytes: 4096})
	boards, err := client.ListBoards(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListBoards() error = %v", err)
	}
	if len(boards) != 1 || boards[0].Name != "Roadmap" {
		t.Fatalf("boards = %+v", boards)
	}
}

func TestGetBoardNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"boards":[]}}`))
	}))
	defer server.Close()

	client := NewClient(config.Config{APIToken: "token", APIVersion: "2026-07", APIURL: server.URL, HTTPTimeout: time.Second, MaxResponseBytes: 4096})
	_, err := client.GetBoard(context.Background(), "missing")
	if err == nil {
		t.Fatal("GetBoard() error = nil, want not found")
	}
}
