import { useState } from 'react';
import UserForm from './UserForm';
import { useGlobalData } from '../useGlobalData';
import styles from './AuthPage.module.css';

const Signup = () => {
  const { refreshGlobalData } = useGlobalData();

  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const createUser = (username: string, password: string) => {
    setError(null);
    setPending(true);

    fetch('/api/user/create', {
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
        if (res.status === 201) {
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
      <h1>Sign up to play mishis4x!</h1>
      <div className={styles.form}>
        <UserForm
          submit={createUser}
          buttonText="Create account"
          pendingText="Creating account…"
          pending={pending}
          error={error}
          passwordMinLength={8}
          passwordAutoComplete="new-password"
        />
      </div>
    </div>
  );
};

export default Signup;
