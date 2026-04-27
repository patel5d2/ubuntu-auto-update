use anyhow::{Context, Result};
use prometheus::{
    Counter, Gauge, Histogram, IntCounter, IntGauge, Opts, Registry, TextEncoder, Encoder,
};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs::OpenOptions;
use std::io::Write;
use std::path::Path;
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};
use sysinfo::{System, SystemExt, DiskExt, ComponentExt, CpuExt};
use tokio::sync::RwLock;
use tracing::{debug, error, info, warn};

use crate::config::MetricsConfig;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SystemMetrics {
    pub cpu_usage_percent: f64,
    pub memory_usage_bytes: u64,
    pub memory_total_bytes: u64,
    pub disk_usage_bytes: u64,
    pub disk_total_bytes: u64,
    pub load_average_1m: f64,
    pub load_average_5m: f64,
    pub load_average_15m: f64,
    pub uptime_seconds: u64,
    pub temperature_celsius: Option<f64>,
    pub network_rx_bytes: u64,
    pub network_tx_bytes: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UpdateMetrics {
    pub last_run_timestamp: u64,
    pub last_run_duration_seconds: f64,
    pub last_run_exit_code: i32,
    pub packages_updated: u64,
    pub packages_available: u64,
    pub reboot_required: bool,
    pub update_success_total: u64,
    pub update_error_total: u64,
    pub bytes_downloaded: u64,
}

pub struct MetricsCollector {
    registry: Registry,
    config: MetricsConfig,
    
    // Update metrics
    last_run_timestamp: IntGauge,
    last_run_duration: Gauge,
    last_run_exit_code: IntGauge,
    packages_updated: IntGauge,
    packages_available: IntGauge,
    reboot_required: IntGauge,
    update_success_counter: IntCounter,
    update_error_counter: IntCounter,
    bytes_downloaded_counter: Counter,
    
    // System metrics
    cpu_usage: Gauge,
    memory_usage: IntGauge,
    memory_total: IntGauge,
    disk_usage: IntGauge,
    disk_total: IntGauge,
    load_average_1m: Gauge,
    load_average_5m: Gauge,
    load_average_15m: Gauge,
    uptime: IntGauge,
    temperature: Gauge,
    
    // Runtime data
    system: Arc<RwLock<System>>,
}

impl MetricsCollector {
    pub fn new(config: MetricsConfig) -> Result<Self> {
        let registry = Registry::new();
        
        // Create update metrics
        let last_run_timestamp = IntGauge::with_opts(Opts::new(
            "ubuntu_auto_update_last_run_timestamp_seconds",
            "Timestamp of the last update run"
        ))?;
        
        let last_run_duration = Gauge::with_opts(Opts::new(
            "ubuntu_auto_update_last_run_duration_seconds",
            "Duration of the last update run in seconds"
        ))?;
        
        let last_run_exit_code = IntGauge::with_opts(Opts::new(
            "ubuntu_auto_update_last_run_exit_code",
            "Exit code of the last update run"
        ))?;
        
        let packages_updated = IntGauge::with_opts(Opts::new(
            "ubuntu_auto_update_packages_updated",
            "Number of packages updated in the last run"
        ))?;
        
        let packages_available = IntGauge::with_opts(Opts::new(
            "ubuntu_auto_update_packages_available",
            "Number of packages available for update"
        ))?;
        
        let reboot_required = IntGauge::with_opts(Opts::new(
            "ubuntu_auto_update_reboot_required",
            "Whether a reboot is required (1 = yes, 0 = no)"
        ))?;
        
        let update_success_counter = IntCounter::with_opts(Opts::new(
            "ubuntu_auto_update_success_total",
            "Total number of successful update runs"
        ))?;
        
        let update_error_counter = IntCounter::with_opts(Opts::new(
            "ubuntu_auto_update_error_total",
            "Total number of failed update runs"
        ))?;
        
        let bytes_downloaded_counter = Counter::with_opts(Opts::new(
            "ubuntu_auto_update_bytes_downloaded_total",
            "Total bytes downloaded during updates"
        ))?;

        // Create system metrics
        let cpu_usage = Gauge::with_opts(Opts::new(
            "system_cpu_usage_percent",
            "Current CPU usage percentage"
        ))?;
        
        let memory_usage = IntGauge::with_opts(Opts::new(
            "system_memory_usage_bytes",
            "Current memory usage in bytes"
        ))?;
        
        let memory_total = IntGauge::with_opts(Opts::new(
            "system_memory_total_bytes",
            "Total system memory in bytes"
        ))?;
        
        let disk_usage = IntGauge::with_opts(Opts::new(
            "system_disk_usage_bytes",
            "Current disk usage in bytes"
        ))?;
        
        let disk_total = IntGauge::with_opts(Opts::new(
            "system_disk_total_bytes",
            "Total disk space in bytes"
        ))?;
        
        let load_average_1m = Gauge::with_opts(Opts::new(
            "system_load_average_1m",
            "System load average over 1 minute"
        ))?;
        
        let load_average_5m = Gauge::with_opts(Opts::new(
            "system_load_average_5m",
            "System load average over 5 minutes"
        ))?;
        
        let load_average_15m = Gauge::with_opts(Opts::new(
            "system_load_average_15m",
            "System load average over 15 minutes"
        ))?;
        
        let uptime = IntGauge::with_opts(Opts::new(
            "system_uptime_seconds",
            "System uptime in seconds"
        ))?;
        
        let temperature = Gauge::with_opts(Opts::new(
            "system_temperature_celsius",
            "System temperature in Celsius"
        ))?;

        // Register metrics
        registry.register(Box::new(last_run_timestamp.clone()))?;
        registry.register(Box::new(last_run_duration.clone()))?;
        registry.register(Box::new(last_run_exit_code.clone()))?;
        registry.register(Box::new(packages_updated.clone()))?;
        registry.register(Box::new(packages_available.clone()))?;
        registry.register(Box::new(reboot_required.clone()))?;
        registry.register(Box::new(update_success_counter.clone()))?;
        registry.register(Box::new(update_error_counter.clone()))?;
        registry.register(Box::new(bytes_downloaded_counter.clone()))?;
        
        if config.collect_system_metrics {
            registry.register(Box::new(cpu_usage.clone()))?;
            registry.register(Box::new(memory_usage.clone()))?;
            registry.register(Box::new(memory_total.clone()))?;
            registry.register(Box::new(disk_usage.clone()))?;
            registry.register(Box::new(disk_total.clone()))?;
            registry.register(Box::new(load_average_1m.clone()))?;
            registry.register(Box::new(load_average_5m.clone()))?;
            registry.register(Box::new(load_average_15m.clone()))?;
            registry.register(Box::new(uptime.clone()))?;
            registry.register(Box::new(temperature.clone()))?;
        }

        Ok(Self {
            registry,
            config,
            last_run_timestamp,
            last_run_duration,
            last_run_exit_code,
            packages_updated,
            packages_available,
            reboot_required,
            update_success_counter,
            update_error_counter,
            bytes_downloaded_counter,
            cpu_usage,
            memory_usage,
            memory_total,
            disk_usage,
            disk_total,
            load_average_1m,
            load_average_5m,
            load_average_15m,
            uptime,
            temperature,
            system: Arc::new(RwLock::new(System::new_all())),
        })
    }

    pub fn record_update_start(&self) {
        let timestamp = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs() as i64;
        self.last_run_timestamp.set(timestamp);
        debug!("Recorded update start at timestamp: {}", timestamp);
    }

    pub fn record_update_completion(&self, duration_secs: f64, exit_code: i32, packages_updated: u64, bytes_downloaded: f64) {
        self.last_run_duration.set(duration_secs);
        self.last_run_exit_code.set(exit_code as i64);
        self.packages_updated.set(packages_updated as i64);
        self.bytes_downloaded_counter.inc_by(bytes_downloaded);

        if exit_code == 0 {
            self.update_success_counter.inc();
            info!("Recorded successful update: {}s, {} packages", duration_secs, packages_updated);
        } else {
            self.update_error_counter.inc();
            warn!("Recorded failed update: {}s, exit code {}", duration_secs, exit_code);
        }
    }

    pub fn set_packages_available(&self, count: u64) {
        self.packages_available.set(count as i64);
        debug!("Set packages available: {}", count);
    }

    pub fn set_reboot_required(&self, required: bool) {
        self.reboot_required.set(if required { 1 } else { 0 });
        debug!("Set reboot required: {}", required);
    }

    pub async fn collect_system_metrics(&self) -> Result<SystemMetrics> {
        if !self.config.collect_system_metrics {
            return Err(anyhow::anyhow!("System metrics collection disabled"));
        }

        let mut system = self.system.write().await;
        system.refresh_all();

        let cpu_usage = system.global_cpu_info().cpu_usage() as f64;
        let memory_usage = system.used_memory();
        let memory_total = system.total_memory();
        
        // Get first disk stats (root filesystem)
        let mut disk_usage = 0;
        let mut disk_total = 0;
        if let Some(disk) = system.disks().first() {
            disk_usage = disk.total_space() - disk.available_space();
            disk_total = disk.total_space();
        }

        let load_avg = system.load_average();
        let uptime = system.uptime();
        
        // Get temperature from first component
        let temperature = system.components()
            .first()
            .map(|c| c.temperature() as f64);

        // Update Prometheus metrics
        self.cpu_usage.set(cpu_usage);
        self.memory_usage.set(memory_usage as i64);
        self.memory_total.set(memory_total as i64);
        self.disk_usage.set(disk_usage as i64);
        self.disk_total.set(disk_total as i64);
        self.load_average_1m.set(load_avg.one);
        self.load_average_5m.set(load_avg.five);
        self.load_average_15m.set(load_avg.fifteen);
        self.uptime.set(uptime as i64);
        
        if let Some(temp) = temperature {
            self.temperature.set(temp);
        }

        Ok(SystemMetrics {
            cpu_usage_percent: cpu_usage,
            memory_usage_bytes: memory_usage,
            memory_total_bytes: memory_total,
            disk_usage_bytes: disk_usage,
            disk_total_bytes: disk_total,
            load_average_1m: load_avg.one,
            load_average_5m: load_avg.five,
            load_average_15m: load_avg.fifteen,
            uptime_seconds: uptime,
            temperature_celsius: temperature,
            network_rx_bytes: 0, // TODO: Implement network stats
            network_tx_bytes: 0, // TODO: Implement network stats
        })
    }

    pub fn export_prometheus_metrics(&self) -> Result<String> {
        let encoder = TextEncoder::new();
        let metric_families = self.registry.gather();
        
        let mut buffer = Vec::new();
        encoder.encode(&metric_families, &mut buffer)?;
        
        Ok(String::from_utf8(buffer)?)
    }

    pub async fn write_textfile_metrics(&self) -> Result<()> {
        if let Some(path) = &self.config.textfile_path {
            let metrics = self.export_prometheus_metrics()?;
            let textfile_path = path.join("ubuntu-auto-update.prom");
            
            let mut file = OpenOptions::new()
                .create(true)
                .write(true)
                .truncate(true)
                .open(&textfile_path)
                .with_context(|| format!("Failed to open textfile: {:?}", textfile_path))?;
            
            file.write_all(metrics.as_bytes())
                .with_context(|| format!("Failed to write textfile: {:?}", textfile_path))?;
            
            debug!("Wrote metrics to textfile: {:?}", textfile_path);
        }
        
        Ok(())
    }

    pub fn get_update_metrics(&self) -> UpdateMetrics {
        UpdateMetrics {
            last_run_timestamp: self.last_run_timestamp.get() as u64,
            last_run_duration_seconds: self.last_run_duration.get(),
            last_run_exit_code: self.last_run_exit_code.get() as i32,
            packages_updated: self.packages_updated.get() as u64,
            packages_available: self.packages_available.get() as u64,
            reboot_required: self.reboot_required.get() == 1,
            update_success_total: self.update_success_counter.get(),
            update_error_total: self.update_error_counter.get(),
            bytes_downloaded: self.bytes_downloaded_counter.get() as u64,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_metrics_collector_creation() {
        let config = MetricsConfig {
            enabled: true,
            port: Some(9100),
            textfile_path: None,
            collect_system_metrics: true,
        };
        
        let collector = MetricsCollector::new(config).unwrap();
        let metrics = collector.export_prometheus_metrics().unwrap();
        
        assert!(metrics.contains("ubuntu_auto_update"));
    }

    #[test]
    fn test_update_metrics_recording() {
        let config = MetricsConfig {
            enabled: true,
            port: Some(9100),
            textfile_path: None,
            collect_system_metrics: false,
        };
        
        let collector = MetricsCollector::new(config).unwrap();
        
        collector.record_update_start();
        collector.record_update_completion(30.5, 0, 5, 1024.0);
        collector.set_packages_available(10);
        collector.set_reboot_required(true);
        
        let update_metrics = collector.get_update_metrics();
        assert_eq!(update_metrics.last_run_exit_code, 0);
        assert_eq!(update_metrics.packages_updated, 5);
        assert_eq!(update_metrics.packages_available, 10);
        assert!(update_metrics.reboot_required);
    }

    #[tokio::test]
    async fn test_system_metrics_collection() {
        let config = MetricsConfig {
            enabled: true,
            port: Some(9100),
            textfile_path: None,
            collect_system_metrics: true,
        };
        
        let collector = MetricsCollector::new(config).unwrap();
        let system_metrics = collector.collect_system_metrics().await.unwrap();
        
        // Basic sanity checks
        assert!(system_metrics.memory_total_bytes > 0);
        assert!(system_metrics.uptime_seconds > 0);
    }
}