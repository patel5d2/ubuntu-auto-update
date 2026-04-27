import { Navigate, Outlet } from 'react-router-dom';
import { isAuthenticated } from '../api';

export function ProtectedRoute() {
  const isAuth = isAuthenticated();
  return isAuth ? <Outlet /> : <Navigate to="/login" />;
}
