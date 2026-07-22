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
    <main className="container">
      <article className="grid">
        <div>
          <hgroup>
            <h1>Sign in</h1>
            <h2>Ubuntu Auto-Update</h2>
          </hgroup>
          <form onSubmit={handleSubmit}>
            <input type="text" name="username" placeholder="Username" aria-label="Username" required />
            <input type="password" name="password" placeholder="Password" aria-label="Password" required />
            {error && <small style={{ color: 'var(--bad)' }}>{error}</small>}
            <button type="submit" className="contrast" disabled={isLoading}>
              {isLoading ? 'Logging in...' : 'Login'}
            </button>
          </form>
        </div>
        <div></div>
      </article>
    </main>
  );
}
