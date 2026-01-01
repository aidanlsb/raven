use anyhow::{Context, Result};
use clap::{Parser, Subcommand};
use std::path::PathBuf;

mod cli;
mod config;
mod schema;
mod parser;
mod index;

use config::Config;

#[derive(Parser)]
#[command(name = "rvn")]
#[command(author, version, about = "Raven - A personal knowledge system")]
struct Cli {
    /// Path to the vault directory
    #[arg(long, global = true)]
    vault: Option<PathBuf>,

    /// Path to config file
    #[arg(long, global = true)]
    config: Option<PathBuf>,

    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Initialize a new vault
    Init {
        /// Path to create the vault
        path: PathBuf,
    },

    /// Validate the vault (check for errors)
    Check {
        /// Treat warnings as errors
        #[arg(long)]
        strict: bool,
    },

    /// Reindex all files
    Reindex,

    /// List tasks
    Tasks {
        /// Filter by status
        #[arg(long)]
        status: Option<String>,

        /// Filter by due date
        #[arg(long)]
        due: Option<String>,

        /// Show all tasks including completed
        #[arg(long)]
        all: bool,
    },

    /// Query traits
    Trait {
        /// Trait name to query
        name: String,

        /// Additional filters (key=value)
        #[arg(trailing_var_arg = true)]
        filters: Vec<String>,
    },

    /// Query objects
    Query {
        /// Query string
        query: String,
    },

    /// Show backlinks to an object
    Backlinks {
        /// Target object ID
        target: String,
    },

    /// Show index statistics
    Stats,

    /// List untyped pages
    Untyped,

    /// Open or create today's daily note
    Daily,

    /// Create a new typed note
    New {
        /// Type of note to create
        #[arg(long, short = 't')]
        r#type: String,

        /// Title of the note
        title: String,
    },
}

fn main() -> Result<()> {
    let cli = Cli::parse();

    // Load config
    let config = if let Some(config_path) = &cli.config {
        Config::load_from(config_path)?
    } else {
        Config::load().unwrap_or_default()
    };

    // Resolve vault path: CLI flag > config file (no fallback to cwd)
    let vault_path = cli.vault
        .or(config.vault)
        .ok_or_else(|| anyhow::anyhow!(
            "No vault specified.\n\n\
            Either:\n  \
            1. Use --vault /path/to/vault\n  \
            2. Set 'vault = \"/path/to/vault\"' in ~/.config/raven/config.toml\n  \
            3. Run 'rvn init /path/to/new/vault' to create one"
        ))?;
    
    // Verify the vault path exists (except for init command)
    if !matches!(cli.command, Commands::Init { .. }) && !vault_path.exists() {
        anyhow::bail!(
            "Vault not found: {}\n\nRun 'rvn init {}' to create it.",
            vault_path.display(),
            vault_path.display()
        );
    }

    match cli.command {
        Commands::Init { path } => cli::commands::init(&path),
        Commands::Check { strict } => cli::commands::check(&vault_path, strict),
        Commands::Reindex => cli::commands::reindex(&vault_path),
        Commands::Tasks { status, due, all } => cli::commands::tasks(&vault_path, status, due, all),
        Commands::Trait { name, filters } => cli::commands::query_trait(&vault_path, &name, &filters),
        Commands::Query { query } => cli::commands::query(&vault_path, &query),
        Commands::Backlinks { target } => cli::commands::backlinks(&vault_path, &target),
        Commands::Stats => cli::commands::stats(&vault_path),
        Commands::Untyped => cli::commands::untyped(&vault_path),
        Commands::Daily => cli::commands::daily(&vault_path),
        Commands::New { r#type, title } => cli::commands::new_note(&vault_path, &r#type, &title),
    }
}
