import { createContext, useState, useEffect } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
export const GlobalDataContext = createContext()

export function GlobalDataProvider({ children }) {
  const [globalData, setGlobalData] = useState(null)
  const navigate = useNavigate()
  const location = useLocation()

  useEffect(() => refreshGlobalData(),[])

  const refreshGlobalData = () => {
    fetch('/api/data')
      .then((res) =>{
        console.log(res)
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
          let path = location.pathname
          if (path === '/login' || path === '/signup'){
            path = '/lobbies'
          }

          setGlobalData(res)
          navigate(path)
        }
      })
      .catch((err) => {
        console.log("error")
      console.log(err)})
    }


  return (
    <GlobalDataContext.Provider value={{ globalData, setGlobalData, refreshGlobalData }}>
      {children}
    </GlobalDataContext.Provider>
  )
}
