//! Markdown parsing utilities using pulldown-cmark

use pulldown_cmark::{Event, HeadingLevel, Parser, Tag, TagEnd};

/// A parsed heading
#[derive(Debug, Clone)]
pub struct Heading {
    pub level: u32,
    pub text: String,
    pub line: usize,
}

/// Extract headings from markdown content using pulldown-cmark
pub fn extract_headings(content: &str, start_line: usize) -> Vec<Heading> {
    let mut headings = Vec::new();
    let parser = Parser::new(content);
    
    let mut current_heading: Option<(u32, usize)> = None;
    let mut heading_text = String::new();
    
    // Pre-compute line number for each byte offset
    let line_numbers: Vec<usize> = {
        let mut lines = vec![start_line; content.len() + 1];
        let mut current = start_line;
        for (i, c) in content.char_indices() {
            lines[i] = current;
            if c == '\n' {
                current += 1;
            }
        }
        if content.len() < lines.len() {
            lines[content.len()] = current;
        }
        lines
    };
    
    for (event, range) in parser.into_offset_iter() {
        let line = *line_numbers.get(range.start).unwrap_or(&start_line);
        
        match event {
            Event::Start(Tag::Heading { level, .. }) => {
                let level_num = match level {
                    HeadingLevel::H1 => 1,
                    HeadingLevel::H2 => 2,
                    HeadingLevel::H3 => 3,
                    HeadingLevel::H4 => 4,
                    HeadingLevel::H5 => 5,
                    HeadingLevel::H6 => 6,
                };
                current_heading = Some((level_num, line));
                heading_text.clear();
            }
            Event::Text(text) if current_heading.is_some() => {
                heading_text.push_str(&text);
            }
            Event::End(TagEnd::Heading(_)) => {
                if let Some((level, heading_line)) = current_heading.take() {
                    if !heading_text.trim().is_empty() {
                        headings.push(Heading {
                            level,
                            text: heading_text.trim().to_string(),
                            line: heading_line,
                        });
                    }
                }
                heading_text.clear();
            }
            _ => {}
        }
    }
    
    headings
}

/// Extract inline tags from content (#tag format)
/// Only extracts tags from text content, not from code blocks or inline code
pub fn extract_inline_tags(content: &str) -> Vec<String> {
    let parser = Parser::new(content);
    let mut tags = Vec::new();
    let mut in_code = false;
    
    for event in parser {
        match event {
            Event::Start(Tag::CodeBlock(_)) => in_code = true,
            Event::End(TagEnd::CodeBlock) => in_code = false,
            Event::Code(_) => {} // Skip inline code
            Event::Text(text) if !in_code => {
                // Extract #tags from regular text
                tags.extend(extract_tags_from_text(&text));
            }
            _ => {}
        }
    }
    
    tags.sort();
    tags.dedup();
    tags
}

/// Extract #tag patterns from a text string
fn extract_tags_from_text(text: &str) -> Vec<String> {
    let mut tags = Vec::new();
    let mut chars = text.chars().peekable();
    let mut prev_char = ' ';
    
    while let Some(c) = chars.next() {
        // # must be preceded by whitespace or start of string or punctuation
        if c == '#' && (prev_char.is_whitespace() || prev_char == '(' || prev_char == '[' || prev_char == '\0') {
            let mut tag = String::new();
            while let Some(&next) = chars.peek() {
                if next.is_alphanumeric() || next == '_' || next == '-' {
                    tag.push(chars.next().unwrap());
                } else {
                    break;
                }
            }
            // Tags must be at least 1 char and not start with a number (avoid #123 issue refs)
            if !tag.is_empty() && !tag.chars().next().unwrap().is_ascii_digit() {
                tags.push(tag);
            }
        }
        prev_char = c;
    }
    
    tags
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_extract_headings() {
        let content = "# Heading 1\n\nSome text\n\n## Heading 2\n\n### Heading 3";
        let headings = extract_headings(content, 1);
        
        assert_eq!(headings.len(), 3);
        assert_eq!(headings[0].level, 1);
        assert_eq!(headings[0].text, "Heading 1");
        assert_eq!(headings[1].level, 2);
        assert_eq!(headings[2].level, 3);
    }
    
    #[test]
    fn test_heading_in_code_block_ignored() {
        let content = "# Real Heading\n\n```\n# Not a heading\n```\n\n## Another Real";
        let headings = extract_headings(content, 1);
        
        assert_eq!(headings.len(), 2);
        assert_eq!(headings[0].text, "Real Heading");
        assert_eq!(headings[1].text, "Another Real");
    }
    
    #[test]
    fn test_extract_inline_tags() {
        let content = "Some text with #tag1 and #tag2, also (#tag3)";
        let tags = extract_inline_tags(content);
        
        assert!(tags.contains(&"tag1".to_string()));
        assert!(tags.contains(&"tag2".to_string()));
        assert!(tags.contains(&"tag3".to_string()));
    }
    
    #[test]
    fn test_tags_in_code_block_ignored() {
        let content = "Real #tag here\n\n```\n#not-a-tag\n```\n\nAnd `#also-not-tag` inline";
        let tags = extract_inline_tags(content);
        
        assert_eq!(tags.len(), 1);
        assert!(tags.contains(&"tag".to_string()));
    }
    
    #[test]
    fn test_issue_numbers_not_tags() {
        let content = "Fix #123 and add #feature";
        let tags = extract_inline_tags(content);
        
        assert_eq!(tags.len(), 1);
        assert!(tags.contains(&"feature".to_string()));
        assert!(!tags.contains(&"123".to_string()));
    }
}
