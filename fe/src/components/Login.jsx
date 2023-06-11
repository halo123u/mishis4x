import { useContext } from 'react'

import { AuthContext } from '../AuthContext'
import UserForm from './UserForm'

const Login = () => {
  const { login } = useContext(AuthContext)
  return (
    <div>
      <h1>Welcome to Mishis4x</h1>
      <UserForm submit={login} buttonText="login" />
    </div>
  )
}

export default Login
