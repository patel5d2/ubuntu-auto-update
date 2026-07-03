import { NavLink, Outlet, useNavigate } from 'react-router-dom';
import { apiLogout, canDoOperator, currentRole } from '../api';

// App shell: sidebar navigation + content outlet. Every protected page
// renders inside this.
export function Layout() {
  const navigate = useNavigate();

  const handleLogout = async (e: React.MouseEvent) => {
    e.preventDefault();
    try {
      await apiLogout();
    } finally {
      navigate('/login');
    }
  };

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">Ubuntu Auto-Update</div>
        <nav>
          <NavLink to="/overview">Overview</NavLink>
          <NavLink to="/hosts">Hosts</NavLink>
          <NavLink to="/schedules">Schedules</NavLink>
          {canDoOperator() && <NavLink to="/settings">Settings</NavLink>}
        </nav>
        <div className="sidebar-footer">
          <span>
            Signed in as <code>{currentRole()}</code>
          </span>
          <a href="/login" onClick={handleLogout}>
            Log out
          </a>
        </div>
      </aside>
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}
