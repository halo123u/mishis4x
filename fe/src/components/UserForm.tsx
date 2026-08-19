import { FC, FormEvent } from 'react';
import Button from './ui/Button';
import styles from './UserForm.module.css';

type UserFormPropsT = {
  submit: (username: string, password: string) => void;
  buttonText: string;
  pendingText?: string;
  pending?: boolean;
  error?: string | null;
  // Only set by Signup - login must never reject a real, existing account
  // for being "too short", even one seeded before these rules existed.
  passwordMinLength?: number;
  passwordAutoComplete?: 'current-password' | 'new-password';
};

const UserForm: FC<UserFormPropsT> = ({
  submit,
  buttonText,
  pendingText,
  pending = false,
  error,
  passwordMinLength,
  passwordAutoComplete = 'current-password',
}) => {
  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const target = event.target as typeof event.target & {
      username: { value: string };
      password: { value: string };
    };
    submit(target.username.value, target.password.value);
  };

  return (
    <form onSubmit={handleSubmit} className={styles.form}>
      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}
      <div className={styles.field}>
        <label htmlFor="username">Username</label>
        <input
          type="text"
          name="username"
          id="username"
          autoComplete="username"
          required
          disabled={pending}
        />
      </div>
      <div className={styles.field}>
        <label htmlFor="password">Password</label>
        <input
          type="password"
          name="password"
          id="password"
          autoComplete={passwordAutoComplete}
          required
          minLength={passwordMinLength}
          disabled={pending}
        />
      </div>

      <Button type="submit" disabled={pending}>
        {pending ? (pendingText ?? 'Please wait…') : buttonText}
      </Button>
    </form>
  );
};

export default UserForm;
