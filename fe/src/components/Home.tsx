import CollectionWidget from './CollectionWidget';
import { useGlobalData } from '../useGlobalData';

const Home = () => {
  const { globalData } = useGlobalData();

  return (
    <div className="stack">
      <h1>Welcome to mishis4x</h1>
      <p className="muted">Matchmaking and lobbies are coming soon.</p>
      {/* Hidden entirely for an account without collection access, rather
          than shown and left to 403 on click - see
          handlers.Data.canAccessCollection. */}
      {globalData?.collection_access && <CollectionWidget />}
    </div>
  );
};

export default Home;
