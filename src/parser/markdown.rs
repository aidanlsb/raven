//! Markdown parser - extracts document structure (headings, etc.)

use pulldown_cmark::{Event, HeadingLevel, Parser, Tag, TagEnd};

/// A heading in the document
#[derive(Debug, Clone)]
pub struct Heading {
    /// Heading level (1-6)
    pub level: u8,
    
    /// Heading text content
    pub text: String,
    
    /// Line number (1-indexed)
    pub line: usize,
}

/// Parsed markdown structure
#[derive(Debug, Clone)]
pub struct MarkdownStructure {
    /// All headings in the document
    pub headings: Vec<Heading>,
}

/// Parse markdown structure from content
pub fn parse_markdown_structure(content: &str, start_line: usize) -> MarkdownStructure {
    let mut headings = Vec::new();
    let mut current_heading_level: Option<u8> = None;
    let mut current_heading_text = String::new();
    let mut current_line = start_line;
    
    // Track line numbers by counting newlines
    let mut char_to_line: Vec<usize> = Vec::with_capacity(content.len());
    let mut line = start_line;
    for c in content.chars() {
        char_to_line.push(line);
        if c == '\n' {
            line += 1;
        }
    }
    
    let parser = Parser::new(content);
    let mut offset = 0;
    
    for (event, range) in parser.into_offset_iter() {
        offset = range.start;
        current_line = char_to_line.get(offset).copied().unwrap_or(start_line);
        
        match event {
            Event::Start(Tag::Heading { level, .. }) => {
                current_heading_level = Some(heading_level_to_u8(level));
                current_heading_text.clear();
            }
            Event::Text(text) if current_heading_level.is_some() => {
                current_heading_text.push_str(&text);
            }
            Event::End(TagEnd::Heading(_)) => {
                if let Some(level) = current_heading_level.take() {
                    headings.push(Heading {
                        level,
                        text: current_heading_text.clone(),
                        line: current_line,
                    });
                }
            }
            _ => {}
        }
    }
    
    MarkdownStructure { headings }
}

fn heading_level_to_u8(level: HeadingLevel) -> u8 {
    match level {
        HeadingLevel::H1 => 1,
        HeadingLevel::H2 => 2,
        HeadingLevel::H3 => 3,
        HeadingLevel::H4 => 4,
        HeadingLevel::H5 => 5,
        HeadingLevel::H6 => 6,
    }
}

/// Find the line range for a heading's content
/// Returns (start_line, end_line) where end_line is exclusive
pub fn find_heading_scope(
    headings: &[Heading],
    heading_idx: usize,
    total_lines: usize,
) -> (usize, usize) {
    let heading = &headings[heading_idx];
    let start = heading.line;
    
    // Find next heading at same or higher level
    let end = headings
        .iter()
        .skip(heading_idx + 1)
        .find(|h| h.level <= heading.level)
        .map(|h| h.line)
        .unwrap_or(total_lines + 1);
    
    (start, end)
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_parse_headings() {
        let content = r#"# Main Title

Some content here.

## Section One

Content for section one.

## Section Two

### Subsection

More content.
"#;
        
        let structure = parse_markdown_structure(content, 1);
        
        assert_eq!(structure.headings.len(), 4);
        assert_eq!(structure.headings[0].level, 1);
        assert_eq!(structure.headings[0].text, "Main Title");
        assert_eq!(structure.headings[1].level, 2);
        assert_eq!(structure.headings[1].text, "Section One");
    }
    
    #[test]
    fn test_heading_scope() {
        let headings = vec![
            Heading { level: 1, text: "Main".to_string(), line: 1 },
            Heading { level: 2, text: "Section 1".to_string(), line: 5 },
            Heading { level: 2, text: "Section 2".to_string(), line: 10 },
        ];
        
        // Section 1 scope is lines 5-9
        let (start, end) = find_heading_scope(&headings, 1, 15);
        assert_eq!(start, 5);
        assert_eq!(end, 10);
        
        // Section 2 scope is lines 10-end
        let (start, end) = find_heading_scope(&headings, 2, 15);
        assert_eq!(start, 10);
        assert_eq!(end, 16);
    }
}
