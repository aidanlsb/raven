package docssvc

// TopicItem is one topic row in a docs section listing.
type TopicItem struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Title string `json:"title"`
}

// SectionsPayload is the success payload for `docs` with no section selected.
type SectionsPayload struct {
	CommandDocs   string        `json:"command_docs"`
	NavigationTip string        `json:"navigation_tip"`
	Sections      []SectionView `json:"sections"`
}

// TopicListPayload is the success payload for `docs <section>`.
type TopicListPayload struct {
	Section string      `json:"section"`
	Title   string      `json:"title"`
	Topics  []TopicItem `json:"topics"`
}

// TopicContentPayload is the success payload for `docs <section> <topic>`.
type TopicContentPayload struct {
	Content string `json:"content"`
	Path    string `json:"path"`
	Section string `json:"section"`
	Title   string `json:"title"`
	Topic   string `json:"topic"`
}

// SearchPayload is the success payload for `docs search`.
type SearchPayload struct {
	Count    int               `json:"count"`
	HasMore  bool              `json:"has_more"`
	Items    []SearchMatchView `json:"items"`
	Limit    int               `json:"limit"`
	Offset   int               `json:"offset"`
	Query    string            `json:"query"`
	Returned int               `json:"returned"`
}

func SectionsEnvelope(sections []SectionView) SectionsPayload {
	return SectionsPayload{
		CommandDocs:   "rvn help <command>",
		NavigationTip: "rvn docs <section> <topic>",
		Sections:      sections,
	}
}

func TopicListEnvelope(section SectionView, topics []TopicRecord) TopicListPayload {
	items := make([]TopicItem, 0, len(topics))
	for _, topic := range topics {
		items = append(items, TopicItem{ID: topic.ID, Path: topic.Path, Title: topic.Title})
	}
	return TopicListPayload{Section: section.ID, Title: section.Title, Topics: items}
}

func TopicContentEnvelope(topic TopicRecord, content string) TopicContentPayload {
	return TopicContentPayload{
		Content: content,
		Path:    topic.Path,
		Section: topic.Section,
		Title:   topic.Title,
		Topic:   topic.ID,
	}
}

func (r *SearchResult) Payload(query string) SearchPayload {
	return SearchPayload{
		Count:    r.Returned,
		HasMore:  r.HasMore,
		Items:    r.Matches,
		Limit:    r.Limit,
		Offset:   r.Offset,
		Query:    query,
		Returned: r.Returned,
	}
}

func (r *FetchResult) Payload() FetchResult {
	return *r
}
