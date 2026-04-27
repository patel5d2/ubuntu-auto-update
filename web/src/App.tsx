import { BrowserRouter as Router, Route, Routes, Navigate } from 'react-router-dom';
import { HostList } from './pages/HostList';
import { HostDetail } from './pages/HostDetail';
import { LoginPage } from './pages/LoginPage';
import { ProtectedRoute } from './components/ProtectedRoute';
import { ExecuteScript } from './pages/ExecuteScript';

function App() {
  return (
    <Router>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<ProtectedRoute />}>
          <Route path="/" element={<Navigate to="/hosts" replace />} />
          <Route path="/hosts" element={<HostList />} />
          <Route path="/hosts/:hostId" element={<HostDetail />} />
          <Route path="/hosts/:hostId/execute-script" element={<ExecuteScript />} />
        </Route>
      </Routes>
    </Router>
  );
}

export default App;
