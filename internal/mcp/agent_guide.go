package mcp

import (
	"embed"
)

const agentGuideIndexPath = "agent-guide/index.md"

// GuideTopic defines a single agent guide topic resource.
type GuideTopic struct {
	Slug        string
	Name        string
	Description string
	Path        string
}

var guideTopics = []GuideTopic{
	{
		Slug:        "critical-rules",
		Name:        "Critical Rules",
		Description: "Non-negotiable safety rules for Raven operations.",
		Path:        "agent-guide/critical-rules.md",
	},
	{
		Slug:        "quickstart",
		Name:        "Quickstart Mental Model",
		Description: "One-pass conceptual overview of Raven and first commands to run.",
		Path:        "agent-guide/quickstart.md",
	},
	{
		Slug:        "onboarding",
		Name:        "Vault Onboarding",
		Description: "Guide for helping users set up a new vault interactively, including teaching sequence and misconceptions.",
		Path:        "agent-guide/onboarding.md",
	},
	{
		Slug:        "getting-started",
		Name:        "Getting Started",
		Description: "Discovery sequence and first steps in a new vault.",
		Path:        "agent-guide/getting-started.md",
	},
	{
		Slug:        "core-concepts",
		Name:        "Core Concepts",
		Description: "Types, traits, references, schema, and file formats.",
		Path:        "agent-guide/core-concepts.md",
	},
	{
		Slug:        "response-contract",
		Name:        "Response Contract",
		Description: "Standard JSON envelope, error/warning handling, and preview/apply semantics.",
		Path:        "agent-guide/response-contract.md",
	},
	{
		Slug:        "write-patterns",
		Name:        "Write Patterns",
		Description: "How to choose new/add/upsert/set/edit/update for safe, predictable writes.",
		Path:        "agent-guide/write-patterns.md",
	},
	{
		Slug:        "querying",
		Name:        "Querying",
		Description: "Raven Query Language reference and query strategy.",
		Path:        "agent-guide/querying.md",
	},
	{
		Slug:        "query-cheatsheet",
		Name:        "Query Cheatsheet",
		Description: "Fast patterns for common RQL queries.",
		Path:        "agent-guide/query-cheatsheet.md",
	},
	{
		Slug:        "query-at-scale",
		Name:        "Query At Scale",
		Description: "Pagination and narrowing strategies for large result sets.",
		Path:        "agent-guide/query-at-scale.md",
	},
	{
		Slug:        "key-flows",
		Name:        "Key Flows",
		Description: "Operational playbook for common end-to-end Raven tasks.",
		Path:        "agent-guide/key-flows.md",
	},
	{
		Slug:        "error-handling",
		Name:        "Error Handling",
		Description: "How to interpret and recover from tool errors.",
		Path:        "agent-guide/error-handling.md",
	},
	{
		Slug:        "issue-types",
		Name:        "Issue Types Reference",
		Description: "`check` issue reference and suggested fixes.",
		Path:        "agent-guide/issue-types.md",
	},
	{
		Slug:        "best-practices",
		Name:        "Best Practices",
		Description: "Operating principles and safety checks.",
		Path:        "agent-guide/best-practices.md",
	},
	{
		Slug:        "examples",
		Name:        "Example Conversations",
		Description: "Example conversations and query translations.",
		Path:        "agent-guide/examples.md",
	},
}

// agentGuideFS embeds the modular agent guide topics.
//
//go:embed agent-guide/*.md
var agentGuideFS embed.FS

func listAgentGuideResources() []Resource {
	resources := []Resource{
		{
			URI:         "raven://guide/index",
			Name:        "Agent Guide Index",
			Description: "Overview of available agent guide topics.",
			MimeType:    "text/markdown",
		},
	}

	for _, topic := range guideTopics {
		resources = append(resources, Resource{
			URI:         "raven://guide/" + topic.Slug,
			Name:        topic.Name,
			Description: topic.Description,
			MimeType:    "text/markdown",
		})
	}

	return resources
}

func getAgentGuideIndex() (string, bool) {
	return readAgentGuideFile(agentGuideIndexPath)
}

func getAgentGuideTopic(slug string) (GuideTopic, string, bool) {
	for _, topic := range guideTopics {
		if topic.Slug == slug {
			content, ok := readAgentGuideFile(topic.Path)
			if !ok {
				return GuideTopic{}, "", false
			}
			return topic, content, true
		}
	}

	return GuideTopic{}, "", false
}

func readAgentGuideFile(path string) (string, bool) {
	data, err := agentGuideFS.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}
