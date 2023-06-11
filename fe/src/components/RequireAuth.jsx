import { useContext } from 'react'
import { useLocation, Navigate } from 'react-router-dom'
import { AuthContext } from '../AuthContext'

function RequireAuth({ children }) {
  const { user } = useContext(AuthContext)
  const location = useLocation()

  if (!user) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  return children
}

export default RequireAuth
