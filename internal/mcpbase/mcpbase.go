// Package mcpbase contains the transport-level conventions shared by xq-family
// domain MCP servers. It intentionally contains no domain tools or schemas.
package mcpbase

import (
	"context"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewReadOnlyServer creates an MCP server whose server-side logs are routed to
// stderr. Domain packages are responsible for registering their own tools and
// resources.
func NewReadOnlyServer(name, version, instructions string) *mcp.Server {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	return mcp.NewServer(&mcp.Implementation{Name: name, Version: version}, &mcp.ServerOptions{
		Instructions: instructions,
		Logger:       logger,
		// Do not advertise the deprecated MCP logging capability merely because
		// the SDK has a local logger. Tool/resource capabilities remain inferred.
		Capabilities: &mcp.ServerCapabilities{},
	})
}

// RunStdio runs a server on the standard local MCP transport. stdout is the
// protocol channel and must not be used for application logging.
func RunStdio(ctx context.Context, server *mcp.Server) error {
	return server.Run(ctx, &mcp.StdioTransport{})
}
