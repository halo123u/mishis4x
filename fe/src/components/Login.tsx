import { useContext, FC } from 'react';
import UserForm from './UserForm.tsx';
import { Link } from 'react-router-dom';
import { GlobalDataContext } from '../GlobalDataContext';

const Login: FC = () => {
  const context = useContext(GlobalDataContext);

  if (!context) {
    throw new Error('context is undefined');
  }

  const { refreshGlobalData } = context;

  const handleLogin = (username: string, password: string) => {
    fetch('/api/user/login', {
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
        console.log(res);
        if (res.status === 200) {
          refreshGlobalData();
        }

        if (res.status === 401) {
          console.log('unauthorized');
        }

        if (res.status === 500) {
          console.log('server error');
        }
      })
      .catch((err) => console.log(err));
  };
  return (
    <div>
      <h1>Welcome to Mishis4x</h1>
      <div className="stack">
        <UserForm submit={handleLogin} buttonText="login" />

        <Link to={`/sign-up`}>Create account</Link>
      </div>
    </div>
  );
};

export default Login;
