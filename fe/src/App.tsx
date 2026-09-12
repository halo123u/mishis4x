import { Route, Routes } from 'react-router-dom';
import { GlobalDataProvider } from './GlobalDataProvider';
import Login from './components/Login.tsx';
import Signup from './components/Signup.tsx';
import RequestInvite from './components/RequestInvite.tsx';
import ForgotPassword from './components/ForgotPassword.tsx';
import ResetPassword from './components/ResetPassword.tsx';
import AdminInvites from './components/AdminInvites.tsx';
import Home from './components/Home.tsx';
import ChangePassword from './components/ChangePassword.tsx';
import Layout from './components/Layout';
import CollectionDashboard from './components/CollectionDashboard.tsx';
import AddSet from './components/AddSet.tsx';
import OnboardCards from './components/OnboardCards.tsx';
import SetDetail from './components/SetDetail.tsx';
import DeckInsights from './components/DeckInsights.tsx';
import ModelList from './components/ModelList.tsx';
import ModelViewer from './components/ModelViewer.tsx';
import ModelDisplay from './components/ModelDisplay.tsx';

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
            <Route path="/forgot-password" element={<ForgotPassword />} />
            <Route path="/reset-password" element={<ResetPassword />} />
            <Route path="/admin" element={<AdminInvites />} />
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
            <Route path="/models" element={<ModelList />} />
            {/* Registered before the /models/:charCode wildcard below -
                react-router v6 ranks a static segment higher regardless
                of declaration order, but this reads more obviously
                correct listed first. See ModelDisplay.tsx's own doc
                comment for what this route actually is. */}
            <Route path="/models/display" element={<ModelDisplay />} />
            <Route path="/models/:charCode" element={<ModelViewer />} />
          </Route>
        </Routes>
      </GlobalDataProvider>
    </div>
  );
}

export default App;
