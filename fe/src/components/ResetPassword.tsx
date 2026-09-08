import { FormEvent, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import Button from './ui/Button';
import { useGlobalData } from '../useGlobalData';
import pageStyles from './AuthPage.module.css';
import formStyles from './UserForm.module.css';

// The second half of "forgot password", reached via the link
// RequestPasswordReset emails out - the token travels as a URL query
// param (?token=..., same convention as Signup's ?invite=...) and rides
// along in the confirm request. Read once at mount, not on every render -
// the link is what got someone here in the first place.
const ResetPassword = () => {
  const { refreshGlobalData } = useGlobalData();
  const [searchParams] = useSearchParams();
  const token = searchParams.get('token');

  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError(null);
    setPending(true);

    const target = event.target as typeof event.target & {
      newPassword: { value: string };
    };

    fetch('/api/user/password-reset/confirm', {
      method: 'POST',
      headers: {
        'Content-type': 'application/json',
      },
      body: JSON.stringify({
        token,
        new_password: target.newPassword.value,
      }),
    })
      .then(async (res) => {
        if (res.status === 200) {
          // The server already logged this session in on success (same
          // auto-login convenience as signup) - just pick up the new
          // session, GlobalDataProvider's own redirect takes it from here.
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

  // No point rendering a form guaranteed to fail server-side - same
  // "missing entirely, not just invalid" distinction Signup's own
  // !inviteCode check makes for its query param. A token that's present
  // but invalid/expired still submits and lets the server give the real
  // reason.
  if (!token) {
    return (
      <div className={pageStyles.page}>
        <h1>Reset your password</h1>
        <p>
          This link is missing its reset code. If you followed a link from an
          email, please use that link directly.
        </p>
        <Link to="/forgot-password" className={pageStyles.link}>
          Request a new reset link
        </Link>
      </div>
    );
  }

  return (
    <div className={pageStyles.page}>
      <h1>Choose a new password</h1>
      <div className={pageStyles.form}>
        <form onSubmit={handleSubmit} className={formStyles.form}>
          {error && (
            <p className={formStyles.error} role="alert">
              {error}
            </p>
          )}
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
            {pending ? 'Resetting…' : 'Reset password'}
          </Button>
        </form>
      </div>
    </div>
  );
};

export default ResetPassword;
