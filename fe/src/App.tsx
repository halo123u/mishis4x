import { Route, Routes } from 'react-router-dom';
import { GlobalDataProvider } from './GlobalDataContext';
import Login from './components/Login.tsx';
import Signup from './components/Signup.tsx';
import Home from './components/Home.tsx';
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
          </Route>
        </Routes>
      </GlobalDataProvider>
    </div>
  );
}

export default App;
