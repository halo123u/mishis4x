import { Route, Routes, Outlet } from 'react-router-dom'
import { AuthProvider } from './AuthContext'
import Login from './components/Login'
import Signup from './components/Signup'
import Lobbies from './components/Lobbies'
import CreateLobby from './components/CreateLobby'
import RequireAuth from './components/RequireAuth'
import Layout from './components/Layout'

function App() {
  return (
    <div className="App">
      <AuthProvider>
        <Routes>
          <Route element={<Layout />}>
            <Route
              exact
              path="/"
              element={
                  <div>Home</div>
              }
            />
            <Route exact path="/login" element={<Login />} />
            <Route exact path="/sign-up" element={<Signup />} />
            <Route
              path="/lobbies"
              element={
                  <Lobbies />
              }
            />
            <Route
              path="/lobbies/create"
              element={
                  <CreateLobby />
              }
            />
          </Route>
        </Routes>
      </AuthProvider>
    </div>
  )
}

export default App
