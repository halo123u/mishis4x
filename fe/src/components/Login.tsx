import { useContext, useState, FC } from 'react';
import UserForm from './UserForm.tsx';
import { Link } from 'react-router-dom';
import { GlobalDataContext } from '../GlobalDataContext';
import styles from './AuthPage.module.css';

const Login: FC = () => {
  const context = useContext(GlobalDataContext);

  if (!context) {
    throw new Error('context is undefined');
  }

  const { refreshGlobalData } = context;

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
        <Link to="/sign-up" className={styles.link}>
          Create account
        </Link>
      </div>
    </div>
  );
};

export default Login;
