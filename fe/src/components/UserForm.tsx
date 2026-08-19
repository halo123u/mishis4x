import { FC, FormEvent } from 'react';
import Button from './ui/Button';
import styles from './UserForm.module.css';

type UserFormPropsT = {
  submit: (username: string, password: string) => void;
  buttonText: string;
};

const UserForm: FC<UserFormPropsT> = (props) => {
  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const target = event.target as typeof event.target & {
      username: { value: string };
      password: { value: string };
    };
    props.submit(target.username.value, target.password.value);
  };

  return (
    <form onSubmit={handleSubmit} className={styles.form}>
      <div className={styles.field}>
        <label htmlFor="username">Username</label>
        <input
          type="text"
          name="username"
          id="username"
          autoComplete="username"
        />
      </div>
      <div className={styles.field}>
        <label htmlFor="password">Password</label>
        <input
          type="password"
          name="password"
          id="password"
          autoComplete="current-password"
        />
      </div>

      <Button type="submit">{props.buttonText}</Button>
    </form>
  );
};

export default UserForm;
