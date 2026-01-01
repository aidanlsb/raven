//! Global configuration for Raven

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::path::PathBuf;

/// Global Raven configuration
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Config {
    /// Default vault path
    #[serde(default)]
    pub vault: Option<PathBuf>,
    
    /// Editor to use for opening files
    #[serde(default)]
    pub editor: Option<String>,
}

impl Config {
    /// Load config from default location (~/.config/raven/config.toml)
    pub fn load() -> Result<Self> {
        let config_path = Self::default_path();
        
        if !config_path.exists() {
            return Ok(Self::default());
        }
        
        let contents = std::fs::read_to_string(&config_path)
            .with_context(|| format!("Failed to read config: {}", config_path.display()))?;
        
        let config: Config = toml::from_str(&contents)
            .with_context(|| format!("Failed to parse config: {}", config_path.display()))?;
        
        Ok(config)
    }
    
    /// Load config from a specific path
    pub fn load_from(path: &PathBuf) -> Result<Self> {
        let contents = std::fs::read_to_string(path)
            .with_context(|| format!("Failed to read config: {}", path.display()))?;
        
        let config: Config = toml::from_str(&contents)
            .with_context(|| format!("Failed to parse config: {}", path.display()))?;
        
        Ok(config)
    }
    
    /// Get default config file path
    /// Checks ~/.config/raven/config.toml first (XDG style), 
    /// then falls back to OS-specific location
    pub fn default_path() -> PathBuf {
        // Prefer XDG-style ~/.config/raven/config.toml
        if let Some(home) = dirs::home_dir() {
            let xdg_path = home.join(".config").join("raven").join("config.toml");
            if xdg_path.exists() {
                return xdg_path;
            }
        }
        
        // Fall back to OS-specific config dir
        dirs::config_dir()
            .unwrap_or_else(|| PathBuf::from("."))
            .join("raven")
            .join("config.toml")
    }
    
    /// Get the XDG-style config path (~/.config/raven/config.toml)
    pub fn xdg_path() -> Option<PathBuf> {
        dirs::home_dir().map(|h| h.join(".config").join("raven").join("config.toml"))
    }
    
    /// Create default config file if it doesn't exist
    pub fn create_default() -> Result<PathBuf> {
        let config_path = Self::default_path();
        
        if config_path.exists() {
            return Ok(config_path);
        }
        
        // Create parent directories
        if let Some(parent) = config_path.parent() {
            std::fs::create_dir_all(parent)?;
        }
        
        let default_config = r#"# Raven Configuration
# See: https://github.com/yourusername/raven

# Default vault path (uncomment and set your path)
# vault = "/path/to/your/vault"

# Editor for opening files (defaults to $EDITOR)
# editor = "code"
"#;
        
        std::fs::write(&config_path, default_config)?;
        
        Ok(config_path)
    }
}
