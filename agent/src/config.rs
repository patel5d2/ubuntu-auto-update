use anyhow::{Context, Result};
use config::{Config, ConfigError, Environment, File};
use serde::{Deserialize, Serialize};
use std::path::PathBuf;
use zeroize::{Zeroize, ZeroizeOnDrop};

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct AgentConfig {
    pub backend: BackendConfig,
    pub security: SecurityConfig,
    pub updates: UpdateConfig,
    pub logging: LoggingConfig,
    pub metrics: MetricsConfig,
    pub enrollment: EnrollmentConfig,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct BackendConfig {
    pub url: String,
    pub timeout_seconds: u64,
    pub retry_attempts: u32,
    pub retry_delay_seconds: u64,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct SecurityConfig {
    pub api_key_file: PathBuf,
    pub cert_file: Option<PathBuf>,
    pub key_file: Option<PathBuf>,
    pub ca_file: Option<PathBuf>,
    pub hmac_secret_file: Option<PathBuf>,
    pub verify_server_cert: bool,
    pub use_mtls: bool,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct UpdateConfig {
    pub dry_run: bool,
    pub auto_reboot: bool,
    pub reboot_delay_minutes: u32,
    pub maintenance_window_start: Option<String>,
    pub maintenance_window_end: Option<String>,
    pub excluded_packages: Vec<String>,
    pub update_sources: UpdateSources,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct UpdateSources {
    pub apt: bool,
    pub snap: bool,
    pub flatpak: bool,
    pub firmware: bool,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct LoggingConfig {
    pub level: String,
    pub format: String, // "json" or "text"
    pub file: Option<PathBuf>,
    pub max_size_mb: u64,
    pub max_files: u32,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct MetricsConfig {
    pub enabled: bool,
    pub port: Option<u16>,
    pub textfile_path: Option<PathBuf>,
    pub collect_system_metrics: bool,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct EnrollmentConfig {
    pub token_file: PathBuf,
    pub host_id_file: PathBuf,
    pub enrollment_url: String,
}

impl Default for AgentConfig {
    fn default() -> Self {
        Self {
            backend: BackendConfig {
                url: "http://localhost:8080".to_string(),
                timeout_seconds: 30,
                retry_attempts: 3,
                retry_delay_seconds: 5,
            },
            security: SecurityConfig {
                api_key_file: PathBuf::from("/etc/ubuntu-auto-update/auth.token"),
                cert_file: None,
                key_file: None,
                ca_file: None,
                hmac_secret_file: Some(PathBuf::from("/etc/ubuntu-auto-update/hmac.key")),
                verify_server_cert: true,
                use_mtls: false,
            },
            updates: UpdateConfig {
                dry_run: false,
                auto_reboot: false,
                reboot_delay_minutes: 5,
                maintenance_window_start: None,
                maintenance_window_end: None,
                excluded_packages: vec![],
                update_sources: UpdateSources {
                    apt: true,
                    snap: true,
                    flatpak: false,
                    firmware: false,
                },
            },
            logging: LoggingConfig {
                level: "info".to_string(),
                format: "json".to_string(),
                file: Some(PathBuf::from("/var/log/ubuntu-auto-update/agent.log")),
                max_size_mb: 100,
                max_files: 5,
            },
            metrics: MetricsConfig {
                enabled: true,
                port: Some(9100),
                textfile_path: Some(PathBuf::from("/var/lib/node_exporter/textfile_collector")),
                collect_system_metrics: true,
            },
            enrollment: EnrollmentConfig {
                token_file: PathBuf::from("/etc/ubuntu-auto-update/enrollment.token"),
                host_id_file: PathBuf::from("/etc/ubuntu-auto-update/host.id"),
                enrollment_url: "http://localhost:8080/api/v1/enroll".to_string(),
            },
        }
    }
}

impl AgentConfig {
    pub fn load() -> Result<Self> {
        let config_paths = [
            "/etc/ubuntu-auto-update/agent.toml",
            "/etc/ubuntu-auto-update/agent.yaml",
            "./agent.toml",
            "./agent.yaml",
        ];

        let mut builder = Config::builder();

        // Try to load from config files
        for path in &config_paths {
            if std::path::Path::new(path).exists() {
                builder = builder.add_source(File::with_name(path).required(false));
                tracing::info!("Loading configuration from {}", path);
            }
        }

        // Override with environment variables
        builder = builder.add_source(
            Environment::with_prefix("UA")
                .prefix_separator("_")
                .separator("__"),
        );

        let config = builder.build()?;
        let agent_config: AgentConfig = config.try_deserialize()?;

        Ok(agent_config)
    }

    pub fn validate(&self) -> Result<(), ConfigError> {
        // Validate URLs
        if self.backend.url.is_empty() {
            return Err(ConfigError::Message("Backend URL cannot be empty".to_string()));
        }

        // Validate timeouts
        if self.backend.timeout_seconds == 0 {
            return Err(ConfigError::Message("Backend timeout must be > 0".to_string()));
        }

        // Validate log level
        if !["trace", "debug", "info", "warn", "error"].contains(&self.logging.level.as_str()) {
            return Err(ConfigError::Message(format!(
                "Invalid log level: {}",
                self.logging.level
            )));
        }

        // Validate log format
        if !["json", "text"].contains(&self.logging.format.as_str()) {
            return Err(ConfigError::Message(format!(
                "Invalid log format: {}",
                self.logging.format
            )));
        }

        Ok(())
    }

    pub fn save_default_config(path: &str) -> Result<()> {
        let config = AgentConfig::default();
        let toml_string = toml::to_string(&config)?;
        std::fs::write(path, toml_string)?;
        Ok(())
    }

    pub fn load_from_file(path: &std::path::Path) -> Result<Self> {
        let content = std::fs::read_to_string(path)
            .with_context(|| format!("Failed to read config file: {:?}", path))?;
        let config: AgentConfig = toml::from_str(&content)
            .with_context(|| format!("Failed to parse config file: {:?}", path))?;
        config.validate()?;
        Ok(config)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::NamedTempFile;

    #[test]
    fn test_default_config_validation() {
        let config = AgentConfig::default();
        assert!(config.validate().is_ok());
    }

    #[test]
    fn test_config_serialization() {
        let config = AgentConfig::default();
        let serialized = toml::to_string(&config).unwrap();
        let deserialized: AgentConfig = toml::from_str(&serialized).unwrap();
        
        assert_eq!(config.backend.url, deserialized.backend.url);
        assert_eq!(config.logging.level, deserialized.logging.level);
    }

    #[test]
    fn test_invalid_log_level() {
        let mut config = AgentConfig::default();
        config.logging.level = "invalid".to_string();
        assert!(config.validate().is_err());
    }
}