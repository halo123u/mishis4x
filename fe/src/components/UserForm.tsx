import { FC, FormEvent } from 'react';

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
    <form onSubmit={handleSubmit} className="stack">
      <div className="stack sm">
        <label htmlFor="username">Username</label>
        <input type="text" name="username" id="username" />
      </div>
      <div className="stack sm">
        <label htmlFor="password">Password</label>
        <input type="password" name="password" id="password" />
      </div>

      <button type="submit">{props.buttonText}</button>
    </form>
  );
};

export default UserForm;
