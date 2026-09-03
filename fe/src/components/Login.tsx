import { useState, FC } from 'react';
import UserForm from './UserForm.tsx';
import { Link } from 'react-router-dom';
import { useGlobalData } from '../useGlobalData';
import styles from './AuthPage.module.css';

const Login: FC = () => {
  const { refreshGlobalData } = useGlobalData();

  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleLogin = (username: string, password: string) => {
    setError(null);
    setPending(true);

    fetch('/api/user/login', {
      method: 'POST',
      headers: {
        'Content-type': 'application/json',
      },
      body: JSON.stringify({
        username,
        password,
      }),
    })
      .then(async (res) => {
        if (res.status === 200) {
          refreshGlobalData();
          return;
        }

        const body = await res.json().catch(() => null);
        setError(body?.error ?? 'Something went wrong. Please try again.');
      })
      .catch(() => {
        setError('Could not reach the server. Please try again.');
      })
      .finally(() => setPending(false));
  };
  return (
    <div className={styles.page}>
      <h1>Welcome to Mishis4x</h1>
      <div className={styles.form}>
        <UserForm
          submit={handleLogin}
          buttonText="Log in"
          pendingText="Logging in…"
          pending={pending}
          error={error}
        />
        <Link to="/request-invite" className={styles.link}>
          Request an invite
        </Link>
      </div>
    </div>
  );
};

export default Login;
