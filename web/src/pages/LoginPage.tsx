import { useState } from 'react';

export function LoginPage() {
  const [error, setError] = useState('');

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError('');
    const form = event.currentTarget;
    const username = (form.elements.namedItem('username') as HTMLInputElement).value;
    const password = (form.elements.namedItem('password') as HTMLInputElement).value;

    try {
      // For development, allow demo login without backend
      if (username === 'admin' && password === 'admin') {
        localStorage.setItem('auth_token', 'demo-token');
        window.location.href = '/';
        return;
      }

      const response = await fetch('http://localhost:8080/api/v1/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ username, password }),
      });

      if (response.ok) {
        window.location.href = '/'; // Redirect to dashboard on success
      } else {
        setError('Invalid username or password');
      }
    } catch (error) {
      console.error('Login error:', error);
      setError('Unable to connect to server. Try admin/admin for demo mode.');
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
            <button type="submit" className="contrast">Login</button>
          </form>
        </div>
        <div></div>
      </article>
    </main>
  );
}
