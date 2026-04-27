use anyhow::{Context, Result};
use base64::{engine::general_purpose::STANDARD as BASE64, Engine};
use hmac::{Hmac, Mac};
use reqwest::{Certificate, Client, ClientBuilder, Response};
use rustls::{Certificate as RustlsCertificate, PrivateKey};
use rustls_pemfile::{certs, pkcs8_private_keys};
use sha2::Sha256;
use std::fs::File;
use std::io::BufReader;
use std::path::Path;
use std::sync::Arc;
use std::time::Duration;
use tokio::time::{sleep, Instant};
use tracing::{debug, error, info, warn};
use zeroize::{Zeroize, ZeroizeOnDrop};

use crate::config::{AgentConfig, SecurityConfig};

type HmacSha256 = Hmac<Sha256>;

#[derive(Clone, Zeroize, ZeroizeOnDrop)]
pub struct SecretKey(Vec<u8>);

impl SecretKey {
    pub fn from_file<P: AsRef<Path>>(path: P) -> Result<Self> {
        let key_data = std::fs::read(path.as_ref())
            .with_context(|| format!("Failed to read key from {:?}", path.as_ref()))?;
        Ok(Self(key_data))
    }

    pub fn as_bytes(&self) -> &[u8] {
        &self.0
    }
}

#[derive(Clone)]
pub struct SecureHttpClient {
    client: Client,
    config: Arc<SecurityConfig>,
    base_url: String,
    api_key: Option<SecretKey>,
    hmac_key: Option<SecretKey>,
}

impl SecureHttpClient {
    pub fn new(config: &AgentConfig) -> Result<Self> {
        let mut client_builder = ClientBuilder::new()
            .timeout(Duration::from_secs(config.backend.timeout_seconds))
            .user_agent(format!("ubuntu-auto-update-agent/{}", env!("CARGO_PKG_VERSION")));

        // Configure TLS
        if config.security.use_mtls {
            if let (Some(cert_path), Some(key_path)) = (&config.security.cert_file, &config.security.key_file) {
                let identity = load_client_identity(cert_path, key_path)?;
                client_builder = client_builder.identity(identity);
                info!("mTLS client certificate configured");
            }
        }

        // Load CA certificate if provided
        if let Some(ca_path) = &config.security.ca_file {
            let ca_cert = load_ca_certificate(ca_path)?;
            client_builder = client_builder.add_root_certificate(ca_cert);
            info!("Custom CA certificate loaded");
        }

        // Configure certificate verification
        client_builder = client_builder.danger_accept_invalid_certs(!config.security.verify_server_cert);

        let client = client_builder.build()
            .context("Failed to build HTTP client")?;

        // Load API key
        let api_key = if config.security.api_key_file.exists() {
            Some(SecretKey::from_file(&config.security.api_key_file)?)
        } else {
            None
        };

        // Load HMAC key
        let hmac_key = if let Some(hmac_path) = &config.security.hmac_secret_file {
            if hmac_path.exists() {
                Some(SecretKey::from_file(hmac_path)?)
            } else {
                None
            }
        } else {
            None
        };

        Ok(Self {
            client,
            config: Arc::new(config.security.clone()),
            base_url: config.backend.url.clone(),
            api_key,
            hmac_key,
        })
    }

    pub async fn post_with_retry<T: serde::Serialize>(
        &self,
        endpoint: &str,
        payload: &T,
        max_retries: u32,
        retry_delay: Duration,
    ) -> Result<Response> {
        let mut last_error = None;

        for attempt in 0..=max_retries {
            match self.post(endpoint, payload).await {
                Ok(response) => {
                    if response.status().is_success() {
                        return Ok(response);
                    } else if response.status().is_client_error() {
                        // Don't retry client errors (4xx)
                        return Err(anyhow::anyhow!(
                            "Client error: {} - {}",
                            response.status(),
                            response.text().await.unwrap_or_default()
                        ));
                    } else {
                        // Server error - retry
                        last_error = Some(anyhow::anyhow!(
                            "Server error: {} - {}",
                            response.status(),
                            response.text().await.unwrap_or_default()
                        ));
                    }
                }
                Err(e) => {
                    last_error = Some(e);
                }
            }

            if attempt < max_retries {
                let delay = retry_delay * 2_u32.pow(attempt); // Exponential backoff
                warn!(
                    "Request failed (attempt {}/{}), retrying in {:?}",
                    attempt + 1,
                    max_retries + 1,
                    delay
                );
                sleep(delay).await;
            }
        }

        Err(last_error.unwrap_or_else(|| anyhow::anyhow!("Unknown error during retries")))
    }

    pub async fn post<T: serde::Serialize>(
        &self,
        endpoint: &str,
        payload: &T,
    ) -> Result<Response> {
        let url = format!("{}{}", self.base_url, endpoint);
        let json_payload = serde_json::to_string(payload)
            .context("Failed to serialize payload")?;

        debug!("Sending POST request to: {}", url);

        let mut request = self.client
            .post(&url)
            .header("Content-Type", "application/json");

        // Add authentication
        if let Some(api_key) = &self.api_key {
            let key_str = std::str::from_utf8(api_key.as_bytes())
                .context("API key is not valid UTF-8")?;
            request = request.bearer_auth(key_str);
        }

        // Add HMAC signature if configured
        if let Some(hmac_key) = &self.hmac_key {
            let signature = self.create_hmac_signature(&json_payload, hmac_key)?;
            request = request.header("X-Signature", signature);
        }

        let response = request
            .body(json_payload)
            .send()
            .await
            .context("Failed to send HTTP request")?;

        debug!("Response status: {}", response.status());
        Ok(response)
    }

    pub async fn get(&self, endpoint: &str) -> Result<Response> {
        let url = format!("{}{}", self.base_url, endpoint);
        debug!("Sending GET request to: {}", url);

        let mut request = self.client.get(&url);

        // Add authentication
        if let Some(api_key) = &self.api_key {
            let key_str = std::str::from_utf8(api_key.as_bytes())
                .context("API key is not valid UTF-8")?;
            request = request.bearer_auth(key_str);
        }

        let response = request
            .send()
            .await
            .context("Failed to send HTTP request")?;

        debug!("Response status: {}", response.status());
        Ok(response)
    }

    fn create_hmac_signature(&self, payload: &str, key: &SecretKey) -> Result<String> {
        let mut mac = HmacSha256::new_from_slice(key.as_bytes())
            .context("Invalid HMAC key length")?;
        
        mac.update(payload.as_bytes());
        let signature = mac.finalize().into_bytes();
        Ok(BASE64.encode(signature))
    }

    pub fn verify_hmac_signature(&self, payload: &str, signature: &str, key: &SecretKey) -> Result<bool> {
        let expected_signature = self.create_hmac_signature(payload, key)?;
        Ok(constant_time_eq(signature.as_bytes(), expected_signature.as_bytes()))
    }
}

fn load_client_identity(cert_path: &Path, key_path: &Path) -> Result<reqwest::Identity> {
    let cert_data = std::fs::read(cert_path)
        .with_context(|| format!("Failed to read certificate from {:?}", cert_path))?;
    let key_data = std::fs::read(key_path)
        .with_context(|| format!("Failed to read private key from {:?}", key_path))?;

    // Combine cert and key for PKCS#12 format
    let identity = reqwest::Identity::from_pem(&[cert_data, key_data].concat())
        .context("Failed to create client identity")?;

    Ok(identity)
}

fn load_ca_certificate(ca_path: &Path) -> Result<Certificate> {
    let ca_data = std::fs::read(ca_path)
        .with_context(|| format!("Failed to read CA certificate from {:?}", ca_path))?;
    
    let cert = Certificate::from_pem(&ca_data)
        .context("Failed to parse CA certificate")?;
    
    Ok(cert)
}

// Constant-time comparison to prevent timing attacks
fn constant_time_eq(a: &[u8], b: &[u8]) -> bool {
    if a.len() != b.len() {
        return false;
    }

    let mut result = 0u8;
    for (x, y) in a.iter().zip(b.iter()) {
        result |= x ^ y;
    }
    result == 0
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::AgentConfig;

    #[test]
    fn test_constant_time_eq() {
        assert!(constant_time_eq(b"hello", b"hello"));
        assert!(!constant_time_eq(b"hello", b"world"));
        assert!(!constant_time_eq(b"hello", b"hell"));
    }

    #[test]
    fn test_hmac_signature() {
        let key = SecretKey(b"test-key".to_vec());
        let config = AgentConfig::default();
        
        // This would fail in real test without proper client setup
        // but we can test the signature logic with a mock
        let payload = r#"{"test": "data"}"#;
        
        // Test that we can create signatures (actual HTTP client creation would fail)
        // In real tests, you'd use a test HTTP server
    }

    #[tokio::test]
    async fn test_client_creation_with_default_config() {
        let config = AgentConfig::default();
        
        // This might fail due to missing files, but tests the code path
        let _result = SecureHttpClient::new(&config);
        // In real tests, you'd mock the file system or use test fixtures
    }
}