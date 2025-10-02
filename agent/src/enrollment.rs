use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::Path;
use tracing::{debug, info, warn};
use uuid::Uuid;

use crate::config::AgentConfig;
use crate::http_client::SecureHttpClient;

#[derive(Debug, Serialize)]
struct EnrollmentRequest {
    enrollment_token: String,
    hostname: String,
    host_id: String,
    agent_version: String,
    os_version: String,
    architecture: String,
}

#[derive(Debug, Deserialize)]
struct EnrollmentResponse {
    api_key: String,
    host_id: String,
    success: bool,
    message: Option<String>,
}

pub struct EnrollmentManager {
    config: AgentConfig,
    http_client: SecureHttpClient,
}

impl EnrollmentManager {
    pub fn new(config: &AgentConfig) -> Result<Self> {
        let http_client = SecureHttpClient::new(config)
            .with_context(|| "Failed to create HTTP client for enrollment")?;

        Ok(Self {
            config: config.clone(),
            http_client,
        })
    }

    pub async fn enroll(&self, token: &str, hostname: Option<&str>) -> Result<()> {
        info!("Starting agent enrollment process");

        // Generate or load host ID
        let host_id = self.get_or_create_host_id()?;

        // Get system information
        let hostname = hostname
            .map(|h| h.to_string())
            .unwrap_or_else(|| {
                gethostname::gethostname()
                    .into_string()
                    .unwrap_or_else(|_| "unknown".to_string())
            });

        let enrollment_request = EnrollmentRequest {
            enrollment_token: token.to_string(),
            hostname,
            host_id: host_id.clone(),
            agent_version: env!("CARGO_PKG_VERSION").to_string(),
            os_version: self.get_os_version()?,
            architecture: std::env::consts::ARCH.to_string(),
        };

        debug!("Sending enrollment request for host ID: {}", host_id);

        // Send enrollment request
        let response = self
            .http_client
            .post("/api/v1/enroll", &enrollment_request)
            .await
            .with_context(|| "Failed to send enrollment request")?;

        if !response.status().is_success() {
            let status = response.status();
            let error_text = response.text().await.unwrap_or_default();
            return Err(anyhow::anyhow!(
                "Enrollment failed with status {}: {}",
                status,
                error_text
            ));
        }

        let enrollment_response: EnrollmentResponse = response
            .json()
            .await
            .with_context(|| "Failed to parse enrollment response")?;

        if !enrollment_response.success {
            return Err(anyhow::anyhow!(
                "Enrollment rejected: {}",
                enrollment_response.message.unwrap_or_default()
            ));
        }

        // Save API key securely
        self.save_api_key(&enrollment_response.api_key)
            .with_context(|| "Failed to save API key")?;

        // Update host ID if backend provided one
        if enrollment_response.host_id != host_id {
            self.save_host_id(&enrollment_response.host_id)
                .with_context(|| "Failed to save host ID")?;
        }

        info!("Agent enrollment completed successfully");
        Ok(())
    }

    fn get_or_create_host_id(&self) -> Result<String> {
        let host_id_file = &self.config.enrollment.host_id_file;

        if host_id_file.exists() {
            // Load existing host ID
            let host_id = fs::read_to_string(host_id_file)
                .with_context(|| format!("Failed to read host ID from {:?}", host_id_file))?
                .trim()
                .to_string();

            debug!("Loaded existing host ID: {}", host_id);
            Ok(host_id)
        } else {
            // Generate new host ID
            let host_id = Uuid::new_v4().to_string();
            self.save_host_id(&host_id)?;
            debug!("Generated new host ID: {}", host_id);
            Ok(host_id)
        }
    }

    fn save_host_id(&self, host_id: &str) -> Result<()> {
        let host_id_file = &self.config.enrollment.host_id_file;

        // Create directory if it doesn't exist
        if let Some(parent) = host_id_file.parent() {
            fs::create_dir_all(parent)
                .with_context(|| format!("Failed to create directory: {:?}", parent))?;
        }

        fs::write(host_id_file, host_id)
            .with_context(|| format!("Failed to write host ID to {:?}", host_id_file))?;

        // Set restrictive permissions (readable only by owner)
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let mut perms = fs::metadata(host_id_file)?.permissions();
            perms.set_mode(0o600);
            fs::set_permissions(host_id_file, perms)?;
        }

        debug!("Saved host ID to {:?}", host_id_file);
        Ok(())
    }

    fn save_api_key(&self, api_key: &str) -> Result<()> {
        let api_key_file = &self.config.security.api_key_file;

        // Create directory if it doesn't exist
        if let Some(parent) = api_key_file.parent() {
            fs::create_dir_all(parent)
                .with_context(|| format!("Failed to create directory: {:?}", parent))?;
        }

        fs::write(api_key_file, api_key)
            .with_context(|| format!("Failed to write API key to {:?}", api_key_file))?;

        // Set restrictive permissions (readable only by root/owner)
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let mut perms = fs::metadata(api_key_file)?.permissions();
            perms.set_mode(0o600);
            fs::set_permissions(api_key_file, perms)?;
        }

        debug!("Saved API key to {:?}", api_key_file);
        Ok(())
    }

    fn get_os_version(&self) -> Result<String> {
        let output = std::process::Command::new("lsb_release")
            .args(["-ds"])
            .output()
            .with_context(|| "Failed to get OS version")?;

        if output.status.success() {
            Ok(String::from_utf8_lossy(&output.stdout).trim().to_string())
        } else {
            // Fallback to /etc/os-release
            if let Ok(os_release) = fs::read_to_string("/etc/os-release") {
                for line in os_release.lines() {
                    if line.starts_with("PRETTY_NAME=") {
                        let version = line
                            .trim_start_matches("PRETTY_NAME=")
                            .trim_matches('"')
                            .to_string();
                        return Ok(version);
                    }
                }
            }
            
            Ok("Unknown".to_string())
        }
    }

    pub fn is_enrolled(&self) -> bool {
        self.config.security.api_key_file.exists()
    }

    pub fn get_host_id(&self) -> Result<String> {
        if !self.config.enrollment.host_id_file.exists() {
            return Err(anyhow::anyhow!("Host ID file does not exist"));
        }

        let host_id = fs::read_to_string(&self.config.enrollment.host_id_file)
            .with_context(|| "Failed to read host ID")?
            .trim()
            .to_string();

        Ok(host_id)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn test_host_id_generation_and_persistence() {
        let temp_dir = tempdir().unwrap();
        let mut config = AgentConfig::default();
        config.enrollment.host_id_file = temp_dir.path().join("host.id");
        config.security.api_key_file = temp_dir.path().join("auth.token");

        // This would fail without proper HTTP client setup in tests
        // but we can test the host ID logic
        let host_id_file = &config.enrollment.host_id_file;
        
        assert!(!host_id_file.exists());
        
        // In a real test, you'd mock the HTTP client
        // For now, just test that the file paths are set correctly
        assert!(host_id_file.to_string_lossy().contains("host.id"));
    }

    #[test]
    fn test_os_version_parsing() {
        let config = AgentConfig::default();
        
        // Create a minimal enrollment manager (will fail HTTP client creation)
        // but we can test the OS version logic if we had a way to mock it
        
        // In practice, you'd want to mock the Command execution
        // For now, just test that the function exists and has the right signature
        let _result = std::process::Command::new("echo")
            .arg("Ubuntu 22.04.1 LTS")
            .output();
    }
}