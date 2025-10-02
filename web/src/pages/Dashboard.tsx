import { useState, useEffect } from 'react';
import { useTheme } from '../components/design-system/ThemeProvider';
import { Button } from '../components/ui/Button';

interface SystemMetrics {
  totalHosts: number;
  onlineHosts: number;
  offlineHosts: number;
  hostsWithUpdates: number;
  successfulUpdates: number;
  failedUpdates: number;
  lastUpdateTime: string;
  systemLoad: {
    cpu: number;
    memory: number;
    disk: number;
  };
  uptimeStats: {
    '24h': number;
    '7d': number;
    '30d': number;
  };
  updateHistory: Array<{
    date: string;
    successful: number;
    failed: number;
  }>;
}

interface Host {
  id: number;
  hostname: string;
  status: 'online' | 'offline' | 'updating' | 'error';
  lastSeen: string;
  updatesAvailable: number;
  criticalUpdates: number;
  systemInfo: {
    os: string;
    kernel: string;
    uptime: string;
    load: number;
  };
}

interface AlertItem {
  id: string;
  type: 'info' | 'warning' | 'error' | 'success';
  title: string;
  message: string;
  timestamp: string;
  read: boolean;
}

export function Dashboard() {
  const { isDark, toggleTheme } = useTheme();
  const [metrics, setMetrics] = useState<SystemMetrics | null>(null);
  const [hosts, setHosts] = useState<Host[]>([]);
  const [alerts, setAlerts] = useState<AlertItem[]>([]);
  const [selectedTimeRange] = useState<'24h' | '7d' | '30d'>('24h');
  const [isLoading, setIsLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedHostStatus, setSelectedHostStatus] = useState<string>('all');

  // Simulate real-time data updates
  useEffect(() => {
    const fetchDashboardData = async () => {
      try {
        setIsLoading(true);
        
        // Simulate API calls - keeping for future integration
        // await Promise.all([
        //   fetch('http://localhost:8081/api/v1/metrics'),
        //   fetch('http://localhost:8081/api/v1/hosts'),
        //   fetch('http://localhost:8081/api/v1/alerts')
        // ]);

        // Mock data for demonstration
        const mockMetrics: SystemMetrics = {
          totalHosts: 247,
          onlineHosts: 234,
          offlineHosts: 13,
          hostsWithUpdates: 89,
          successfulUpdates: 1456,
          failedUpdates: 23,
          lastUpdateTime: new Date().toISOString(),
          systemLoad: {
            cpu: 68,
            memory: 74,
            disk: 45
          },
          uptimeStats: {
            '24h': 99.8,
            '7d': 99.5,
            '30d': 99.2
          },
          updateHistory: generateMockHistory()
        };

        const mockHosts: Host[] = [
          {
            id: 1,
            hostname: 'prod-web-01.company.com',
            status: 'online',
            lastSeen: '2 minutes ago',
            updatesAvailable: 5,
            criticalUpdates: 1,
            systemInfo: {
              os: 'Ubuntu 22.04 LTS',
              kernel: '5.15.0-84-generic',
              uptime: '45 days, 3 hours',
              load: 0.85
            }
          },
          {
            id: 2,
            hostname: 'prod-db-01.company.com', 
            status: 'updating',
            lastSeen: '1 minute ago',
            updatesAvailable: 12,
            criticalUpdates: 3,
            systemInfo: {
              os: 'Ubuntu 20.04 LTS',
              kernel: '5.4.0-128-generic',
              uptime: '87 days, 12 hours',
              load: 2.34
            }
          },
          {
            id: 3,
            hostname: 'staging-app-02.company.com',
            status: 'error',
            lastSeen: '15 minutes ago',
            updatesAvailable: 0,
            criticalUpdates: 0,
            systemInfo: {
              os: 'Ubuntu 22.04 LTS',
              kernel: '5.15.0-82-generic',
              uptime: '12 days, 8 hours',
              load: 0.42
            }
          }
        ];

        const mockAlerts: AlertItem[] = [
          {
            id: '1',
            type: 'error',
            title: 'Critical Security Updates Available',
            message: '15 hosts have critical security updates that require immediate attention',
            timestamp: '5 minutes ago',
            read: false
          },
          {
            id: '2',
            type: 'warning',
            title: 'High System Load Detected',
            message: 'prod-db-01.company.com is experiencing high CPU load (95%)',
            timestamp: '12 minutes ago',
            read: false
          },
          {
            id: '3',
            type: 'success',
            title: 'Batch Updates Completed',
            message: 'Successfully updated 45 hosts in the staging environment',
            timestamp: '1 hour ago',
            read: true
          }
        ];

        setMetrics(mockMetrics);
        setHosts(mockHosts);
        setAlerts(mockAlerts);
      } catch (error) {
        console.error('Failed to fetch dashboard data:', error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchDashboardData();
    
    // Set up real-time updates
    const interval = setInterval(fetchDashboardData, 30000); // Update every 30 seconds
    return () => clearInterval(interval);
  }, []);

  const generateMockHistory = () => {
    const history = [];
    for (let i = 6; i >= 0; i--) {
      const date = new Date();
      date.setDate(date.getDate() - i);
      history.push({
        date: date.toISOString().split('T')[0],
        successful: Math.floor(Math.random() * 100) + 50,
        failed: Math.floor(Math.random() * 10) + 1
      });
    }
    return history;
  };

  const filteredHosts = hosts.filter(host => {
    const matchesSearch = host.hostname.toLowerCase().includes(searchQuery.toLowerCase());
    const matchesStatus = selectedHostStatus === 'all' || host.status === selectedHostStatus;
    return matchesSearch && matchesStatus;
  });

  // Helper function for status colors - keeping for future use
  // const getStatusColor = (status: string) => {
  //   switch (status) {
  //     case 'online': return 'text-success-600';
  //     case 'offline': return 'text-secondary-500';
  //     case 'updating': return 'text-primary-600';
  //     case 'error': return 'text-error-600';
  //     default: return 'text-secondary-500';
  //   }
  // };

  const getStatusBadgeColor = (status: string) => {
    switch (status) {
      case 'online': return 'bg-success-100 text-success-800';
      case 'offline': return 'bg-secondary-100 text-secondary-800';
      case 'updating': return 'bg-primary-100 text-primary-800';
      case 'error': return 'bg-error-100 text-error-800';
      default: return 'bg-secondary-100 text-secondary-800';
    }
  };

  const getAlertColor = (type: string) => {
    switch (type) {
      case 'info': return 'border-l-primary-500 bg-primary-50';
      case 'warning': return 'border-l-warning-500 bg-warning-50';
      case 'error': return 'border-l-error-500 bg-error-50';
      case 'success': return 'border-l-success-500 bg-success-50';
      default: return 'border-l-secondary-500 bg-secondary-50';
    }
  };

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600"></div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background">
      {/* Header */}
      <header className="bg-surface shadow-sm border-b border-border-primary">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex justify-between items-center h-16">
            <div className="flex items-center">
              <h1 className="text-2xl font-bold text-text-primary">Ubuntu Auto-Update</h1>
              <span className="ml-2 px-2 py-1 text-xs font-medium bg-primary-100 text-primary-800 rounded-full">
                Enterprise
              </span>
            </div>
            
            <div className="flex items-center space-x-4">
              {/* Theme Toggle */}
              <Button
                variant="ghost"
                size="sm"
                onClick={toggleTheme}
                leftIcon={
                  isDark ? (
                    <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                      <path fillRule="evenodd" d="M10 2a1 1 0 011 1v1a1 1 0 11-2 0V3a1 1 0 011-1zm4 8a4 4 0 11-8 0 4 4 0 018 0zm-.464 4.95l.707.707a1 1 0 001.414-1.414l-.707-.707a1 1 0 00-1.414 1.414zm2.12-10.607a1 1 0 010 1.414l-.706.707a1 1 0 11-1.414-1.414l.707-.707a1 1 0 011.414 0zM17 11a1 1 0 100-2h-1a1 1 0 100 2h1zm-7 4a1 1 0 011 1v1a1 1 0 11-2 0v-1a1 1 0 011-1zM5.05 6.464A1 1 0 106.465 5.05l-.708-.707a1 1 0 00-1.414 1.414l.707.707zm1.414 8.486l-.707.707a1 1 0 01-1.414-1.414l.707-.707a1 1 0 011.414 1.414zM4 11a1 1 0 100-2H3a1 1 0 000 2h1z" clipRule="evenodd" />
                    </svg>
                  ) : (
                    <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                      <path d="M17.293 13.293A8 8 0 016.707 2.707a8.001 8.001 0 1010.586 10.586z" />
                    </svg>
                  )
                }
              >
                {isDark ? 'Light' : 'Dark'}
              </Button>
              
              {/* Notifications */}
              <div className="relative">
                <Button variant="ghost" size="sm">
                  <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 17h5l-5-5m0 0l-5 5m5-5V3" />
                  </svg>
                  {alerts.filter(alert => !alert.read).length > 0 && (
                    <span className="absolute -top-1 -right-1 h-4 w-4 bg-error-500 text-white text-xs rounded-full flex items-center justify-center">
                      {alerts.filter(alert => !alert.read).length}
                    </span>
                  )}
                </Button>
              </div>

              {/* User Menu */}
              <div className="flex items-center space-x-2">
                <div className="h-8 w-8 bg-primary-600 rounded-full flex items-center justify-center">
                  <span className="text-sm font-medium text-white">A</span>
                </div>
                <span className="text-sm font-medium text-text-primary">Admin</span>
              </div>
            </div>
          </div>
        </div>
      </header>

      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        {/* Key Metrics Cards */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
          <div className="bg-surface p-4 rounded-lg shadow-sm border border-border-primary">
            <div className="flex items-center">
              <div className="flex-1">
                <p className="text-sm font-medium text-text-secondary">Total Hosts</p>
                <p className="text-xl font-bold text-text-primary">{metrics?.totalHosts}</p>
              </div>
              <div className="h-8 w-8 bg-primary-100 rounded-lg flex items-center justify-center">
                <svg className="h-4 w-4 text-primary-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
                </svg>
              </div>
            </div>
          </div>

          <div className="bg-surface p-4 rounded-lg shadow-sm border border-border-primary">
            <div className="flex items-center">
              <div className="flex-1">
                <p className="text-sm font-medium text-text-secondary">Online Hosts</p>
                <p className="text-xl font-bold text-success-600">{metrics?.onlineHosts}</p>
              </div>
              <div className="h-8 w-8 bg-success-100 rounded-lg flex items-center justify-center">
                <svg className="h-4 w-4 text-success-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
            </div>
          </div>

          <div className="bg-surface p-4 rounded-lg shadow-sm border border-border-primary">
            <div className="flex items-center">
              <div className="flex-1">
                <p className="text-sm font-medium text-text-secondary">Updates Available</p>
                <p className="text-xl font-bold text-warning-600">{metrics?.hostsWithUpdates}</p>
              </div>
              <div className="h-8 w-8 bg-warning-100 rounded-lg flex items-center justify-center">
                <svg className="h-4 w-4 text-warning-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.728-.833-2.498 0L4.732 16.5c-.77.833.192 2.5 1.732 2.5z" />
                </svg>
              </div>
            </div>
          </div>

          <div className="bg-surface p-4 rounded-lg shadow-sm border border-border-primary">
            <div className="flex items-center">
              <div className="flex-1">
                <p className="text-sm font-medium text-text-secondary">System Uptime</p>
                <p className="text-xl font-bold text-text-primary">{metrics?.uptimeStats[selectedTimeRange]}%</p>
              </div>
              <div className="h-8 w-8 bg-secondary-100 rounded-lg flex items-center justify-center">
                <svg className="h-4 w-4 text-secondary-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                </svg>
              </div>
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          {/* Hosts Table */}
          <div className="lg:col-span-2">
            <div className="bg-surface rounded-lg shadow-sm border border-border-primary">
              <div className="px-6 py-4 border-b border-border-primary">
                <div className="flex items-center justify-between">
                  <h3 className="text-lg font-medium text-text-primary">Managed Hosts</h3>
                  <div className="flex items-center space-x-4">
                    <div className="relative">
                      <input
                        type="text"
                        placeholder="Search hosts..."
                        value={searchQuery}
                        onChange={(e) => setSearchQuery(e.target.value)}
                        className="pl-10 pr-4 py-2 border border-border-primary rounded-md bg-background text-text-primary placeholder-text-secondary focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
                      />
                      <svg className="absolute left-3 top-2.5 h-4 w-4 text-text-secondary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                      </svg>
                    </div>
                    <select
                      value={selectedHostStatus}
                      onChange={(e) => setSelectedHostStatus(e.target.value)}
                      className="border border-border-primary rounded-md bg-background text-text-primary focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500 px-3 py-2"
                    >
                      <option value="all">All Status</option>
                      <option value="online">Online</option>
                      <option value="offline">Offline</option>
                      <option value="updating">Updating</option>
                      <option value="error">Error</option>
                    </select>
                  </div>
                </div>
              </div>

              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-secondary-50">
                    <tr>
                      <th className="px-6 py-3 text-left text-xs font-medium text-text-secondary uppercase tracking-wider">Host</th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-text-secondary uppercase tracking-wider">Status</th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-text-secondary uppercase tracking-wider">Updates</th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-text-secondary uppercase tracking-wider">Last Seen</th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-text-secondary uppercase tracking-wider">Actions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border-primary">
                    {filteredHosts.map((host) => (
                      <tr key={host.id} className="hover:bg-secondary-50 transition-colors">
                        <td className="px-6 py-4 whitespace-nowrap">
                          <div>
                            <div className="text-sm font-medium text-text-primary">{host.hostname}</div>
                            <div className="text-sm text-text-secondary">{host.systemInfo.os}</div>
                          </div>
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap">
                          <span className={`inline-flex px-2 py-1 text-xs font-semibold rounded-full ${getStatusBadgeColor(host.status)}`}>
                            {host.status}
                          </span>
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap">
                          <div className="text-sm text-text-primary">{host.updatesAvailable} available</div>
                          {host.criticalUpdates > 0 && (
                            <div className="text-xs text-error-600">{host.criticalUpdates} critical</div>
                          )}
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap text-sm text-text-secondary">
                          {host.lastSeen}
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap text-sm text-text-secondary">
                          <div className="flex space-x-2">
                            <Button size="xs" variant="primary">Update</Button>
                            <Button size="xs" variant="secondary">Details</Button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          </div>

          {/* Alerts Panel */}
          <div className="space-y-6">
            <div className="bg-surface rounded-lg shadow-sm border border-border-primary">
              <div className="px-6 py-4 border-b border-border-primary">
                <h3 className="text-lg font-medium text-text-primary">Recent Alerts</h3>
              </div>
              <div className="p-6 space-y-4">
                {alerts.slice(0, 5).map((alert) => (
                  <div key={alert.id} className={`p-4 border-l-4 rounded-r-md ${getAlertColor(alert.type)}`}>
                    <div className="flex">
                      <div className="flex-1">
                        <p className="text-sm font-medium text-text-primary">{alert.title}</p>
                        <p className="text-sm text-text-secondary mt-1">{alert.message}</p>
                        <p className="text-xs text-text-secondary mt-2">{alert.timestamp}</p>
                      </div>
                      {!alert.read && (
                        <div className="h-2 w-2 bg-primary-600 rounded-full mt-2"></div>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>

            {/* System Load */}
            <div className="bg-surface rounded-lg shadow-sm border border-border-primary">
              <div className="px-6 py-4 border-b border-border-primary">
                <h3 className="text-lg font-medium text-text-primary">System Load</h3>
              </div>
              <div className="p-6 space-y-4">
                <div>
                  <div className="flex justify-between text-sm">
                    <span className="text-text-secondary">CPU Usage</span>
                    <span className="text-text-primary">{metrics?.systemLoad.cpu}%</span>
                  </div>
                  <div className="w-full bg-secondary-200 rounded-full h-2 mt-1">
                    <div 
                      className="bg-primary-600 h-2 rounded-full" 
                      style={{ width: `${metrics?.systemLoad.cpu}%` }}
                    ></div>
                  </div>
                </div>

                <div>
                  <div className="flex justify-between text-sm">
                    <span className="text-text-secondary">Memory Usage</span>
                    <span className="text-text-primary">{metrics?.systemLoad.memory}%</span>
                  </div>
                  <div className="w-full bg-secondary-200 rounded-full h-2 mt-1">
                    <div 
                      className="bg-warning-500 h-2 rounded-full" 
                      style={{ width: `${metrics?.systemLoad.memory}%` }}
                    ></div>
                  </div>
                </div>

                <div>
                  <div className="flex justify-between text-sm">
                    <span className="text-text-secondary">Disk Usage</span>
                    <span className="text-text-primary">{metrics?.systemLoad.disk}%</span>
                  </div>
                  <div className="w-full bg-secondary-200 rounded-full h-2 mt-1">
                    <div 
                      className="bg-success-500 h-2 rounded-full" 
                      style={{ width: `${metrics?.systemLoad.disk}%` }}
                    ></div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}