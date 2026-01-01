//! Schema loader - parses schema.yaml

use anyhow::{Context, Result};
use std::path::Path;

use super::Schema;

/// Load schema from a schema.yaml file
pub fn load_schema(vault_path: &Path) -> Result<Schema> {
    let schema_path = vault_path.join("schema.yaml");
    
    if !schema_path.exists() {
        // Return default schema with just the 'page' type
        return Ok(Schema::default());
    }
    
    let contents = std::fs::read_to_string(&schema_path)
        .with_context(|| format!("Failed to read schema file: {}", schema_path.display()))?;
    
    let mut schema: Schema = serde_yaml::from_str(&contents)
        .with_context(|| format!("Failed to parse schema file: {}", schema_path.display()))?;
    
    // Ensure 'page' type exists as fallback
    if !schema.types.contains_key("page") {
        schema.types.insert("page".to_string(), Default::default());
    }
    
    // Ensure 'task' trait has built-in alias if not specified
    if let Some(task_trait) = schema.traits.get_mut("task") {
        if task_trait.cli.is_none() {
            task_trait.cli = Some(super::TraitCliConfig {
                alias: Some("tasks".to_string()),
                default_query: Some("status:todo OR status:in_progress".to_string()),
            });
        }
    }
    
    Ok(schema)
}

/// Create a default schema.yaml file
pub fn create_default_schema(vault_path: &Path) -> Result<()> {
    let schema_path = vault_path.join("schema.yaml");
    
    let default_schema = r#"# Raven Schema Configuration
# Define your types and traits here

types:
  # Example: person type
  # person:
  #   fields:
  #     name:
  #       type: string
  #       required: true
  #     email:
  #       type: string
  #   detect:
  #     path_pattern: "^people/"

  # Example: daily note type
  # daily:
  #   fields:
  #     date:
  #       type: date
  #       derived: from_filename
  #   detect:
  #     path_pattern: "^daily/\\d{4}-\\d{2}-\\d{2}\\.md$"

traits:
  task:
    fields:
      due:
        type: date
      priority:
        type: enum
        values: [low, medium, high]
        default: medium
      status:
        type: enum
        values: [todo, in_progress, done]
        default: todo
    cli:
      alias: tasks
      default_query: "status:todo OR status:in_progress"

  remind:
    fields:
      at:
        type: datetime
        positional: true

  highlight:
    fields:
      color:
        type: enum
        values: [yellow, red, green, blue]
        default: yellow
"#;
    
    std::fs::write(&schema_path, default_schema)
        .with_context(|| format!("Failed to write schema file: {}", schema_path.display()))?;
    
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;
    
    #[test]
    fn test_load_default_schema() {
        let dir = tempdir().unwrap();
        let schema = load_schema(dir.path()).unwrap();
        
        assert!(schema.types.contains_key("page"));
    }
    
    #[test]
    fn test_load_custom_schema() {
        let dir = tempdir().unwrap();
        let schema_content = r#"
types:
  person:
    fields:
      name:
        type: string
        required: true
traits:
  task:
    fields:
      due:
        type: date
"#;
        std::fs::write(dir.path().join("schema.yaml"), schema_content).unwrap();
        
        let schema = load_schema(dir.path()).unwrap();
        
        assert!(schema.types.contains_key("person"));
        assert!(schema.types.contains_key("page")); // Fallback added
        assert!(schema.traits.contains_key("task"));
    }
}
