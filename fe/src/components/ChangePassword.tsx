import { FC, FormEvent, useState } from 'react';
import Button from './ui/Button';
import styles from './AuthPage.module.css';
import formStyles from './UserForm.module.css';

// Not built on UserForm - that component is specifically username+password
// shaped for login/signup. This is a one-off current+new password form, not
// worth forcing into a shared shape for.
const ChangePassword: FC = () => {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError(null);
    setSuccess(false);
    setPending(true);

    const form = event.currentTarget;
    const target = form as typeof form & {
      currentPassword: { value: string };
      newPassword: { value: string };
    };

    fetch('/api/user/password', {
      method: 'POST',
      headers: {
        'Content-type': 'application/json',
      },
      body: JSON.stringify({
        currentPassword: target.currentPassword.value,
        newPassword: target.newPassword.value,
      }),
    })
      .then(async (res) => {
        if (res.status === 200) {
          setSuccess(true);
          form.reset();
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
      <h1>Change password</h1>
      <form onSubmit={handleSubmit} className={formStyles.form}>
        {error && (
          <p className={formStyles.error} role="alert">
            {error}
          </p>
        )}
        {success && (
          <p className={formStyles.success} role="status">
            Password updated. Other devices you were logged in on have been
            signed out.
          </p>
        )}
        <div className={formStyles.field}>
          <label htmlFor="currentPassword">Current password</label>
          <input
            type="password"
            name="currentPassword"
            id="currentPassword"
            autoComplete="current-password"
            required
            disabled={pending}
          />
        </div>
        <div className={formStyles.field}>
          <label htmlFor="newPassword">New password</label>
          <input
            type="password"
            name="newPassword"
            id="newPassword"
            autoComplete="new-password"
            required
            minLength={8}
            disabled={pending}
          />
        </div>

        <Button type="submit" disabled={pending}>
          {pending ? 'Updating…' : 'Update password'}
        </Button>
      </form>
    </div>
  );
};

export default ChangePassword;
