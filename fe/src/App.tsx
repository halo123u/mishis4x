import { Route, Routes } from 'react-router-dom';
import { GlobalDataProvider } from './GlobalDataProvider';
import Login from './components/Login.tsx';
import Signup from './components/Signup.tsx';
import Home from './components/Home.tsx';
import ChangePassword from './components/ChangePassword.tsx';
import Layout from './components/Layout';

function App() {
  return (
    <div className="App">
      <GlobalDataProvider>
        <Routes>
          <Route element={<Layout />}>
            <Route path="/" element={<Home />} />
            <Route path="/login" element={<Login />} />
            <Route path="/sign-up" element={<Signup />} />
            <Route path="/account" element={<ChangePassword />} />
          </Route>
        </Routes>
      </GlobalDataProvider>
    </div>
  );
}

export default App;
