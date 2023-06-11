import { createContext, useState } from 'react'
import { useNavigate } from 'react-router-dom'
export const AuthContext = createContext()

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null)
  const navigate = useNavigate()

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
      .then((res) => res.json())
      .then((res) => {
        setUser(res)
        navigate('/lobbies')
      })
      .catch((err) => console.log(err))
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
