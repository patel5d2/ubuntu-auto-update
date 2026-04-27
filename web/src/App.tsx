import { BrowserRouter as Router, Route, Routes } from 'react-router-dom';
import { HostList } from './pages/HostList';
import { HostDetail } from './pages/HostDetail';
import { LoginPage } from './pages/LoginPage';
import { Dashboard } from './pages/Dashboard';
import { ProtectedRoute } from './components/ProtectedRoute';
import { ThemeProvider } from './components/design-system/ThemeProvider';
import { ExecuteScript } from './pages/ExecuteScript';

function App() {
  return (
    <ThemeProvider>
      <Router>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route element={<ProtectedRoute />}>
            <Route path="/*" element={<DashboardLayout />} />
          </Route>
        </Routes>
      </Router>
    </ThemeProvider>
  );
}

function DashboardLayout() {
  return (
    <Routes>
      <Route path="/" element={<Dashboard />} />
      <Route path="/dashboard" element={<Dashboard />} />
      <Route path="/hosts" element={<HostList />} />
      <Route path="/hosts/:hostId" element={<HostDetail />} />
      <Route path="/hosts/:hostId/execute-script" element={<ExecuteScript />} />
    </Routes>
  );
}

export default App;
