use anyhow::{Context, Result};
use std::path::Path;
use tracing::Level;
use tracing_subscriber::{fmt, layer::SubscriberExt, util::SubscriberInitExt, EnvFilter, Registry};

use crate::config::LoggingConfig;

pub fn setup_logging(config: &LoggingConfig) -> Result<()> {
    let level = parse_log_level(&config.level)?;

    let env_filter = EnvFilter::builder()
        .with_default_directive(level.into())
        .from_env_lossy()
        .add_directive("reqwest=warn".parse().unwrap())
        .add_directive("rustls=warn".parse().unwrap());

    let subscriber = Registry::default().with(env_filter);

    // Compose the per-format layers inline. The earlier helper used a
    // generic `F: Layer<S>`, which is too loose to call `.with_writer()`
    // on — that's a method on `fmt::Layer`, not on `Layer` in general.
    // Inlining keeps the typing trivial and the layer composition explicit.
    match config.format.as_str() {
        "json" => {
            if let Some(log_file) = &config.file {
                let non_blocking = make_file_writer(log_file, config)?;
                let file_layer = fmt::layer()
                    .json()
                    .with_current_span(true)
                    .with_span_list(true)
                    .with_writer(non_blocking);
                let stdout_layer = fmt::layer()
                    .json()
                    .with_current_span(false)
                    .with_span_list(false);
                subscriber.with(file_layer).with(stdout_layer).init();
            } else {
                let fmt_layer = fmt::layer()
                    .json()
                    .with_current_span(true)
                    .with_span_list(true);
                subscriber.with(fmt_layer).init();
            }
        }
        "text" => {
            if let Some(log_file) = &config.file {
                let non_blocking = make_file_writer(log_file, config)?;
                let file_layer = fmt::layer()
                    .with_target(true)
                    .with_thread_ids(true)
                    .with_file(false)
                    .with_line_number(false)
                    .with_writer(non_blocking);
                let stdout_layer = fmt::layer()
                    .with_target(false)
                    .with_thread_ids(false)
                    .compact();
                subscriber.with(file_layer).with(stdout_layer).init();
            } else {
                let fmt_layer = fmt::layer()
                    .with_target(true)
                    .with_thread_ids(true)
                    .with_file(false)
                    .with_line_number(false);
                subscriber.with(fmt_layer).init();
            }
        }
        _ => return Err(anyhow::anyhow!("Unsupported log format: {}", config.format)),
    }

    tracing::info!(
        "Logging initialized: level={}, format={}, file={:?}",
        config.level,
        config.format,
        config.file
    );

    Ok(())
}

// make_file_writer builds a rolling daily file appender plus a non-blocking
// writer, then leaks the worker guard so the background flush thread keeps
// running for the life of the process. (Returning the guard would force every
// caller into managing its lifetime.)
fn make_file_writer(
    log_file: &Path,
    config: &LoggingConfig,
) -> Result<tracing_appender::non_blocking::NonBlocking> {
    if let Some(parent) = log_file.parent() {
        std::fs::create_dir_all(parent)
            .with_context(|| format!("Failed to create log directory: {:?}", parent))?;
    }

    let file_appender = tracing_appender::rolling::Builder::new()
        .rotation(tracing_appender::rolling::Rotation::DAILY)
        .filename_prefix("agent")
        .filename_suffix("log")
        .max_log_files(config.max_files as usize)
        .build(log_file.parent().unwrap_or_else(|| Path::new(".")))
        .with_context(|| "Failed to create file appender")?;

    let (non_blocking, guard) = tracing_appender::non_blocking(file_appender);
    std::mem::forget(guard);
    Ok(non_blocking)
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

        assert!(parse_log_level(&config.level).is_ok());
        assert!(matches!(config.format.as_str(), "json" | "text"));
    }
}
