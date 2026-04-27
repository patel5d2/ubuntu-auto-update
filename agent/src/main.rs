mod config;
mod http_client;
mod metrics;
mod updater;
mod enrollment;
mod logging;

use anyhow::{Context, Result};
use clap::{Parser, Subcommand};
use serde::{Deserialize, Serialize};
use std::path::PathBuf;
use std::process;
use std::time::{Duration, Instant};
use tokio::signal;
use tracing::{error, info, warn, debug};

use crate::config::AgentConfig;
use crate::http_client::SecureHttpClient;
use crate::metrics::MetricsCollector;
use crate::updater::{UpdateManager, UpdateResults as UpdaterUpdateResults};
use crate::enrollment::EnrollmentManager;
use crate::logging::setup_logging;

#[derive(Parser)]
#[command(author, version, about, long_about = None)]
struct Cli {
    /// Configuration file path
    #[arg(short, long)]
    config: Option<PathBuf>,
    
    /// Override backend URL
    #[arg(long)]
    backend_url: Option<String>,
    
    /// Enable verbose logging
    #[arg(short, long, action = clap::ArgAction::Count)]
    verbose: u8,
    
    /// Enable dry-run mode (no actual updates)
    #[arg(long)]
    dry_run: bool,

    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Run system updates and report to backend
    Run {
        /// Force run even during maintenance window
        #[arg(long)]
        force: bool,
    },
    /// Enroll this agent with the backend
    Enroll {
        /// Enrollment token from backend
        token: String,
        /// Custom hostname (defaults to system hostname)
        #[arg(long)]
        hostname: Option<String>,
    },
    /// Generate default configuration file
    GenerateConfig {
        /// Output path for configuration file
        #[arg(short, long, default_value = "/etc/ubuntu-auto-update/agent.toml")]
        output: PathBuf,
    },
    /// Show agent status and metrics
    Status,
    /// Export Prometheus metrics
    Metrics,
    /// Test connectivity to backend
    Test,
}

#[derive(Debug, Serialize, Deserialize)]
struct HostReport {
    pub hostname: String,
    pub agent_version: String,
    pub timestamp: chrono::DateTime<chrono::Utc>,
    pub update_results: UpdateResults,
    pub system_info: SystemInfo,
    pub metrics: serde_json::Value,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct UpdateResults {
    pub success: bool,
    pub duration_seconds: f64,
    pub packages_updated: u64,
    pub packages_available: u64,
    pub bytes_downloaded: u64,
    pub reboot_required: bool,
    pub error_message: Option<String>,
    pub apt_output: String,
    pub snap_output: Option<String>,
    pub flatpak_output: Option<String>,
}

#[derive(Debug, Serialize, Deserialize)]
struct SystemInfo {
    pub os_version: String,
    pub kernel_version: String,
    pub architecture: String,
    pub uptime_seconds: u64,
    pub load_average: Vec<f64>,
    pub memory_total_bytes: u64,
    pub memory_available_bytes: u64,
    pub disk_usage_percent: f64,
}

#[tokio::main]
async fn main() -> Result<()> {
    let args = Cli::parse();
    
    // Load configuration
    let mut config = if let Some(config_path) = &args.config {
        AgentConfig::load_from_file(config_path)
            .with_context(|| format!("Failed to load config from {:?}", config_path))?
    } else {
        AgentConfig::load()
            .unwrap_or_else(|e| {
                eprintln!("Warning: Failed to load config, using defaults: {}", e);
                AgentConfig::default()
            })
    };
    
    // Apply CLI overrides
    if let Some(backend_url) = &args.backend_url {
        config.backend.url = backend_url.clone();
    }
    if args.dry_run {
        config.updates.dry_run = true;
    }
    
    // Override log level based on verbosity
    match args.verbose {
        0 => {}, // Use config default
        1 => config.logging.level = "debug".to_string(),
        2 => config.logging.level = "trace".to_string(),
        _ => config.logging.level = "trace".to_string(),
    }
    
    // Validate configuration
    config.validate()
        .with_context(|| "Configuration validation failed")?;
    
    // Setup logging
    setup_logging(&config.logging)
        .with_context(|| "Failed to setup logging")?;
    
    info!("Starting Ubuntu Auto-Update Agent v{}", env!("CARGO_PKG_VERSION"));
    debug!("Configuration loaded: backend={}", config.backend.url);
    
    match args.command {
        Commands::GenerateConfig { output } => {
            generate_default_config(&output).await
        }
        Commands::Run { force } => {
            run_updates(&config, force).await
        }
        Commands::Enroll { token, hostname } => {
            enroll_agent(&config, &token, hostname).await
        }
        Commands::Status => {
            show_status(&config).await
        }
        Commands::Metrics => {
            export_metrics(&config).await
        }
        Commands::Test => {
            test_connectivity(&config).await
        }
    }
}

async fn generate_default_config(output_path: &PathBuf) -> Result<()> {
    info!("Generating default configuration at {:?}", output_path);
    
    if let Some(parent) = output_path.parent() {
        std::fs::create_dir_all(parent)
            .with_context(|| format!("Failed to create config directory: {:?}", parent))?;
    }
    
    AgentConfig::save_default_config(output_path.to_str().unwrap())
        .with_context(|| "Failed to save default configuration")?;
    
    info!("Default configuration generated successfully");
    Ok(())
}

async fn run_updates(config: &AgentConfig, force: bool) -> Result<()> {
    info!("Starting update run (dry_run={})", config.updates.dry_run);
    let start_time = Instant::now();
    
    // Initialize metrics collector
    let metrics_collector = if config.metrics.enabled {
        Some(MetricsCollector::new(config.metrics.clone())
            .with_context(|| "Failed to initialize metrics collector")?)
    } else {
        None
    };
    
    if let Some(metrics) = &metrics_collector {
        metrics.record_update_start();
    }
    
    // Initialize HTTP client
    let http_client = SecureHttpClient::new(config)
        .with_context(|| "Failed to initialize HTTP client")?;
    
    // Initialize update manager
    let mut update_manager = UpdateManager::new(config.clone())
        .with_context(|| "Failed to initialize update manager")?;
    
    // Check maintenance window
    if !force && !update_manager.is_in_maintenance_window() {
        warn!("Outside maintenance window, skipping update (use --force to override)");
        return Ok(());
    }
    
    // Run updates
    let update_result = update_manager.run_updates().await;
    let duration = start_time.elapsed();
    
    // Collect system metrics if enabled
    let system_metrics = if let Some(metrics) = &metrics_collector {
        metrics.collect_system_metrics().await.ok()
    } else {
        None
    };
    
    // Record metrics
    if let Some(metrics) = &metrics_collector {
        match &update_result {
            Ok(results) => {
                metrics.record_update_completion(
                    duration.as_secs_f64(),
                    0, // success exit code
                    results.packages_updated,
                    results.bytes_downloaded as f64,
                );
                metrics.set_packages_available(results.packages_available);
                metrics.set_reboot_required(results.reboot_required);
            }
            Err(_) => {
                metrics.record_update_completion(
                    duration.as_secs_f64(),
                    1, // error exit code
                    0,
                    0.0,
                );
            }
        }
        
        // Write textfile metrics
        if let Err(e) = metrics.write_textfile_metrics().await {
            warn!("Failed to write textfile metrics: {}", e);
        }
    }
    
    // Send report to backend
    match &update_result {
        Ok(results) => {
            let converted_results = convert_updater_results(results);
            let report = create_host_report(config, &converted_results, system_metrics.as_ref(), duration)?;
            send_report_to_backend(&http_client, &report).await
                .with_context(|| "Failed to send report to backend")?;
            
            info!("Update completed successfully in {:.2}s", duration.as_secs_f64());
            
            // Handle reboot if required and enabled
            if results.reboot_required && config.updates.auto_reboot {
                info!("Reboot required, scheduling reboot in {} minutes", config.updates.reboot_delay_minutes);
                schedule_reboot(config.updates.reboot_delay_minutes).await?;
            }
            
            Ok(())
        }
        Err(e) => {
            error!("Update failed: {}", e);
            
            // Still try to send error report
            let error_results = UpdateResults {
                success: false,
                duration_seconds: duration.as_secs_f64(),
                packages_updated: 0,
                packages_available: 0,
                bytes_downloaded: 0,
                reboot_required: false,
                error_message: Some(e.to_string()),
                apt_output: String::new(),
                snap_output: None,
                flatpak_output: None,
            };
            
            let report = create_host_report(config, &error_results, system_metrics.as_ref(), duration)?;
            let _ = send_report_to_backend(&http_client, &report).await;
            
            Err(anyhow::anyhow!("Update failed: {}", e))
        }
    }
}

async fn enroll_agent(config: &AgentConfig, token: &str, hostname: Option<String>) -> Result<()> {
    info!("Starting agent enrollment");
    
    let enrollment_manager = EnrollmentManager::new(config)
        .with_context(|| "Failed to initialize enrollment manager")?;
    
    enrollment_manager.enroll(token, hostname.as_deref()).await
        .with_context(|| "Enrollment failed")?;
    
    info!("Agent enrolled successfully");
    Ok(())
}

async fn show_status(config: &AgentConfig) -> Result<()> {
    println!("Ubuntu Auto-Update Agent Status");
    println!("================================");
    println!("Version: {}", env!("CARGO_PKG_VERSION"));
    println!("Backend URL: {}", config.backend.url);
    
    // Check if enrolled
    if config.security.api_key_file.exists() {
        println!("Status: Enrolled");
    } else {
        println!("Status: Not enrolled");
    }
    
    // Show last metrics if available
    if config.metrics.enabled {
        if let Ok(metrics_collector) = MetricsCollector::new(config.metrics.clone()) {
            let update_metrics = metrics_collector.get_update_metrics();
            println!("\nLast Update:");
            if update_metrics.last_run_timestamp > 0 {
                let last_run = chrono::DateTime::from_timestamp(update_metrics.last_run_timestamp as i64, 0);
                println!("  Time: {:?}", last_run);
                println!("  Duration: {:.2}s", update_metrics.last_run_duration_seconds);
                println!("  Exit Code: {}", update_metrics.last_run_exit_code);
                println!("  Packages Updated: {}", update_metrics.packages_updated);
                println!("  Packages Available: {}", update_metrics.packages_available);
                println!("  Reboot Required: {}", update_metrics.reboot_required);
            } else {
                println!("  No previous runs recorded");
            }
        }
    }
    
    Ok(())
}

async fn export_metrics(config: &AgentConfig) -> Result<()> {
    if !config.metrics.enabled {
        println!("Metrics collection is disabled");
        return Ok(());
    }
    
    let metrics_collector = MetricsCollector::new(config.metrics.clone())
        .with_context(|| "Failed to initialize metrics collector")?;
    
    let prometheus_output = metrics_collector.export_prometheus_metrics()
        .with_context(|| "Failed to export metrics")?;
    
    println!("{}", prometheus_output);
    Ok(())
}

async fn test_connectivity(config: &AgentConfig) -> Result<()> {
    info!("Testing connectivity to backend: {}", config.backend.url);
    
    let http_client = SecureHttpClient::new(config)
        .with_context(|| "Failed to initialize HTTP client")?;
    
    let start = Instant::now();
    match http_client.get("/api/v1/health").await {
        Ok(response) => {
            let duration = start.elapsed();
            println!("✓ Backend reachable");
            println!("  Status: {}", response.status());
            println!("  Response time: {:.2}ms", duration.as_millis());
            
            if response.status().is_success() {
                println!("✓ Backend is healthy");
            } else {
                println!("⚠ Backend returned non-success status");
            }
        }
        Err(e) => {
            println!("✗ Failed to reach backend: {}", e);
            return Err(e);
        }
    }
    
    Ok(())
}

fn create_host_report(
    config: &AgentConfig,
    update_results: &UpdateResults,
    system_metrics: Option<&crate::metrics::SystemMetrics>,
    duration: Duration,
) -> Result<HostReport> {
    let hostname = gethostname::gethostname().into_string()
        .map_err(|_| anyhow::anyhow!("Failed to get hostname"))?;
    
    let system_info = SystemInfo {
        os_version: get_os_version()?,
        kernel_version: get_kernel_version()?,
        architecture: std::env::consts::ARCH.to_string(),
        uptime_seconds: system_metrics.map(|m| m.uptime_seconds).unwrap_or(0),
        load_average: system_metrics.map(|m| vec![m.load_average_1m, m.load_average_5m, m.load_average_15m]).unwrap_or_default(),
        memory_total_bytes: system_metrics.map(|m| m.memory_total_bytes).unwrap_or(0),
        memory_available_bytes: system_metrics.map(|m| m.memory_total_bytes - m.memory_usage_bytes).unwrap_or(0),
        disk_usage_percent: system_metrics.map(|m| {
            if m.disk_total_bytes > 0 {
                (m.disk_usage_bytes as f64 / m.disk_total_bytes as f64) * 100.0
            } else {
                0.0
            }
        }).unwrap_or(0.0),
    };
    
    let metrics_json = if let Some(metrics) = system_metrics {
        serde_json::to_value(metrics)
            .with_context(|| "Failed to serialize metrics")?  
    } else {
        serde_json::Value::Null
    };
    
    Ok(HostReport {
        hostname,
        agent_version: env!("CARGO_PKG_VERSION").to_string(),
        timestamp: chrono::Utc::now(),
        update_results: update_results.clone(),
        system_info,
        metrics: metrics_json,
    })
}

async fn send_report_to_backend(client: &SecureHttpClient, report: &HostReport) -> Result<()> {
    debug!("Sending report to backend for host: {}", report.hostname);
    
    let response = client
        .post_with_retry(
            "/api/v1/report",
            report,
            3, // max retries
            Duration::from_secs(5), // retry delay
        )
        .await
        .with_context(|| "Failed to send report to backend")?;
    
    if response.status().is_success() {
        info!("Report sent successfully to backend");
    } else {
        let status = response.status();
        let body = response.text().await.unwrap_or_default();
        return Err(anyhow::anyhow!(
            "Backend returned error: {} - {}",
            status,
            body
        ));
    }
    
    Ok(())
}

async fn schedule_reboot(delay_minutes: u32) -> Result<()> {
    info!("Scheduling system reboot in {} minutes", delay_minutes);
    
    let delay_seconds = delay_minutes * 60;
    let output = std::process::Command::new("shutdown")
        .args(["-r", &format!("+{}", delay_minutes), "Scheduled reboot after system updates"])
        .output()
        .with_context(|| "Failed to schedule reboot")?;
    
    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        return Err(anyhow::anyhow!("Failed to schedule reboot: {}", stderr));
    }
    
    info!("Reboot scheduled successfully");
    Ok(())
}

fn get_os_version() -> Result<String> {
    let output = std::process::Command::new("lsb_release")
        .args(["-ds"])
        .output()
        .with_context(|| "Failed to get OS version")?;
    
    if output.status.success() {
        Ok(String::from_utf8_lossy(&output.stdout).trim().to_string())
    } else {
        Ok("Unknown".to_string())
    }
}

fn convert_updater_results(updater_results: &UpdaterUpdateResults) -> UpdateResults {
    UpdateResults {
        success: updater_results.success,
        duration_seconds: updater_results.duration_seconds,
        packages_updated: updater_results.packages_updated,
        packages_available: updater_results.packages_available,
        bytes_downloaded: updater_results.bytes_downloaded,
        reboot_required: updater_results.reboot_required,
        error_message: updater_results.error_message.clone(),
        apt_output: updater_results.apt_output.clone(),
        snap_output: updater_results.snap_output.clone(),
        flatpak_output: updater_results.flatpak_output.clone(),
    }
}

fn get_kernel_version() -> Result<String> {
    let output = std::process::Command::new("uname")
        .arg("-r")
        .output()
        .with_context(|| "Failed to get kernel version")?;
    
    if output.status.success() {
        Ok(String::from_utf8_lossy(&output.stdout).trim().to_string())
    } else {
        Ok("Unknown".to_string())
    }
}
