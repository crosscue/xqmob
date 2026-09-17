//go:build mcpintegration

package mcpserver

import (
	"context"
	"os"
	"testing"

	"github.com/crosscue/xqmob/internal/analyst"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPDatasetSummaryAndEntitySearch(t *testing.T) {
	root := os.Getenv("XQMOB_TEST_DATASET")
	if root == "" {
		t.Skip("XQMOB_TEST_DATASET not set")
	}
	ds, err := analyst.Open(root, analyst.Options{Threads: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer ds.Close()

	s := NewServer(ds)
	c := mcp.NewClient(&mcp.Implementation{Name: "xqmob-test-client", Version: "0.0.1"}, nil)
	t1, t2 := mcp.NewInMemoryTransports()
	ss, err := s.Connect(context.Background(), t1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := c.Connect(context.Background(), t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "dataset_summary", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("dataset_summary returned tool error: %+v", res.Content)
	}

	res, err = cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "search_entities", Arguments: map[string]any{"limit": 2}})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("search_entities returned tool error: %+v", res.Content)
	}

	rr, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "xqmob://guidance/analyst"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rr.Contents) != 1 || rr.Contents[0].Text == "" {
		t.Fatal("analyst guidance resource is empty")
	}
}
