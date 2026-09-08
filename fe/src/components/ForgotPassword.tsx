import { useState, FormEvent } from 'react';
import { Link } from 'react-router-dom';
import Button from './ui/Button';
import pageStyles from './AuthPage.module.css';
import formStyles from './UserForm.module.css';

// The public "forgot password" entry point - be/handlers/password_reset.go's
// RequestPasswordReset. Submitting always shows the same success message
// regardless of whether the address actually matches an account (the
// server's response is identical either way too - see that handler's own
// doc comment) - this can't be used to find out which emails are
// registered.
const ForgotPassword = () => {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitted, setSubmitted] = useState(false);

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError(null);
    setPending(true);

    const target = event.target as typeof event.target & {
      email_address: { value: string };
    };
    const emailAddress = target.email_address.value;

    fetch('/api/user/password-reset/request', {
      method: 'POST',
      headers: {
        'Content-type': 'application/json',
      },
      body: JSON.stringify({ email_address: emailAddress }),
    })
      .then(async (res) => {
        if (res.status === 200) {
          setSubmitted(true);
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

  if (submitted) {
    return (
      <div className={pageStyles.page}>
        <h1>Reset your password</h1>
        <p className={formStyles.success}>
          If that email matches an account, you'll get a link to reset your
          password. The link expires in an hour.
        </p>
      </div>
    );
  }

  return (
    <div className={pageStyles.page}>
      <h1>Reset your password</h1>
      <div className={pageStyles.form}>
        <form onSubmit={handleSubmit} className={formStyles.form}>
          {error && (
            <p className={formStyles.error} role="alert">
              {error}
            </p>
          )}
          <div className={formStyles.field}>
            <label htmlFor="email_address">Email address</label>
            <input
              type="email"
              name="email_address"
              id="email_address"
              autoComplete="email"
              required
              disabled={pending}
            />
          </div>
          <Button type="submit" disabled={pending}>
            {pending ? 'Sending…' : 'Send reset link'}
          </Button>
        </form>
        <Link to="/login" className={pageStyles.link}>
          Back to log in
        </Link>
      </div>
    </div>
  );
};

export default ForgotPassword;
