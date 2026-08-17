# drawio-mcp

A small Go MCP server for finding bundled draw.io shapes and placing them in `.drawio` files.

The server is self-contained: the libraries in `template/*.xml` are compiled into the binary with `go:embed`.

## Tools

- `list_shapes` - list shapes and filter by source, category, or tags
- `find_shapes` - search shape IDs, names, descriptions, and tags
- `create_diagram` - create an in-memory draw.io document
- `open_diagram` - open compressed or uncompressed draw.io XML
- `save_diagram` - save an open document
- `close_diagram` - release an open document; unsaved changes require `force: true`
- `place_shape` - place a shape using its catalog ID and default or explicit dimensions
- `move_shape` - move a shape using absolute page coordinates
- `set_shape_label` - change or clear a shape label
- `delete_shape` - delete a shape, its descendants, and connected edges
- `inspect_diagram` - inspect pages and vertex cells
- `inspect_region` - return shapes inside a page rectangle and optionally connected or crossing edges
- `route_edges` - automatically route directed orthogonal edges on shareable grid lanes

Bundled IDs are stable and namespaced, for example `default.rectangle`, `saturday.golang`, and `cloudflare.wrangler`.

## Build and run

```sh
go test ./...
go build -o drawio-mcp ./cmd/drawio-mcp
./drawio-mcp
```

Run the current checkout without building a binary first:

```sh
go run ./cmd/drawio-mcp
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

## MCP registration with Go

Run the latest published version directly. `go run` downloads the module into the Go build cache and starts the MCP server without installing a command into `PATH`:

```json
{
  "mcpServers": {
    "drawio": {
      "command": "go",
      "args": [
        "run",
        "github.com/snowmerak/drawio-mcp/cmd/drawio-mcp@latest"
      ]
    }
  }
}
```

The equivalent shell command is:

```sh
go run github.com/snowmerak/drawio-mcp/cmd/drawio-mcp@latest
```

To install a reusable executable instead:

```sh
go install github.com/snowmerak/drawio-mcp/cmd/drawio-mcp@latest
```

After ensuring `$(go env GOPATH)/bin` is in `PATH`, register the installed command:

```json
{
  "mcpServers": {
    "drawio": {
      "command": "drawio-mcp"
    }
  }
}
```

For local development, clients that support a working-directory setting can run the current checkout:

```json
{
  "mcpServers": {
    "drawio-local": {
      "command": "go",
      "args": ["run", "./cmd/drawio-mcp"],
      "cwd": "/absolute/path/to/drawio-mcp"
    }
  }
}
```

`create_diagram` does not write immediately. Use the returned `document_id` with `place_shape`, then call `save_diagram`. Existing compressed draw.io pages can be opened; saved output is normalized to editable, uncompressed `mxGraphModel` XML.

`close_diagram` removes a document from the server's in-memory document manager. It rejects modified documents unless `force` is true. `move_shape` uses absolute page coordinates, including for nested shapes. `delete_shape` also removes descendant shapes and connected edges so it cannot leave dangling edge endpoints.

`inspect_region` accepts a page rectangle in draw.io model coordinates:

```json
{
  "document_id": "doc-a41d93b75e420fa1",
  "page": "Architecture",
  "x": 100,
  "y": 80,
  "width": 600,
  "height": 400,
  "match": "intersects",
  "edge_mode": "connected",
  "include_external_nodes": true,
  "include_style": false,
  "include_metadata": false,
  "limit": 200
}
```

`match` may be `intersects` or `contained`. `edge_mode` may be `none`, `connected`, or `intersects`. Embedded image payloads are omitted even when styles are requested.

`route_edges` routes a batch atomically on directed orthogonal grid lanes:

```json
{
  "document_id": "doc-a41d93b75e420fa1",
  "page": "Architecture",
  "connections": [
    {"source_id": "api-1", "target_id": "db-1", "label": "query"},
    {"source_id": "api-2", "target_id": "db-2", "label": "query"}
  ],
  "grid_size": 20,
  "clearance": 20,
  "bend_penalty": 20,
  "new_lane_cost": 10,
  "shared_lane_cost": 3
}
```

Each empty lane can be occupied once per direction. Unlabeled edges may share it only in that same direction; an edge traveling in the opposite direction must find another lane. An edge with a non-empty label reserves its entire route exclusively, so no other edge may share those lane segments in either direction. Routed lane IDs and labeled-edge exclusivity are restored from saved edges on later calls. If any connection cannot be routed, no edge from the batch is added.

Whole-route exclusivity is intentionally conservative. A future refinement can estimate the label bounds, reserve only the affected lane interval, avoid perpendicular crossings through that interval, and offset the label from its own polyline.

## Generate the example through MCP

After building the server, run the example MCP client:

```sh
go run ./cmd/drawio-example -server ./build/drawio-mcp.exe -output ./examples/generated-architecture.drawio
```

The example client uses `find_shapes`, `create_diagram`, `place_shape`, and `save_diagram` over a real stdio MCP connection.
