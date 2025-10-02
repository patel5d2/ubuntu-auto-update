use anyhow::{Context, Result};
use chrono::{DateTime, Local, NaiveTime, TimeZone};
use regex::Regex;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::path::Path;
use std::process::{Command, Output, Stdio};
use std::time::Duration;
use tokio::time::timeout;
use tracing::{debug, error, info, warn};

use crate::config::AgentConfig;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UpdateResults {
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

pub struct UpdateManager {
    config: AgentConfig,
    dry_run: bool,
}

impl UpdateManager {
    pub fn new(config: AgentConfig) -> Result<Self> {
        Ok(Self {
            dry_run: config.updates.dry_run,
            config,
        })
    }

    pub fn is_in_maintenance_window(&self) -> bool {
        let (start, end) = match (&self.config.updates.maintenance_window_start, &self.config.updates.maintenance_window_end) {
            (Some(start_str), Some(end_str)) => {
                let start_time = match NaiveTime::parse_from_str(start_str, "%H:%M") {
                    Ok(time) => time,
                    Err(e) => {
                        warn!("Failed to parse maintenance window start: {} - {}", start_str, e);
                        return true; // Default to allowing updates
                    }
                };
                
                let end_time = match NaiveTime::parse_from_str(end_str, "%H:%M") {
                    Ok(time) => time,
                    Err(e) => {
                        warn!("Failed to parse maintenance window end: {} - {}", end_str, e);
                        return true; // Default to allowing updates
                    }
                };
                
                (start_time, end_time)
            }
            _ => return true, // No maintenance window configured
        };

        let now = Local::now().time();
        
        // Handle maintenance windows that cross midnight
        if start <= end {
            now >= start && now <= end
        } else {
            now >= start || now <= end
        }
    }

    pub async fn run_updates(&mut self) -> Result<UpdateResults> {
        info!("Starting system update process (dry_run: {})", self.dry_run);
        let start_time = std::time::Instant::now();

        let mut results = UpdateResults {
            success: false,
            duration_seconds: 0.0,
            packages_updated: 0,
            packages_available: 0,
            bytes_downloaded: 0,
            reboot_required: false,
            error_message: None,
            apt_output: String::new(),
            snap_output: None,
            flatpak_output: None,
        };

        // Check if we're root (required for most operations)
        if !self.is_running_as_root() && !self.dry_run {
            return Err(anyhow::anyhow!("Must run as root to perform system updates"));
        }

        // Run apt updates
        if self.config.updates.update_sources.apt {
            match self.run_apt_updates().await {
                Ok(apt_results) => {
                    results.apt_output = apt_results.output;
                    results.packages_updated += apt_results.packages_updated;
                    results.packages_available += apt_results.packages_available;
                    results.bytes_downloaded += apt_results.bytes_downloaded;
                }
                Err(e) => {
                    error!("APT updates failed: {}", e);
                    results.error_message = Some(format!("APT: {}", e));
                    results.duration_seconds = start_time.elapsed().as_secs_f64();
                    return Ok(results);
                }
            }
        }

        // Run snap updates
        if self.config.updates.update_sources.snap {
            match self.run_snap_updates().await {
                Ok(snap_output) => {
                    results.snap_output = Some(snap_output);
                }
                Err(e) => {
                    warn!("Snap updates failed: {}", e);
                    // Don't fail the entire update for snap failures
                }
            }
        }

        // Run flatpak updates
        if self.config.updates.update_sources.flatpak {
            match self.run_flatpak_updates().await {
                Ok(flatpak_output) => {
                    results.flatpak_output = Some(flatpak_output);
                }
                Err(e) => {
                    warn!("Flatpak updates failed: {}", e);
                    // Don't fail the entire update for flatpak failures
                }
            }
        }

        // Check if reboot is required
        results.reboot_required = self.check_reboot_required()?;

        results.success = true;
        results.duration_seconds = start_time.elapsed().as_secs_f64();
        
        info!(
            "Update process completed successfully in {:.2}s: {} packages updated, {} available, reboot required: {}",
            results.duration_seconds,
            results.packages_updated,
            results.packages_available,
            results.reboot_required
        );

        Ok(results)
    }

    async fn run_apt_updates(&self) -> Result<AptResults> {
        info!("Running APT updates");

        // First, update package lists
        let update_output = self.run_command_with_timeout(
            "apt-get",
            &["update"],
            Duration::from_secs(300), // 5 minutes
        ).await?;

        if !update_output.status.success() {
            return Err(anyhow::anyhow!(
                "apt-get update failed: {}",
                String::from_utf8_lossy(&update_output.stderr)
            ));
        }

        // Get list of available updates
        let list_output = self.run_command_with_timeout(
            "apt",
            &["list", "--upgradable"],
            Duration::from_secs(60),
        ).await?;

        let packages_available = if list_output.status.success() {
            self.parse_apt_upgradable_count(&String::from_utf8_lossy(&list_output.stdout))?
        } else {
            0
        };

        let mut apt_output = format!("=== APT Update Output ===\n{}", 
            String::from_utf8_lossy(&update_output.stdout));

        let (packages_updated, bytes_downloaded) = if self.dry_run {
            // Dry run - just show what would be updated
            let dry_run_output = self.run_command_with_timeout(
                "apt-get",
                &["--dry-run", "upgrade"],
                Duration::from_secs(300),
            ).await?;

            apt_output.push_str(&format!("\n=== Dry Run Upgrade Output ===\n{}", 
                String::from_utf8_lossy(&dry_run_output.stdout)));

            (0, 0) // No actual updates in dry run
        } else {
            // Apply excluded packages filter
            let mut upgrade_args = vec!["upgrade", "-y"];
            for excluded in &self.config.updates.excluded_packages {
                upgrade_args.extend_from_slice(&["--hold", excluded]);
            }

            // Run the actual upgrade
            let upgrade_output = self.run_command_with_timeout(
                "apt-get",
                &upgrade_args,
                Duration::from_secs(1800), // 30 minutes
            ).await?;

            apt_output.push_str(&format!("\n=== Upgrade Output ===\n{}", 
                String::from_utf8_lossy(&upgrade_output.stdout)));

            if !upgrade_output.status.success() {
                return Err(anyhow::anyhow!(
                    "apt-get upgrade failed: {}",
                    String::from_utf8_lossy(&upgrade_output.stderr)
                ));
            }

            let packages_updated = self.parse_apt_packages_updated(&String::from_utf8_lossy(&upgrade_output.stdout))?;
            let bytes_downloaded = self.parse_apt_bytes_downloaded(&String::from_utf8_lossy(&upgrade_output.stdout))?;

            // Clean up
            let _ = self.run_command_with_timeout(
                "apt-get",
                &["autoremove", "-y"],
                Duration::from_secs(300),
            ).await;

            let _ = self.run_command_with_timeout(
                "apt-get",
                &["autoclean"],
                Duration::from_secs(60),
            ).await;

            (packages_updated, bytes_downloaded)
        };

        Ok(AptResults {
            output: apt_output,
            packages_updated,
            packages_available,
            bytes_downloaded,
        })
    }

    async fn run_snap_updates(&self) -> Result<String> {
        info!("Running snap updates");

        if !Path::new("/usr/bin/snap").exists() {
            return Ok("Snap not installed".to_string());
        }

        let output = if self.dry_run {
            self.run_command_with_timeout(
                "snap",
                &["refresh", "--list"],
                Duration::from_secs(60),
            ).await?
        } else {
            self.run_command_with_timeout(
                "snap",
                &["refresh"],
                Duration::from_secs(900), // 15 minutes
            ).await?
        };

        Ok(String::from_utf8_lossy(&output.stdout).to_string())
    }

    async fn run_flatpak_updates(&self) -> Result<String> {
        info!("Running flatpak updates");

        if !Path::new("/usr/bin/flatpak").exists() {
            return Ok("Flatpak not installed".to_string());
        }

        let output = if self.dry_run {
            self.run_command_with_timeout(
                "flatpak",
                &["update", "--show-details"],
                Duration::from_secs(60),
            ).await?
        } else {
            self.run_command_with_timeout(
                "flatpak",
                &["update", "-y"],
                Duration::from_secs(900), // 15 minutes
            ).await?
        };

        Ok(String::from_utf8_lossy(&output.stdout).to_string())
    }

    async fn run_command_with_timeout(&self, command: &str, args: &[&str], timeout_duration: Duration) -> Result<Output> {
        debug!("Running command: {} {}", command, args.join(" "));

        let child = Command::new(command)
            .args(args)
            .stdin(Stdio::null())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .with_context(|| format!("Failed to spawn command: {}", command))?;

        let output = timeout(timeout_duration, async {
            tokio::task::spawn_blocking(move || {
                child.wait_with_output()
            }).await.unwrap()
        }).await
        .with_context(|| format!("Command timed out after {:?}: {}", timeout_duration, command))?
        .with_context(|| format!("Command failed: {}", command))?;

        debug!("Command completed with exit code: {:?}", output.status.code());
        Ok(output)
    }

    fn check_reboot_required(&self) -> Result<bool> {
        // Check /var/run/reboot-required file
        if Path::new("/var/run/reboot-required").exists() {
            return Ok(true);
        }

        // Check if kernel has been updated
        let output = Command::new("uname")
            .arg("-r")
            .output()
            .with_context(|| "Failed to get kernel version")?;

        if output.status.success() {
            let running_kernel = String::from_utf8_lossy(&output.stdout).trim().to_string();
            
            // Check if there's a newer kernel installed
            let dpkg_output = Command::new("dpkg")
                .args(&["-l", "linux-image-*"])
                .output();

            if let Ok(dpkg_output) = dpkg_output {
                if dpkg_output.status.success() {
                    let dpkg_list = String::from_utf8_lossy(&dpkg_output.stdout);
                    // This is a simplified check - in reality you'd want more sophisticated kernel version comparison
                    if !dpkg_list.contains(&running_kernel) {
                        return Ok(true);
                    }
                }
            }
        }

        Ok(false)
    }

    fn parse_apt_upgradable_count(&self, output: &str) -> Result<u64> {
        let lines: Vec<&str> = output.lines().collect();
        // First line is usually "Listing..." so count actual package lines
        let count = lines.iter()
            .skip(1) // Skip header
            .filter(|line| line.contains("/") && line.contains("upgradable"))
            .count();
        
        Ok(count as u64)
    }

    fn parse_apt_packages_updated(&self, output: &str) -> Result<u64> {
        // Look for patterns like "X upgraded, Y newly installed"
        let re = Regex::new(r"(\d+)\s+upgraded")?;
        
        if let Some(captures) = re.captures(output) {
            if let Some(count_str) = captures.get(1) {
                return Ok(count_str.as_str().parse::<u64>()?);
            }
        }
        
        Ok(0)
    }

    fn parse_apt_bytes_downloaded(&self, output: &str) -> Result<u64> {
        // Look for patterns like "Need to get 42.1 MB of archives"
        let re = Regex::new(r"Need to get ([0-9.,]+)\s*([kMG]?B)")?;
        
        if let Some(captures) = re.captures(output) {
            if let (Some(size_str), Some(unit_str)) = (captures.get(1), captures.get(2)) {
                let size: f64 = size_str.as_str().replace(",", "").parse()?;
                let multiplier = match unit_str.as_str() {
                    "kB" => 1_000,
                    "MB" => 1_000_000,
                    "GB" => 1_000_000_000,
                    _ => 1,
                };
                
                return Ok((size * multiplier as f64) as u64);
            }
        }
        
        Ok(0)
    }

    fn is_running_as_root(&self) -> bool {
        // Check if running as root
        std::process::id() == 0 || 
        std::env::var("USER").unwrap_or_default() == "root" ||
        std::env::var("EUID").unwrap_or_default() == "0"
    }
}

#[derive(Debug)]
struct AptResults {
    output: String,
    packages_updated: u64,
    packages_available: u64,
    bytes_downloaded: u64,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::*;

    #[test]
    fn test_parse_apt_upgradable_count() {
        let manager = UpdateManager::new(AgentConfig::default()).unwrap();
        
        let output = r#"Listing...
firefox/jammy-updates,jammy-security 108.0.1+build1-0ubuntu0.22.04.1 amd64 [upgradable from: 108.0+build2-0ubuntu0.22.04.1]
thunderbird/jammy-updates,jammy-security 1:102.6.0+build1-0ubuntu0.22.04.1 amd64 [upgradable from: 1:102.5.1+build2-0ubuntu0.22.04.1]
"#;
        
        let count = manager.parse_apt_upgradable_count(output).unwrap();
        assert_eq!(count, 2);
    }

    #[test]
    fn test_parse_apt_packages_updated() {
        let manager = UpdateManager::new(AgentConfig::default()).unwrap();
        
        let output = r#"
Reading package lists...
Building dependency tree...
The following packages will be upgraded:
  firefox thunderbird
2 upgraded, 0 newly installed, 0 to remove and 0 not upgraded.
"#;
        
        let count = manager.parse_apt_packages_updated(output).unwrap();
        assert_eq!(count, 2);
    }

    #[test]
    fn test_parse_apt_bytes_downloaded() {
        let manager = UpdateManager::new(AgentConfig::default()).unwrap();
        
        let output = r#"
The following packages will be upgraded:
  firefox thunderbird
2 upgraded, 0 newly installed, 0 to remove and 0 not upgraded.
Need to get 42.1 MB of archives.
After this operation, 512 kB of additional disk space will be used.
"#;
        
        let bytes = manager.parse_apt_bytes_downloaded(output).unwrap();
        assert_eq!(bytes, 42_100_000);
    }

    #[test]
    fn test_maintenance_window_check() {
        let mut config = AgentConfig::default();
        config.updates.maintenance_window_start = Some("02:00".to_string());
        config.updates.maintenance_window_end = Some("04:00".to_string());
        
        let manager = UpdateManager::new(config).unwrap();
        
        // This test would need to be run at different times or mock the time
        // For now, just ensure it doesn't panic
        let _in_window = manager.is_in_maintenance_window();
    }
}