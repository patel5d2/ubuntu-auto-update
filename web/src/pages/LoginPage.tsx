import { useState } from 'react';
import { apiLogin } from '../api';

export function LoginPage() {
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError('');
    setIsLoading(true);

    const data = new FormData(event.currentTarget);
    const username = String(data.get('username') ?? '');
    const password = String(data.get('password') ?? '');

    try {
      await apiLogin(username, password);
      window.location.href = '/';
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Login failed';
      setError(message);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <main style={{ minHeight: '100vh', display: 'grid', placeItems: 'center', padding: '1.5rem' }}>
      <article style={{ width: '100%', maxWidth: '24rem', margin: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', fontWeight: 700, letterSpacing: '-0.02em', marginBottom: '1.25rem' }}>
          <span style={{ width: '0.55rem', height: '0.55rem', borderRadius: '50%', background: 'var(--accent)', flexShrink: 0 }} />
          Ubuntu Auto-Update
        </div>
        <hgroup>
          <h1 style={{ marginBottom: '0.25rem' }}>Sign in</h1>
          <h2 style={{ color: 'var(--ink-muted)' }}>Manage your fleet's updates</h2>
        </hgroup>
        <form onSubmit={handleSubmit}>
          <input type="text" name="username" placeholder="Username" aria-label="Username" required />
          <input type="password" name="password" placeholder="Password" aria-label="Password" required />
          {error && <small style={{ color: 'var(--bad)', display: 'block', marginBottom: '0.5rem' }}>{error}</small>}
          <button type="submit" disabled={isLoading}>
            {isLoading ? 'Logging in...' : 'Login'}
          </button>
        </form>
      </article>
    </main>
  );
}
