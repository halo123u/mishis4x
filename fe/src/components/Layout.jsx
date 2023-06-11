import { Outlet } from 'react-router-dom'
import Navigation from './Navigation'

const Layout = () => {
  return (
    <main>
      <Navigation />
      <Outlet />
    </main>
  )
}

export default Layout
