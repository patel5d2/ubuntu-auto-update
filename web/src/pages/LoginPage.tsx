import { useState } from 'react';
import { apiLogin } from '../api';

export function LoginPage() {
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError('');
    setIsLoading(true);

    const form = event.currentTarget;
    const username = (form.elements.namedItem('username') as HTMLInputElement).value;
    const password = (form.elements.namedItem('password') as HTMLInputElement).value;

    try {
      await apiLogin(username, password);
      window.location.href = '/';
    } catch (err) {
      console.error('Login error:', err);
      setError('Invalid username or password. Please try again.');
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
            {error && <small style={{ color: 'var(--pico-color-red-500)' }}>{error}</small>}
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
