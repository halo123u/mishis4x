import { Link } from 'react-router-dom';
import styles from './CollectionWidget.module.css';

// The entry point into the card-collection tracker (GET /api/sets,
// GET /api/sets/{id}/cards) - a clickable tile rather than a nav link since
// Home is currently just a handful of stub sections that'll likely each get
// one of these as the app grows.
const CollectionWidget = () => {
  return (
    <Link to="/collection" className={styles.widget}>
      <span className={styles.eyebrow}>Collection</span>
      <h2 className={styles.title}>Card Manager</h2>
      <p className={styles.description}>
        Browse sets and see which cards you own.
      </p>
    </Link>
  );
};

export default CollectionWidget;
