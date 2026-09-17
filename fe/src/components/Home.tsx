import CollectionWidget from './CollectionWidget';
import PriceMoversWidget from './PriceMoversWidget';
import styles from './Home.module.css';

const Home = () => {
  return (
    <div className="stack">
      <h1>Welcome to mishis4x</h1>
      <p className="muted">Matchmaking and lobbies are coming soon.</p>
      <div className={styles.widgets}>
        {/* Open to any authenticated user (this page itself already
            requires being logged in) - the collection tracker's data
            isn't eBay-sourced (catalog/images come from TCG Republic,
            ownership/price-paid is the user's own), so there's no access
            restriction to check here. See handlers.Data.
            CollectionOwnerUserID's doc comment for why that restriction
            exists at all (reserved for a future market-rate feature) and
            why it never applied to this widget. */}
        <CollectionWidget />
        {/* Renders nothing while price_trends_enabled is off - see its
            own doc comment. */}
        <PriceMoversWidget />
      </div>
    </div>
  );
};

export default Home;
