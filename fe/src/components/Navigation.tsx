import { Link, useNavigate } from 'react-router-dom';
import { useGlobalData } from '../useGlobalData';
import Button from './ui/Button';
import styles from './Navigation.module.css';

const Navigation = () => {
  const { globalData, setGlobalData } = useGlobalData();

  const navigate = useNavigate();

  const handleLogout = (e: React.MouseEvent<HTMLButtonElement>) => {
    e.preventDefault();
    // Log out client-side regardless of how the request goes - if the
    // server is unreachable, the user still expects "Log out" to work.
    fetch('/api/logout')
      .catch((err) => {
        console.error('POST /api/logout failed:', err);
      })
      .finally(() => {
        setGlobalData(null);
        navigate('/login');
      });
  };

  return (
    <header>
      <nav className={`row ${styles.nav}`}>
        <a href="/" className={styles.brand}>
          mishis<span className={styles.accent}>4x</span>
        </a>
        <div className="spacer" />
        {globalData && (
          <ul className={styles.userMenu}>
            <li className={styles.greeting}>
              Hello, <strong>{globalData.user.username}</strong>
            </li>
            {globalData.user.is_admin && (
              <li>
                <Link to="/admin">Admin</Link>
              </li>
            )}
            <li>
              <Link to="/account">Account</Link>
            </li>
            <li>
              <Button variant="danger" onClick={handleLogout}>
                Log out
              </Button>
            </li>
          </ul>
        )}
      </nav>
    </header>
  );
};

export default Navigation;
