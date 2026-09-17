package mcpserver

import (
	"context"
	"embed"
	"fmt"
	"strings"

	"github.com/crosscue/xqmob/internal/analyst"
	"github.com/crosscue/xqmob/internal/config"
	"github.com/crosscue/xqmob/internal/mcpbase"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed resources/*.md
var resourceFS embed.FS

type Options struct {
	DuckDBThreads int
}

type datasetSummaryOutput struct {
	analyst.DatasetSummary
	MCPContract string `json:"mcp_contract"`
}

func RunStdio(ctx context.Context, datasetRoot string, opts Options) error {
	ds, err := analyst.Open(datasetRoot, analyst.Options{Threads: opts.DuckDBThreads})
	if err != nil {
		return err
	}
	defer ds.Close()

	server := NewServer(ds)

	// stdout is exclusively the MCP protocol channel in stdio mode. All
	// server-side diagnostics are routed to stderr by mcpbase.
	return mcpbase.RunStdio(ctx, server)
}

// NewServer constructs the domain MCP surface for an already opened dataset.
// It is exported primarily for integration testing and future non-stdio adapters.
func NewServer(ds *analyst.Dataset) *mcp.Server {
	guidance, _ := resourceFS.ReadFile("resources/ANALYST-GUIDANCE.md")
	server := mcpbase.NewReadOnlyServer("xqmob", config.Version, string(guidance))
	registerResources(server, ds)
	registerTools(server, ds)
	return server
}

func registerResources(server *mcp.Server, ds *analyst.Dataset) {
	addTextResource := func(uri, name, description, mime, text string) {
		server.AddResource(&mcp.Resource{URI: uri, Name: name, Description: description, MIMEType: mime},
			func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: mime, Text: text}}}, nil
			})
	}
	manifest, _ := ds.ManifestJSON()
	addTextResource("xqmob://dataset/manifest", "Dataset manifest", "Complete xqmob manifest for the opened dataset.", "application/json", string(manifest))
	capabilities := fmt.Sprintf(`{"analysis_contract":%q,"h3_materialized":%t,"compatibility_mode":%t}`, ds.Manifest.AnalysisContract, ds.H3Available(), ds.Manifest.AnalysisContract != config.AnalysisContractVersion)
	addTextResource("xqmob://dataset/capabilities", "Dataset capabilities", "Analyst API capabilities available for the opened dataset.", "application/json", capabilities)
	analyticalFile := "ANALYTICAL-CONTRACT-V0.2.md"
	if ds.Manifest.AnalysisContract == "xqmob-analysis-v0.1" {
		analyticalFile = "ANALYTICAL-CONTRACT-V0.1.md"
	}
	static := []struct{ uri, file, name, desc string }{
		{"xqmob://guidance/analyst", "ANALYST-GUIDANCE.md", "Mobility analyst guidance", "Evidence and inference rules for LLM analysis of xqmob data."},
		{"xqmob://contract/analytical", analyticalFile, "Analytical contract", "Analytical projection contract matching the opened dataset."},
		{"xqmob://contract/spatial-indexing", "SPATIAL-INDEXING.md", "Spatial indexing contract", "Geohash and H3 analytical indexing design; H3 capabilities depend on the opened dataset contract."},
		{"xqmob://contract/eventizer", "MOBILITY-EVENTIZER-V1.2.md", "Mobility eventizer contract", "Reference mobility eventizer algorithm contract."},
	}
	for _, r := range static {
		b, err := resourceFS.ReadFile("resources/" + r.file)
		if err != nil {
			continue
		}
		addTextResource(r.uri, r.name, r.desc, "text/markdown", string(b))
	}
}

func registerTools(server *mcp.Server, ds *analyst.Dataset) {
	mcp.AddTool(server, &mcp.Tool{Name: "dataset_summary", Description: "Return the compact dataset-level xqmob manifest summary. Use this first when beginning an investigation."},
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			out := datasetSummaryOutput{DatasetSummary: ds.Summary(), MCPContract: config.MCPContractVersion}
			return nil, out, nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: "search_entities", Description: "Find entity summaries using analytical filters. Results are bounded and cursor-paginated; use this to discover entities before requesting observations."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in analyst.SearchEntitiesInput) (*mcp.CallToolResult, any, error) {
			out, err := ds.SearchEntities(ctx, in)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "get_entity", Description: "Return the analytical summary for one raw entity ID."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in analyst.EntityInput) (*mcp.CallToolResult, any, error) {
			out, err := ds.GetEntity(ctx, in.EntityID)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "get_entity_timeline", Description: "Return a chronological union of semantic events, presence intervals, and confirmed transitions for one entity. Use this before drilling into observations."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in analyst.TimePageInput) (*mcp.CallToolResult, any, error) {
			out, err := ds.GetTimeline(ctx, in)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "get_presence", Description: "Query inferred presence intervals by entity, classification, time range, and H3 cell when the dataset materializes H3."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in analyst.PresenceInput) (*mcp.CallToolResult, any, error) {
			out, err := ds.GetPresence(ctx, in)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "get_transitions", Description: "Query confirmed movements between presence intervals, with entity/time/accuracy-support filters and optional H3 filters when materialized."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in analyst.TransitionInput) (*mcp.CallToolResult, any, error) {
			out, err := ds.GetTransitions(ctx, in)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "get_segments", Description: "Query physically continuous trajectory segments. GAP alone does not split a segment; DISCONTINUITY does."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in analyst.TimePageInput) (*mcp.CallToolResult, any, error) {
			out, err := ds.GetSegments(ctx, in)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "get_events", Description: "Query flattened semantic mobility events by entity, event type, time range, and H3 cell when materialized."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in analyst.EventInput) (*mcp.CallToolResult, any, error) {
			out, err := ds.GetEvents(ctx, in)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "get_observations", Description: "Query normalized source observations. Requires entity_id, or h3_cell when H3 is materialized; capped at 1000 rows per call. Use observations to verify derived findings."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in analyst.ObservationInput) (*mcp.CallToolResult, any, error) {
			out, err := ds.GetObservations(ctx, in)
			return nil, out, err
		})
	if ds.H3Available() {
		mcp.AddTool(server, &mcp.Tool{Name: "describe_h3_cell", Description: "Describe one materialized H3 analytical cell: observations, entities, presence/dwell, incoming/outgoing transitions, top flows, and top entities."},
			func(ctx context.Context, _ *mcp.CallToolRequest, in analyst.H3CellInput) (*mcp.CallToolResult, any, error) {
				out, err := ds.DescribeH3Cell(ctx, in)
				return nil, out, err
			})
	}
	mcp.AddTool(server, &mcp.Tool{Name: "explain_event", Description: "Explain one semantic event by returning the event, nearby supporting observations, and linked presence/segment context where available."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in analyst.EventIDInput) (*mcp.CallToolResult, any, error) {
			out, err := ds.ExplainEvent(ctx, in.EventID)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "explain_discontinuity", Description: "Explain a DISCONTINUITY event using its adjacent evidence, configured speed/jump thresholds, HA support, and derived reason. A discontinuity is a model violation, not proof of physical travel."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in analyst.EventIDInput) (*mcp.CallToolResult, any, error) {
			out, err := ds.ExplainDiscontinuity(ctx, in.EventID)
			return nil, out, err
		})
}

func ServerDescription() string {
	return strings.TrimSpace(`xqmob MCP is a read-only local mobility analyst interface. It uses DuckDB to query xqmob Parquet/GeoParquet projections with filter and projection pushdown. Collection results are cursor-paginated and capped at 1000 rows per tool call. stdout is reserved exclusively for MCP stdio protocol messages.`)
}

func BuildNote() string {
	return fmt.Sprintf("%s uses embedded DuckDB and the official MCP Go SDK; CGO must be enabled for the MCP-capable binary", config.MCPContractVersion)
}
