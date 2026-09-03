import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import UserForm from './UserForm';
import { useGlobalData } from '../useGlobalData';
import styles from './AuthPage.module.css';

const Signup = () => {
  const { refreshGlobalData } = useGlobalData();
  // Signup is invite-only (see be/handlers/users.go's UserCreate) - the
  // token travels as a URL query param (?invite=..., see be/cmd/invite.go
  // for how one gets minted) and rides along in the create-account
  // request. Read once at mount, not on every render - the invite link
  // is what got someone here in the first place, no reason to re-read it.
  const [searchParams] = useSearchParams();
  const inviteToken = searchParams.get('invite');

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
        invite_token: inviteToken,
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
  // No point rendering a form that's guaranteed to fail server-side - if
  // someone landed here without an invite link at all (as opposed to one
  // that's invalid/already-used, which still submits and lets the server
  // give the real reason), say so plainly instead.
  if (!inviteToken) {
    return (
      <div className={styles.page}>
        <h1>Sign up to play mishis4x!</h1>
        <p>
          This app is invite-only right now. If someone shared a sign-up link
          with you, please use that link directly.
        </p>
      </div>
    );
  }

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
