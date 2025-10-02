import { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
// import { useTheme } from '../components/design-system/ThemeProvider';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { Modal, ConfirmModal } from '../components/ui/Modal';
import { Card, CardHeader, CardContent, StatCard, AlertCard } from '../components/ui/Card';

interface HostDetails {
  id: number;
  hostname: string;
  ip: string;
  status: 'online' | 'offline' | 'updating' | 'error' | 'maintenance';
  lastSeen: string;
  uptime: string;
  os: string;
  kernel: string;
  architecture: string;
  updatesAvailable: number;
  criticalUpdates: number;
  systemInfo: {
    cpu: {
      model: string;
      cores: number;
      usage: number;
    };
    memory: {
      total: string;
      used: string;
      usage: number;
    };
    disk: {
      total: string;
      used: string;
      usage: number;
    };
    network: {
      interface: string;
      rx: string;
      tx: string;
    };
  };
  services: Array<{
    name: string;
    status: 'running' | 'stopped' | 'failed' | 'inactive';
    uptime: string;
  }>;
  updateHistory: Array<{
    date: string;
    type: 'security' | 'system' | 'application';
    packages: number;
    status: 'success' | 'failed' | 'partial';
  }>;
  scheduledMaintenance?: {
    date: string;
    duration: string;
    type: string;
  };
}

export function HostDetailAdvanced() {
  const { hostId } = useParams<{ hostId: string }>();
  const navigate = useNavigate();
  // const { theme, isDark } = useTheme(); // Available for theme customization
  
  const [host, setHost] = useState<HostDetails | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isUpdating, setIsUpdating] = useState(false);
  const [showUpdateModal, setShowUpdateModal] = useState(false);
  const [showScheduleModal, setShowScheduleModal] = useState(false);
  const [showRebootModal, setShowRebootModal] = useState(false);
  const [selectedTab, setSelectedTab] = useState<'overview' | 'services' | 'updates' | 'logs'>('overview');
  const [scheduledDate, setScheduledDate] = useState('');
  const [scheduledTime, setScheduledTime] = useState('');

  useEffect(() => {
    const fetchHostDetails = async () => {
      try {
        setIsLoading(true);
        // Mock API call
        const mockHost: HostDetails = {
          id: parseInt(hostId || '1'),
          hostname: 'prod-web-01.company.com',
          ip: '10.0.1.15',
          status: 'online',
          lastSeen: '2 minutes ago',
          uptime: '45 days, 3 hours, 12 minutes',
          os: 'Ubuntu 22.04.3 LTS',
          kernel: '5.15.0-84-generic',
          architecture: 'x86_64',
          updatesAvailable: 12,
          criticalUpdates: 3,
          systemInfo: {
            cpu: {
              model: 'Intel(R) Xeon(R) CPU E5-2670 v3',
              cores: 8,
              usage: 23
            },
            memory: {
              total: '16.0 GB',
              used: '8.7 GB',
              usage: 54
            },
            disk: {
              total: '500 GB',
              used: '89 GB',
              usage: 18
            },
            network: {
              interface: 'eth0',
              rx: '2.3 TB',
              tx: '1.8 TB'
            }
          },
          services: [
            { name: 'nginx', status: 'running', uptime: '45d 3h' },
            { name: 'postgresql', status: 'running', uptime: '45d 3h' },
            { name: 'redis', status: 'running', uptime: '45d 2h' },
            { name: 'docker', status: 'running', uptime: '45d 3h' },
            { name: 'fail2ban', status: 'stopped', uptime: '-' }
          ],
          updateHistory: [
            { date: '2023-10-15', type: 'security', packages: 5, status: 'success' },
            { date: '2023-10-10', type: 'system', packages: 23, status: 'success' },
            { date: '2023-10-05', type: 'application', packages: 8, status: 'partial' },
            { date: '2023-09-28', type: 'security', packages: 12, status: 'success' }
          ],
          scheduledMaintenance: {
            date: '2023-10-25',
            duration: '2 hours',
            type: 'System Updates & Security Patches'
          }
        };
        
        setHost(mockHost);
      } catch (error) {
        console.error('Failed to fetch host details:', error);
      } finally {
        setIsLoading(false);
      }
    };

    if (hostId) {
      fetchHostDetails();
    }
  }, [hostId]);

  const handleUpdateHost = async () => {
    setIsUpdating(true);
    try {
      // Simulate update process
      await new Promise(resolve => setTimeout(resolve, 3000));
      setShowUpdateModal(false);
      // Refresh host data
    } catch (error) {
      console.error('Update failed:', error);
    } finally {
      setIsUpdating(false);
    }
  };

  const handleScheduleMaintenance = () => {
    if (scheduledDate && scheduledTime) {
      console.log('Scheduling maintenance for:', scheduledDate, scheduledTime);
      setShowScheduleModal(false);
      setScheduledDate('');
      setScheduledTime('');
    }
  };

  const handleReboot = async () => {
    try {
      // Simulate reboot
      await new Promise(resolve => setTimeout(resolve, 1000));
      setShowRebootModal(false);
      console.log('Rebooting host...');
    } catch (error) {
      console.error('Reboot failed:', error);
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'online': return 'text-success-600';
      case 'offline': return 'text-secondary-500';
      case 'updating': return 'text-primary-600';
      case 'error': return 'text-error-600';
      case 'maintenance': return 'text-warning-600';
      default: return 'text-secondary-500';
    }
  };

  const getServiceStatusColor = (status: string) => {
    switch (status) {
      case 'running': return 'bg-success-100 text-success-800';
      case 'stopped': return 'bg-secondary-100 text-secondary-800';
      case 'failed': return 'bg-error-100 text-error-800';
      case 'inactive': return 'bg-warning-100 text-warning-800';
      default: return 'bg-secondary-100 text-secondary-800';
    }
  };

  const getUpdateStatusColor = (status: string) => {
    switch (status) {
      case 'success': return 'text-success-600';
      case 'failed': return 'text-error-600';
      case 'partial': return 'text-warning-600';
      default: return 'text-secondary-600';
    }
  };

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600"></div>
      </div>
    );
  }

  if (!host) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-center">
          <h2 className="text-2xl font-bold text-text-primary mb-2">Host Not Found</h2>
          <p className="text-text-secondary mb-4">The requested host could not be found.</p>
          <Button onClick={() => navigate('/hosts')}>Back to Hosts</Button>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background">
      {/* Header */}
      <div className="bg-surface border-b border-border-primary">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="py-6">
            <div className="flex items-center justify-between">
              <div className="flex items-center space-x-4">
                <Button 
                  variant="ghost" 
                  size="sm" 
                  onClick={() => navigate(-1)}
                  leftIcon={
                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
                    </svg>
                  }
                >
                  Back
                </Button>
                <div>
                  <h1 className="text-2xl font-bold text-text-primary">{host.hostname}</h1>
                  <div className="flex items-center space-x-4 mt-1">
                    <span className={`text-sm ${getStatusColor(host.status)}`}>
                      ● {host.status.charAt(0).toUpperCase() + host.status.slice(1)}
                    </span>
                    <span className="text-sm text-text-secondary">{host.ip}</span>
                    <span className="text-sm text-text-secondary">Last seen: {host.lastSeen}</span>
                  </div>
                </div>
              </div>

              <div className="flex items-center space-x-3">
                <Button 
                  variant="secondary" 
                  size="sm"
                  onClick={() => setShowScheduleModal(true)}
                  leftIcon={
                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
                    </svg>
                  }
                >
                  Schedule
                </Button>
                <Button 
                  variant="warning"
                  size="sm"
                  onClick={() => setShowRebootModal(true)}
                  leftIcon={
                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                    </svg>
                  }
                >
                  Reboot
                </Button>
                <Button 
                  variant="primary"
                  onClick={() => setShowUpdateModal(true)}
                  disabled={host.updatesAvailable === 0}
                  leftIcon={
                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                    </svg>
                  }
                >
                  Update ({host.updatesAvailable})
                </Button>
              </div>
            </div>

            {/* Tabs */}
            <div className="mt-6">
              <nav className="flex space-x-8">
                {[
                  { key: 'overview', label: 'Overview' },
                  { key: 'services', label: 'Services' },
                  { key: 'updates', label: 'Updates' },
                  { key: 'logs', label: 'Logs' }
                ].map((tab) => (
                  <button
                    key={tab.key}
                    onClick={() => setSelectedTab(tab.key as any)}
                    className={`py-2 px-1 border-b-2 font-medium text-sm transition-colors ${
                      selectedTab === tab.key
                        ? 'border-primary-500 text-primary-600'
                        : 'border-transparent text-text-secondary hover:text-text-primary hover:border-secondary-300'
                    }`}
                  >
                    {tab.label}
                  </button>
                ))}
              </nav>
            </div>
          </div>
        </div>
      </div>

      {/* Content */}
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        {/* Alerts */}
        {host.criticalUpdates > 0 && (
          <div className="mb-6">
            <AlertCard
              type="error"
              title="Critical Security Updates Available"
              message={`${host.criticalUpdates} critical security updates require immediate attention.`}
              action={
                <Button size="sm" variant="error" onClick={() => setShowUpdateModal(true)}>
                  Update Now
                </Button>
              }
            />
          </div>
        )}

        {host.scheduledMaintenance && (
          <div className="mb-6">
            <AlertCard
              type="warning"
              title="Scheduled Maintenance"
              message={`${host.scheduledMaintenance.type} scheduled for ${host.scheduledMaintenance.date} (${host.scheduledMaintenance.duration})`}
              action={
                <Button size="sm" variant="secondary">
                  View Details
                </Button>
              }
            />
          </div>
        )}

        {/* Tab Content */}
        {selectedTab === 'overview' && (
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            {/* System Stats */}
            <div className="lg:col-span-2 space-y-6">
              <Card>
                <CardHeader>
                  <h3 className="text-lg font-medium text-text-primary">System Information</h3>
                </CardHeader>
                <CardContent>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div>
                      <h4 className="text-sm font-medium text-text-secondary mb-2">Operating System</h4>
                      <p className="text-text-primary">{host.os}</p>
                    </div>
                    <div>
                      <h4 className="text-sm font-medium text-text-secondary mb-2">Kernel</h4>
                      <p className="text-text-primary">{host.kernel}</p>
                    </div>
                    <div>
                      <h4 className="text-sm font-medium text-text-secondary mb-2">Architecture</h4>
                      <p className="text-text-primary">{host.architecture}</p>
                    </div>
                    <div>
                      <h4 className="text-sm font-medium text-text-secondary mb-2">Uptime</h4>
                      <p className="text-text-primary">{host.uptime}</p>
                    </div>
                  </div>
                </CardContent>
              </Card>

              {/* Resource Usage */}
              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                <StatCard
                  title="CPU Usage"
                  value={`${host.systemInfo.cpu.usage}%`}
                  subtitle={`${host.systemInfo.cpu.cores} cores • ${host.systemInfo.cpu.model}`}
                  variant={host.systemInfo.cpu.usage > 80 ? 'error' : host.systemInfo.cpu.usage > 60 ? 'warning' : 'success'}
                  icon={
                    <svg className="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" />
                    </svg>
                  }
                />

                <StatCard
                  title="Memory Usage"
                  value={`${host.systemInfo.memory.usage}%`}
                  subtitle={`${host.systemInfo.memory.used} / ${host.systemInfo.memory.total}`}
                  variant={host.systemInfo.memory.usage > 80 ? 'error' : host.systemInfo.memory.usage > 60 ? 'warning' : 'success'}
                  icon={
                    <svg className="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
                    </svg>
                  }
                />

                <StatCard
                  title="Disk Usage"
                  value={`${host.systemInfo.disk.usage}%`}
                  subtitle={`${host.systemInfo.disk.used} / ${host.systemInfo.disk.total}`}
                  variant={host.systemInfo.disk.usage > 80 ? 'error' : host.systemInfo.disk.usage > 60 ? 'warning' : 'success'}
                  icon={
                    <svg className="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4m0 5c0 2.21-3.582 4-8 4s-8-1.79-8-4" />
                    </svg>
                  }
                />
              </div>
            </div>

            {/* Quick Actions & Network */}
            <div className="space-y-6">
              <Card>
                <CardHeader>
                  <h3 className="text-lg font-medium text-text-primary">Network Interface</h3>
                </CardHeader>
                <CardContent>
                  <div className="space-y-3">
                    <div>
                      <span className="text-sm text-text-secondary">Interface:</span>
                      <span className="ml-2 text-text-primary">{host.systemInfo.network.interface}</span>
                    </div>
                    <div>
                      <span className="text-sm text-text-secondary">RX:</span>
                      <span className="ml-2 text-text-primary">{host.systemInfo.network.rx}</span>
                    </div>
                    <div>
                      <span className="text-sm text-text-secondary">TX:</span>
                      <span className="ml-2 text-text-primary">{host.systemInfo.network.tx}</span>
                    </div>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <h3 className="text-lg font-medium text-text-primary">Quick Actions</h3>
                </CardHeader>
                <CardContent>
                  <div className="space-y-3">
                    <Button fullWidth variant="secondary" size="sm">
                      View Full Logs
                    </Button>
                    <Button fullWidth variant="secondary" size="sm">
                      SSH Terminal
                    </Button>
                    <Button fullWidth variant="secondary" size="sm">
                      Download Config
                    </Button>
                    <Button fullWidth variant="secondary" size="sm">
                      Performance Report
                    </Button>
                  </div>
                </CardContent>
              </Card>
            </div>
          </div>
        )}

        {selectedTab === 'services' && (
          <Card>
            <CardHeader>
              <h3 className="text-lg font-medium text-text-primary">System Services</h3>
              <Button size="sm" variant="secondary">
                Refresh
              </Button>
            </CardHeader>
            <CardContent>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-secondary-50">
                    <tr>
                      <th className="px-6 py-3 text-left text-xs font-medium text-text-secondary uppercase tracking-wider">Service</th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-text-secondary uppercase tracking-wider">Status</th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-text-secondary uppercase tracking-wider">Uptime</th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-text-secondary uppercase tracking-wider">Actions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border-primary">
                    {host.services.map((service, index) => (
                      <tr key={index} className="hover:bg-secondary-50">
                        <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-text-primary">
                          {service.name}
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap">
                          <span className={`inline-flex px-2 py-1 text-xs font-semibold rounded-full ${getServiceStatusColor(service.status)}`}>
                            {service.status}
                          </span>
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap text-sm text-text-secondary">
                          {service.uptime}
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap text-sm">
                          <div className="flex space-x-2">
                            {service.status === 'running' ? (
                              <>
                                <Button size="xs" variant="warning">Stop</Button>
                                <Button size="xs" variant="secondary">Restart</Button>
                              </>
                            ) : (
                              <Button size="xs" variant="success">Start</Button>
                            )}
                            <Button size="xs" variant="secondary">Logs</Button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>
        )}

        {selectedTab === 'updates' && (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <Card>
              <CardHeader>
                <h3 className="text-lg font-medium text-text-primary">Available Updates</h3>
                <Button size="sm" variant="primary" onClick={() => setShowUpdateModal(true)}>
                  Update All
                </Button>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="flex items-center justify-between p-3 bg-error-50 border border-error-200 rounded-md">
                    <div>
                      <p className="text-sm font-medium text-text-primary">Security Updates</p>
                      <p className="text-xs text-text-secondary">{host.criticalUpdates} critical packages</p>
                    </div>
                    <Button size="xs" variant="error">Update</Button>
                  </div>
                  <div className="flex items-center justify-between p-3 bg-warning-50 border border-warning-200 rounded-md">
                    <div>
                      <p className="text-sm font-medium text-text-primary">System Updates</p>
                      <p className="text-xs text-text-secondary">{host.updatesAvailable - host.criticalUpdates} packages</p>
                    </div>
                    <Button size="xs" variant="warning">Update</Button>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <h3 className="text-lg font-medium text-text-primary">Update History</h3>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  {host.updateHistory.map((update, index) => (
                    <div key={index} className="flex items-center justify-between p-3 border border-border-primary rounded-md">
                      <div>
                        <p className="text-sm font-medium text-text-primary">{update.date}</p>
                        <p className="text-xs text-text-secondary capitalize">{update.type} • {update.packages} packages</p>
                      </div>
                      <span className={`text-xs font-medium capitalize ${getUpdateStatusColor(update.status)}`}>
                        {update.status}
                      </span>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          </div>
        )}

        {selectedTab === 'logs' && (
          <Card>
            <CardHeader>
              <h3 className="text-lg font-medium text-text-primary">System Logs</h3>
              <div className="flex space-x-2">
                <Button size="sm" variant="secondary">Download</Button>
                <Button size="sm" variant="secondary">Refresh</Button>
              </div>
            </CardHeader>
            <CardContent>
              <div className="bg-secondary-900 text-secondary-100 p-4 rounded-md font-mono text-xs overflow-x-auto">
                <div>Oct 16 14:23:15 prod-web-01 systemd[1]: Started Daily apt download activities.</div>
                <div>Oct 16 14:23:15 prod-web-01 systemd[1]: Started Daily apt upgrade and clean activities.</div>
                <div>Oct 16 14:20:01 prod-web-01 CRON[1234]: (root) CMD (/usr/local/bin/backup-script.sh)</div>
                <div>Oct 16 14:15:23 prod-web-01 nginx[5678]: 203.0.113.1 - - [16/Oct/2023:14:15:23 +0000] "GET / HTTP/1.1" 200 612</div>
                <div>Oct 16 14:12:45 prod-web-01 kernel: [123456.789012] TCP: request_sock_TCP: Possible SYN flooding</div>
                <div className="text-warning-400">Oct 16 14:10:01 prod-web-01 fail2ban[9012]: WARNING jail 'ssh' has been started</div>
                <div className="text-success-400">Oct 16 14:05:33 prod-web-01 systemd[1]: Reloading nginx configuration...</div>
                <div className="text-error-400">Oct 16 13:58:12 prod-web-01 postgresql[3456]: ERROR: connection to database failed</div>
              </div>
            </CardContent>
          </Card>
        )}
      </div>

      {/* Update Modal */}
      <ConfirmModal
        isOpen={showUpdateModal}
        onClose={() => setShowUpdateModal(false)}
        onConfirm={handleUpdateHost}
        title="Update Host"
        message={`Are you sure you want to install ${host.updatesAvailable} available updates? This may require a system reboot.`}
        confirmText="Update Now"
        variant="default"
        isLoading={isUpdating}
      />

      {/* Schedule Maintenance Modal */}
      <Modal
        isOpen={showScheduleModal}
        onClose={() => setShowScheduleModal(false)}
        title="Schedule Maintenance"
        footer={
          <div className="flex justify-end space-x-3">
            <Button variant="secondary" onClick={() => setShowScheduleModal(false)}>
              Cancel
            </Button>
            <Button variant="primary" onClick={handleScheduleMaintenance}>
              Schedule
            </Button>
          </div>
        }
      >
        <div className="space-y-4">
          <Input
            label="Date"
            type="date"
            value={scheduledDate}
            onChange={(e) => setScheduledDate(e.target.value)}
            required
            fullWidth
          />
          <Input
            label="Time"
            type="time"
            value={scheduledTime}
            onChange={(e) => setScheduledTime(e.target.value)}
            required
            fullWidth
          />
          <Input
            label="Maintenance Type"
            placeholder="e.g., Security Updates, System Maintenance"
            fullWidth
          />
          <Input
            label="Estimated Duration (hours)"
            type="number"
            placeholder="2"
            fullWidth
          />
        </div>
      </Modal>

      {/* Reboot Confirmation Modal */}
      <ConfirmModal
        isOpen={showRebootModal}
        onClose={() => setShowRebootModal(false)}
        onConfirm={handleReboot}
        title="Reboot Host"
        message="Are you sure you want to reboot this host? This will temporarily interrupt all services."
        confirmText="Reboot"
        variant="warning"
      />
    </div>
  );
}