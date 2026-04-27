use anyhow::{Context, Result};
use std::path::Path;
use tracing::Level;
use tracing_subscriber::{
    fmt::{self, MakeWriter},
    layer::SubscriberExt,
    util::SubscriberInitExt,
    Layer,
    EnvFilter, Registry
};

use crate::config::LoggingConfig;

pub fn setup_logging(config: &LoggingConfig) -> Result<()> {
    let level = parse_log_level(&config.level)?;
    
    // Create base filter
    let env_filter = EnvFilter::builder()
        .with_default_directive(level.into())
        .from_env_lossy()
        .add_directive("reqwest=warn".parse().unwrap()) // Reduce reqwest verbosity
        .add_directive("rustls=warn".parse().unwrap()); // Reduce TLS verbosity

    let subscriber = Registry::default().with(env_filter);

    match config.format.as_str() {
        "json" => {
            let fmt_layer = fmt::layer()
                .json()
                .with_current_span(true)
                .with_span_list(true);
            
            if let Some(log_file) = &config.file {
                setup_file_logging(subscriber, fmt_layer, log_file, config)?;
            } else {
                subscriber.with(fmt_layer).init();
            }
        }
        "text" => {
            let fmt_layer = fmt::layer()
                .with_target(true)
                .with_thread_ids(true)
                .with_file(false)
                .with_line_number(false);
            
            if let Some(log_file) = &config.file {
                setup_file_logging(subscriber, fmt_layer, log_file, config)?;
            } else {
                subscriber.with(fmt_layer).init();
            }
        }
        _ => {
            return Err(anyhow::anyhow!("Unsupported log format: {}", config.format));
        }
    }

    // Log startup message
    tracing::info!(
        "Logging initialized: level={}, format={}, file={:?}",
        config.level,
        config.format,
        config.file
    );

    Ok(())
}

fn setup_file_logging<S, F>(
    subscriber: S,
    fmt_layer: F,
    log_file: &Path,
    config: &LoggingConfig,
) -> Result<()> 
where
    S: tracing_subscriber::layer::SubscriberExt + Send + Sync,
    F: tracing_subscriber::Layer<S> + Send + Sync,
{
    // Create log directory if it doesn't exist
    if let Some(parent) = log_file.parent() {
        std::fs::create_dir_all(parent)
            .with_context(|| format!("Failed to create log directory: {:?}", parent))?;
    }

    // Setup file appender with rotation
    let file_appender = tracing_appender::rolling::Builder::new()
        .rotation(tracing_appender::rolling::Rotation::DAILY)
        .filename_prefix("agent")
        .filename_suffix("log")
        .max_log_files(config.max_files as usize)
        .build(log_file.parent().unwrap_or_else(|| Path::new(".")))
        .with_context(|| "Failed to create file appender")?;

    let (non_blocking, _guard) = tracing_appender::non_blocking(file_appender);

    // Create file layer
    let file_layer = fmt_layer.with_writer(non_blocking);
    
    // Also log to stdout for systemd/container environments
    let stdout_layer = match config.format.as_str() {
        "json" => fmt::layer()
            .json()
            .with_current_span(false)
            .with_span_list(false)
            .boxed(),
        _ => fmt::layer()
            .with_target(false)
            .with_thread_ids(false)
            .compact()
            .boxed(),
    };

    subscriber
        .with(file_layer)
        .with(stdout_layer)
        .init();

    // Store guard to prevent dropping (would stop file logging)
    std::mem::forget(_guard);

    Ok(())
}

fn parse_log_level(level: &str) -> Result<Level> {
    match level.to_lowercase().as_str() {
        "trace" => Ok(Level::TRACE),
        "debug" => Ok(Level::DEBUG),
        "info" => Ok(Level::INFO),
        "warn" | "warning" => Ok(Level::WARN),
        "error" => Ok(Level::ERROR),
        _ => Err(anyhow::anyhow!("Invalid log level: {}", level)),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_log_level() {
        assert_eq!(parse_log_level("trace").unwrap(), Level::TRACE);
        assert_eq!(parse_log_level("DEBUG").unwrap(), Level::DEBUG);
        assert_eq!(parse_log_level("Info").unwrap(), Level::INFO);
        assert_eq!(parse_log_level("warn").unwrap(), Level::WARN);
        assert_eq!(parse_log_level("warning").unwrap(), Level::WARN);
        assert_eq!(parse_log_level("error").unwrap(), Level::ERROR);
        
        assert!(parse_log_level("invalid").is_err());
    }

    #[test]
    fn test_logging_config_validation() {
        let config = LoggingConfig {
            level: "info".to_string(),
            format: "json".to_string(),
            file: None,
            max_size_mb: 100,
            max_files: 5,
        };

        // Test that we can parse the level
        assert!(parse_log_level(&config.level).is_ok());
        
        // Test format validation would happen in setup_logging
        assert!(matches!(config.format.as_str(), "json" | "text"));
    }
}