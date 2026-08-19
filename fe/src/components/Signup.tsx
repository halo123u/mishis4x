import { useContext } from 'react';
import UserForm from './UserForm';
import { GlobalDataContext } from '../GlobalDataContext';
import styles from './AuthPage.module.css';

const Signup = () => {
  const context = useContext(GlobalDataContext);

  if (!context) {
    throw new Error('context is undefined');
  }

  const { refreshGlobalData } = context;
  const createUser = (username: string, password: string) => {
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
      .then((res) => {
        if (res.status === 201) {
          refreshGlobalData();
        }

        if (res.status === 401) {
          console.log('unauthorized');
          // TODO show error message
        }

        // Todo: add a catch all errors
      })
      .catch((err) => console.log(err));
  };
  return (
    <div className={styles.page}>
      <h1>Sign up to play mishis4x!</h1>
      <div className={styles.form}>
        <UserForm submit={createUser} buttonText="Create account" />
      </div>
    </div>
  );
};

export default Signup;
