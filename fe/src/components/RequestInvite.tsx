import { useState, FormEvent } from 'react';
import { Link } from 'react-router-dom';
import Button from './ui/Button';
import pageStyles from './AuthPage.module.css';
import formStyles from './UserForm.module.css';

// The public "request an invite" form - signup is invite-only (see
// be/handlers/users.go's UserCreate and be/handlers/invites.go's
// RequestInvite), and this is how someone with no invite link gets into
// the queue for one. Submitting only ever creates a 'requested' row -
// nothing here hands out a usable code. The app owner reviews requests
// via `be invite-list` and decides via invite-approve/invite-deny; an
// approval is what actually emails a sign-up link out.
const RequestInvite = () => {
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

    fetch('/api/invites/request', {
      method: 'POST',
      headers: {
        'Content-type': 'application/json',
      },
      body: JSON.stringify({ email_address: emailAddress }),
    })
      .then(async (res) => {
        if (res.status === 201) {
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
        <h1>Request an invite</h1>
        <p className={formStyles.success}>
          Thanks! If your request is approved, you'll get an email with a
          sign-up link.
        </p>
      </div>
    );
  }

  return (
    <div className={pageStyles.page}>
      <h1>Request an invite</h1>
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
            {pending ? 'Submitting…' : 'Request invite'}
          </Button>
        </form>
        <Link to="/login" className={pageStyles.link}>
          Already have an account?
        </Link>
      </div>
    </div>
  );
};

export default RequestInvite;
