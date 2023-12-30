import { createContext, useState, useEffect } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
export const AuthContext = createContext()

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null)
  const navigate = useNavigate()
  const location = useLocation()

  useEffect(() => {
    //TODO maybe a better way to do this
    if (location.pathname !== '/login' && location.pathname !== '/sign-up') {
    fetch('/api/user/data')
      .then((res) =>{
        if(res.status === 200){
          return res.json()
        }

        if (res.status === 401) {
          console.log('unauthorized')
          navigate('/login')
          
        }

        if (res.status === 500) {
          console.log('server error')
        } 
      }
      )
      .then((res) => {
        // when unathorized this is undefined
        // TODO maybe find a better way to do this
        if (!!res){
          setUser(res)
          navigate('/lobbies')
        }
      })
      .catch((err) => {
        console.log("error")
      console.log(err)})
    }
  }, [])


  function login(username, password) {
    console.log('login')
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
        console.log(res)
        if(res.status === 200){
          navigate('/lobbies')
        }

        if (res.status === 401) {
          console.log('unauthorized')
        }

        if (res.status === 500) {
          console.log('server error')
        }
        
      })
      .catch((err) => console.log(err))
  }

  function logout() {
    console.log('logout')
    setUser(null)
  }

  function updateUser(user) {
    setUser(user)
  }
  function logout() {
    console.log('logout')
    setUser(null)
  }

  return (
    <AuthContext.Provider value={{ user, login, logout, updateUser }}>
      {children}
    </AuthContext.Provider>
  )
}
