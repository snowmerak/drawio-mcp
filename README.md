# drawio-mcp

A small Go MCP server for finding bundled draw.io shapes and placing them in `.drawio` files.

The server is self-contained: the libraries in `template/*.xml` are compiled into the binary with `go:embed`.

## Tools

- `list_shapes` — list shapes and filter by source, category, or tags
- `find_shapes` — search shape IDs, names, descriptions, and tags
- `create_diagram` — create an in-memory draw.io document
- `open_diagram` — open compressed or uncompressed draw.io XML
- `save_diagram` — save an open document
- `place_shape` — place a shape using its catalog ID and default or explicit dimensions
- `inspect_diagram` — inspect pages and vertex cells

Bundled IDs are stable and namespaced, for example `default.rectangle`, `saturday.golang`, and `cloudflare.wrangler`.

## Build and run

```sh
go test ./...
go build -o drawio-mcp ./cmd/drawio-mcp
./drawio-mcp
```

The process uses MCP over stdio, so stdout is reserved for protocol messages.

Example client configuration:

```json
{
  "mcpServers": {
    "drawio": {
      "command": "/absolute/path/to/drawio-mcp"
    }
  }
}
```

`create_diagram` does not write immediately. Use the returned `document_id` with `place_shape`, then call `save_diagram`. Existing compressed draw.io pages can be opened; saved output is normalized to editable, uncompressed `mxGraphModel` XML.

## Generate the example through MCP

After building the server, run the example MCP client:

```sh
go run ./cmd/drawio-example -server ./build/drawio-mcp.exe -output ./examples/generated-architecture.drawio
```

The example client uses `find_shapes`, `create_diagram`, `place_shape`, and `save_diagram` over a real stdio MCP connection.
