import { Navigate, Outlet } from 'react-router-dom';

const useAuth = () => {
  const isAuthenticated = document.cookie.includes('auth_token=');
  console.log("Checking auth. document.cookie:", document.cookie);
  console.log("Is authenticated:", isAuthenticated);
  return isAuthenticated;
};

export function ProtectedRoute() {
  const isAuth = useAuth();
  return isAuth ? <Outlet /> : <Navigate to="/login" />;
}
