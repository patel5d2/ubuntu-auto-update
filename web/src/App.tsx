import { BrowserRouter as Router, Route, Routes, Navigate } from 'react-router-dom';
import { HostList } from './pages/HostList';
import { HostDetail } from './pages/HostDetail';
import { LoginPage } from './pages/LoginPage';
import { ProtectedRoute } from './components/ProtectedRoute';
import { ExecuteScript } from './pages/ExecuteScript';
import { BulkUpdate } from './pages/BulkUpdate';
import { Overview } from './pages/Overview';
import { Schedules } from './pages/Schedules';
import { Playbooks } from './pages/Playbooks';
import { Settings } from './pages/Settings';
import { Layout } from './components/Layout';
import { ToastProvider } from './components/Toast';
import { ConfirmProvider } from './components/ConfirmDialog';
import { EventsProvider } from './hooks/useEvents';

function App() {
  return (
    <ToastProvider>
      <ConfirmProvider>
        <EventsProvider>
          <Router>
            <Routes>
              <Route path="/login" element={<LoginPage />} />
              <Route element={<ProtectedRoute />}>
                <Route element={<Layout />}>
                  <Route path="/" element={<Navigate to="/overview" replace />} />
                  <Route path="/overview" element={<Overview />} />
                  <Route path="/hosts" element={<HostList />} />
                  <Route path="/hosts/bulk/:groupId" element={<BulkUpdate />} />
                  <Route path="/hosts/:hostId" element={<HostDetail />} />
                  <Route path="/hosts/:hostId/execute-script" element={<ExecuteScript />} />
                  <Route path="/schedules" element={<Schedules />} />
                  <Route path="/playbooks" element={<Playbooks />} />
                  <Route path="/settings" element={<Settings />} />
                </Route>
              </Route>
            </Routes>
          </Router>
        </EventsProvider>
      </ConfirmProvider>
    </ToastProvider>
  );
}

export default App;
