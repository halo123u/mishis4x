import { useContext } from 'react'
import { useNavigate } from 'react-router-dom'
import UserForm from './UserForm'
import { GlobalDataContext } from '../GlobalDataContext'

const Signup = () => {
  const { refreshGlobalData } = useContext(GlobalDataContext)
  const navigate = useNavigate()
  const createUser = (username, password) => {
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

      if(res.status === 200){
        refreshGlobalData()
        navigate('/lobbies')
      }

      if (res.status === 401) {
        console.log('unauthorized')
        // TODO show error message
      }

      // Todo: add a catch all errors 
    
      
    }).catch((err) => console.log(err))
  }
  return (
    <div>
      <h1>Sign up to play mishis4x!</h1>
      <UserForm submit={createUser} buttonText="create account" />
    </div>
  )
}

export default Signup
