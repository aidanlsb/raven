//! Schema validator - validates field values against schema definitions

use anyhow::{bail, Result};
use std::collections::HashMap;

use super::{FieldDefinition, FieldType, FieldValue, Schema};

/// Validation error with context
#[derive(Debug, Clone)]
pub struct ValidationError {
    pub field: String,
    pub message: String,
}

impl std::fmt::Display for ValidationError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "Field '{}': {}", self.field, self.message)
    }
}

/// Validate a set of fields against a type's field definitions
pub fn validate_fields(
    fields: &HashMap<String, FieldValue>,
    field_defs: &HashMap<String, FieldDefinition>,
    _schema: &Schema,
) -> Vec<ValidationError> {
    let mut errors = Vec::new();
    
    // Check required fields are present
    for (name, def) in field_defs {
        if def.required && !fields.contains_key(name) && def.default.is_none() {
            errors.push(ValidationError {
                field: name.clone(),
                message: "Required field is missing".to_string(),
            });
        }
    }
    
    // Validate each provided field
    for (name, value) in fields {
        // Skip reserved fields
        if name == "id" || name == "type" || name == "tags" {
            continue;
        }
        
        if let Some(def) = field_defs.get(name) {
            if let Err(e) = validate_field_value(name, value, def) {
                errors.push(ValidationError {
                    field: name.clone(),
                    message: e.to_string(),
                });
            }
        }
        // Note: Unknown fields are allowed (schema is not strict)
    }
    
    errors
}

/// Validate a single field value against its definition
fn validate_field_value(
    _name: &str,
    value: &FieldValue,
    def: &FieldDefinition,
) -> Result<()> {
    match (&def.field_type, value) {
        // String types
        (FieldType::String, FieldValue::String(_)) => Ok(()),
        (FieldType::String, _) => bail!("Expected string"),
        
        // String array
        (FieldType::StringArray, FieldValue::Array(arr)) => {
            for v in arr {
                if !matches!(v, FieldValue::String(_)) {
                    bail!("Expected array of strings");
                }
            }
            Ok(())
        }
        (FieldType::StringArray, _) => bail!("Expected array of strings"),
        
        // Number types
        (FieldType::Number, FieldValue::Number(n)) => {
            if let Some(min) = def.min {
                if *n < min {
                    bail!("Value {} is below minimum {}", n, min);
                }
            }
            if let Some(max) = def.max {
                if *n > max {
                    bail!("Value {} is above maximum {}", n, max);
                }
            }
            Ok(())
        }
        (FieldType::Number, _) => bail!("Expected number"),
        
        // Number array
        (FieldType::NumberArray, FieldValue::Array(arr)) => {
            for v in arr {
                if !matches!(v, FieldValue::Number(_)) {
                    bail!("Expected array of numbers");
                }
            }
            Ok(())
        }
        (FieldType::NumberArray, _) => bail!("Expected array of numbers"),
        
        // Date types
        (FieldType::Date, FieldValue::Date(_)) => Ok(()),
        (FieldType::Date, FieldValue::String(s)) => {
            if chrono::NaiveDate::parse_from_str(s, "%Y-%m-%d").is_ok() {
                Ok(())
            } else {
                bail!("Invalid date format, expected YYYY-MM-DD")
            }
        }
        (FieldType::Date, _) => bail!("Expected date"),
        
        // Date array
        (FieldType::DateArray, FieldValue::Array(arr)) => {
            for v in arr {
                if !matches!(v, FieldValue::Date(_)) {
                    bail!("Expected array of dates");
                }
            }
            Ok(())
        }
        (FieldType::DateArray, _) => bail!("Expected array of dates"),
        
        // Datetime
        (FieldType::Datetime, FieldValue::Datetime(_)) => Ok(()),
        (FieldType::Datetime, FieldValue::String(s)) => {
            if chrono::DateTime::parse_from_rfc3339(s).is_ok() 
                || chrono::NaiveDateTime::parse_from_str(s, "%Y-%m-%dT%H:%M").is_ok()
                || chrono::NaiveDateTime::parse_from_str(s, "%Y-%m-%dT%H:%M:%S").is_ok()
            {
                Ok(())
            } else {
                bail!("Invalid datetime format")
            }
        }
        (FieldType::Datetime, _) => bail!("Expected datetime"),
        
        // Enum
        (FieldType::Enum, FieldValue::String(s)) => {
            if let Some(values) = &def.values {
                if values.contains(s) {
                    Ok(())
                } else {
                    bail!("Invalid enum value '{}', expected one of: {:?}", s, values)
                }
            } else {
                bail!("Enum type missing 'values' definition")
            }
        }
        (FieldType::Enum, _) => bail!("Expected enum value (string)"),
        
        // Bool
        (FieldType::Bool, FieldValue::Bool(_)) => Ok(()),
        (FieldType::Bool, _) => bail!("Expected boolean"),
        
        // Ref
        (FieldType::Ref, FieldValue::Ref(_)) => Ok(()),
        (FieldType::Ref, FieldValue::String(_)) => Ok(()), // Allow string as ref
        (FieldType::Ref, _) => bail!("Expected reference"),
        
        // Ref array
        (FieldType::RefArray, FieldValue::Array(arr)) => {
            for v in arr {
                if !matches!(v, FieldValue::Ref(_) | FieldValue::String(_)) {
                    bail!("Expected array of references");
                }
            }
            Ok(())
        }
        (FieldType::RefArray, _) => bail!("Expected array of references"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_validate_required_field() {
        let mut field_defs = HashMap::new();
        field_defs.insert("name".to_string(), FieldDefinition {
            field_type: FieldType::String,
            required: true,
            default: None,
            values: None,
            target: None,
            min: None,
            max: None,
            derived: None,
            positional: false,
        });
        
        let fields = HashMap::new();
        let schema = Schema::default();
        
        let errors = validate_fields(&fields, &field_defs, &schema);
        assert_eq!(errors.len(), 1);
        assert_eq!(errors[0].field, "name");
    }
    
    #[test]
    fn test_validate_enum_field() {
        let mut field_defs = HashMap::new();
        field_defs.insert("status".to_string(), FieldDefinition {
            field_type: FieldType::Enum,
            required: false,
            default: None,
            values: Some(vec!["active".to_string(), "done".to_string()]),
            target: None,
            min: None,
            max: None,
            derived: None,
            positional: false,
        });
        
        let mut fields = HashMap::new();
        fields.insert("status".to_string(), FieldValue::String("invalid".to_string()));
        
        let schema = Schema::default();
        
        let errors = validate_fields(&fields, &field_defs, &schema);
        assert_eq!(errors.len(), 1);
    }
}
