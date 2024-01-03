import { Route, Routes, Outlet } from 'react-router-dom'
import { GlobalDataProvider } from './GlobalDataContext'
import Login from './components/Login'
import Signup from './components/Signup'
import Lobbies from './components/Lobbies'
import CreateLobby from './components/CreateLobby'
import Home from './components/Home'
import Layout from './components/Layout'

function App() {
  return (
    <div className="App">
      <GlobalDataProvider>
        <Routes>
          <Route element={<Layout />}>
            <Route
              exact
              path="/"
              element={
                <Home />
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
      </GlobalDataProvider>
    </div>
  )
}

export default App
