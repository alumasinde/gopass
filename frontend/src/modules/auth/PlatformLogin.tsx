import { useState } from 'react';
import type { FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { usePlatformAuth } from '../../contexts/PlatformAuthContext';
import { apiErrorMessage } from '../../lib/api';

export default function PlatformLogin() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const { login } = usePlatformAuth();
  const navigate = useNavigate();

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setIsLoading(true);

    try {
      await login(email, password);
      navigate('/platform/dashboard');
    } catch (err: any) {
      setError(apiErrorMessage(err, 'Login failed. Please check your credentials.'));
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="extracted-style-2f3af921d8">
      <div className="extracted-style-f89833f3d6">
        <h1 className="extracted-style-57b79321ce">Platform Admin Login</h1>
        
        {error && (
          <div className="extracted-style-bfdca8fd79">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit}>
          <div className="extracted-style-363cdaa22c">
            <label htmlFor="email" className="extracted-style-fa7d82998f">
              Email
            </label>
            <input
              id="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              className="extracted-style-ddcfa2b1f1"
              placeholder="admin@example.com"
            />
          </div>

          <div className="extracted-style-561f378bf0">
            <label htmlFor="password" className="extracted-style-fa7d82998f">
              Password
            </label>
            <input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              className="extracted-style-ddcfa2b1f1"
              placeholder="••••••••••••••"
            />
          </div>

          <button
            type="submit"
            disabled={isLoading}
            className="legacy-action-button legacy-action-button-submit legacy-action-button-full"
          >
            {isLoading ? 'Signing in...' : 'Sign In'}
          </button>
        </form>
        <p className="extracted-style-5f51ba28d8">
          Need your company workspace? <Link to="/tenant/login" className="extracted-style-22a6de80d6">Tenant sign in</Link>
        </p>
      </div>
    </div>
  );
}
