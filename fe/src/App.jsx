import { useContext } from "react";
import { Route, Routes, Outlet, Navigate, useLocation } from "react-router-dom";
import { AuthProvider, AuthContext } from "./AuthContext";
import Login from "./components/Login";
import Signup from "./components/Signup";
import Lobbies from "./components/Lobbies";

function Layout() {
  return (
    <div>
      <nav>This will be a nav</nav>
      <Outlet />
    </div>
  );
}

function RequireAuth({ children }) {
  const { user } = useContext(AuthContext);
  const location = useLocation();

  if (!user) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }

  return children;
}

function App() {
  return (
    <div className="App">
      <AuthProvider>
        <Routes>
          <Route element={<Layout />}>
            <Route exact path="/" element={<div>Home</div>} />
            <Route exact path="/login" element={<Login />} />
            <Route exact path="/sign-up" element={<Signup />} />
            <Route
              path="/protected"
              element={
                <RequireAuth>
                  <Lobbies />
                </RequireAuth>
              }
            />
          </Route>
        </Routes>
      </AuthProvider>
    </div>
  );
}

export default App;
