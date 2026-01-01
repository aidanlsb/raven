//! Type declaration parser - parses ::type(id=..., ...) syntax

use anyhow::{bail, Result};
use regex::Regex;
use std::collections::HashMap;

use crate::schema::FieldValue;

lazy_static::lazy_static! {
    // Matches ::typename(args...)
    static ref TYPE_DECL_REGEX: Regex = Regex::new(
        r"^::(\w+)\s*\(([^)]*)\)\s*$"
    ).unwrap();
}

/// A parsed type declaration
#[derive(Debug, Clone)]
pub struct TypeDeclaration {
    /// The type name (e.g., "meeting")
    pub type_name: String,
    
    /// The object ID (required for embedded types)
    pub id: Option<String>,
    
    /// Other field values
    pub fields: HashMap<String, FieldValue>,
    
    /// Line number where the declaration appears
    pub line: usize,
}

/// Parse a type declaration from a line
pub fn parse_type_declaration(line: &str, line_number: usize) -> Result<Option<TypeDeclaration>> {
    let trimmed = line.trim();
    
    if !trimmed.starts_with("::") {
        return Ok(None);
    }
    
    let caps = match TYPE_DECL_REGEX.captures(trimmed) {
        Some(c) => c,
        None => bail!("Invalid type declaration syntax: {}", trimmed),
    };
    
    let type_name = caps.get(1).unwrap().as_str().to_string();
    let args_str = caps.get(2).unwrap().as_str();
    
    let fields = parse_arguments(args_str)?;
    let id = fields.get("id").and_then(|v| v.as_str()).map(|s| s.to_string());
    
    Ok(Some(TypeDeclaration {
        type_name,
        id,
        fields,
        line: line_number,
    }))
}

/// Parse comma-separated key=value arguments
fn parse_arguments(args: &str) -> Result<HashMap<String, FieldValue>> {
    let mut fields = HashMap::new();
    
    if args.trim().is_empty() {
        return Ok(fields);
    }
    
    // Simple state machine to handle nested brackets and quotes
    let mut current_key = String::new();
    let mut current_value = String::new();
    let mut in_key = true;
    let mut in_quotes = false;
    let mut bracket_depth = 0;
    
    for c in args.chars() {
        match c {
            '"' if bracket_depth == 0 => {
                in_quotes = !in_quotes;
                current_value.push(c);
            }
            '[' if !in_quotes => {
                bracket_depth += 1;
                current_value.push(c);
            }
            ']' if !in_quotes => {
                bracket_depth -= 1;
                current_value.push(c);
            }
            '=' if !in_quotes && bracket_depth == 0 && in_key => {
                in_key = false;
            }
            ',' if !in_quotes && bracket_depth == 0 => {
                // End of argument
                let key = current_key.trim().to_string();
                let value = parse_value(current_value.trim())?;
                if !key.is_empty() {
                    fields.insert(key, value);
                }
                current_key.clear();
                current_value.clear();
                in_key = true;
            }
            _ => {
                if in_key {
                    current_key.push(c);
                } else {
                    current_value.push(c);
                }
            }
        }
    }
    
    // Handle last argument
    let key = current_key.trim().to_string();
    if !key.is_empty() {
        let value = parse_value(current_value.trim())?;
        fields.insert(key, value);
    }
    
    Ok(fields)
}

/// Parse a single value
fn parse_value(s: &str) -> Result<FieldValue> {
    let s = s.trim();
    
    if s.is_empty() {
        return Ok(FieldValue::Null);
    }
    
    // Reference [[...]] - exactly 2 opening and 2 closing brackets
    // Must check before array. Note: [[[ref]]] would be array of refs, not a ref.
    if s.starts_with("[[") && !s.starts_with("[[[") && s.ends_with("]]") {
        return Ok(FieldValue::Ref(s[2..s.len()-2].to_string()));
    }
    
    // Array (including array of refs like [[[a]], [[b]]])
    if s.starts_with('[') && s.ends_with(']') {
        let inner = &s[1..s.len()-1];
        let items = parse_array_items(inner)?;
        return Ok(FieldValue::Array(items));
    }
    
    // Quoted string
    if s.starts_with('"') && s.ends_with('"') && s.len() >= 2 {
        return Ok(FieldValue::String(s[1..s.len()-1].to_string()));
    }
    
    // Boolean
    if s == "true" {
        return Ok(FieldValue::Bool(true));
    }
    if s == "false" {
        return Ok(FieldValue::Bool(false));
    }
    
    // Number
    if let Ok(n) = s.parse::<f64>() {
        return Ok(FieldValue::Number(n));
    }
    
    // Date (YYYY-MM-DD) or datetime (YYYY-MM-DDTHH:MM)
    if s.len() >= 10 && s.chars().take(4).all(|c| c.is_ascii_digit()) {
        if s.contains('T') {
            return Ok(FieldValue::Datetime(s.to_string()));
        } else if s.len() == 10 {
            return Ok(FieldValue::Date(s.to_string()));
        }
    }
    
    // Time only (HH:MM)
    if s.len() == 5 && s.chars().nth(2) == Some(':') {
        return Ok(FieldValue::String(s.to_string()));
    }
    
    // Default to string
    Ok(FieldValue::String(s.to_string()))
}

/// Parse array items, handling nested references
fn parse_array_items(s: &str) -> Result<Vec<FieldValue>> {
    let mut items = Vec::new();
    let mut current = String::new();
    let mut bracket_depth = 0;
    let mut in_quotes = false;
    
    for c in s.chars() {
        match c {
            '"' => {
                in_quotes = !in_quotes;
                current.push(c);
            }
            '[' if !in_quotes => {
                bracket_depth += 1;
                current.push(c);
            }
            ']' if !in_quotes => {
                bracket_depth -= 1;
                current.push(c);
            }
            ',' if !in_quotes && bracket_depth == 0 => {
                let item = parse_value(current.trim())?;
                if !matches!(item, FieldValue::Null) {
                    items.push(item);
                }
                current.clear();
            }
            _ => {
                current.push(c);
            }
        }
    }
    
    // Handle last item
    if !current.trim().is_empty() {
        let item = parse_value(current.trim())?;
        if !matches!(item, FieldValue::Null) {
            items.push(item);
        }
    }
    
    Ok(items)
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_parse_simple_type_decl() {
        let result = parse_type_declaration("::meeting(id=standup, time=09:00)", 5).unwrap();
        let decl = result.unwrap();
        
        assert_eq!(decl.type_name, "meeting");
        assert_eq!(decl.id, Some("standup".to_string()));
        assert_eq!(decl.line, 5);
    }
    
    #[test]
    fn test_parse_type_with_refs() {
        let result = parse_type_declaration(
            "::meeting(id=standup, attendees=[[[people/alice]], [[people/bob]]])",
            1
        ).unwrap();
        let decl = result.unwrap();
        
        assert_eq!(decl.type_name, "meeting");
        let attendees = decl.fields.get("attendees").unwrap();
        if let FieldValue::Array(arr) = attendees {
            assert_eq!(arr.len(), 2);
            assert_eq!(arr[0], FieldValue::Ref("people/alice".to_string()));
        } else {
            panic!("Expected array");
        }
    }
    
    #[test]
    fn test_not_a_type_decl() {
        let result = parse_type_declaration("Some regular text", 1).unwrap();
        assert!(result.is_none());
    }
}
