import { Route, Routes } from 'react-router-dom';
import { GlobalDataProvider } from './GlobalDataProvider';
import Login from './components/Login.tsx';
import Signup from './components/Signup.tsx';
import RequestInvite from './components/RequestInvite.tsx';
import Home from './components/Home.tsx';
import ChangePassword from './components/ChangePassword.tsx';
import Layout from './components/Layout';
import CollectionDashboard from './components/CollectionDashboard.tsx';
import AddSet from './components/AddSet.tsx';
import OnboardCards from './components/OnboardCards.tsx';
import SetDetail from './components/SetDetail.tsx';
import DeckInsights from './components/DeckInsights.tsx';

function App() {
  return (
    <div className="App">
      <GlobalDataProvider>
        <Routes>
          <Route element={<Layout />}>
            <Route path="/" element={<Home />} />
            <Route path="/login" element={<Login />} />
            <Route path="/sign-up" element={<Signup />} />
            <Route path="/request-invite" element={<RequestInvite />} />
            <Route path="/account" element={<ChangePassword />} />
            <Route path="/collection" element={<CollectionDashboard />} />
            <Route path="/collection/add" element={<AddSet />} />
            <Route
              path="/collection/:setID/onboard"
              element={<OnboardCards />}
            />
            <Route
              path="/collection/:setID/insights"
              element={<DeckInsights />}
            />
            <Route path="/collection/:setID" element={<SetDetail />} />
          </Route>
        </Routes>
      </GlobalDataProvider>
    </div>
  );
}

export default App;
