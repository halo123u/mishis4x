import { createContext, useState, useEffect } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
export const GlobalDataContext = createContext()

export function GlobalDataProvider({ children }) {
  const [globalData, setGlobalData] = useState(null)
  const navigate = useNavigate()
  const location = useLocation()

  useEffect(() => {
    //TODO maybe a better way to do this
    if (location.pathname !== '/login' && location.pathname !== '/sign-up') {
    fetch('/api/user/data')
      .then((res) =>{
        if(res.status === 200){
          console.log("getting data")
          return res.json()
        }

        if (res.status === 401) {
          console.log('unauthorized')
          setGlobalData(null)
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
          setGlobalData(res)
          navigate(location.pathname )
        }
      })
      .catch((err) => {
        console.log("error")
      console.log(err)})
    }
  }, [])


  function logout() {
    console.log('logout')
    setUser(null)
  }

  return (
    <GlobalDataContext.Provider value={{ globalData, setGlobalData }}>
      {children}
    </GlobalDataContext.Provider>
  )
}
