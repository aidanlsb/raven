# Getting Started

When first interacting with a Raven vault, follow this discovery sequence:

1. **Understand the schema**: `raven_schema(subcommand="types")` and `raven_schema(subcommand="traits")`
2. **Get vault overview**: `raven_stats()` to see object counts and structure
3. **Check saved queries**: `raven_query(list=true)` to see pre-defined queries
4. **Discover workflows**: `raven_workflow_list()` to find available workflows

You can also fetch the `raven://schema/current` MCP resource for the complete schema.yaml.
