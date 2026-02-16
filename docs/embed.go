package docs

import "embed"

// FS contains long-form Markdown docs bundled with the rvn binary.
//
//go:embed guide reference design index.yaml
var FS embed.FS
