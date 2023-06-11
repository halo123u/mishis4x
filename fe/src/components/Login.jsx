import { useContext } from 'react'

import { Link } from 'react-router-dom'
import { AuthContext } from '../AuthContext'
import UserForm from './UserForm'

const Login = () => {
  const { login } = useContext(AuthContext)
  return (
    <div>
      <h1>Login</h1>
      <UserForm submit={login} buttonText="login" />
      <div>
        <Link to={`/sign-up`}>Create account</Link>
      </div>
    </div>
  )
}

export default Login
