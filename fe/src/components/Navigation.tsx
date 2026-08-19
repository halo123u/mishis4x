import { useNavigate } from 'react-router-dom';
import { useContext } from 'react';
import { GlobalDataContext } from '../GlobalDataContext';
import Button from './ui/Button';
import styles from './Navigation.module.css';

const Navigation = () => {
  const context = useContext(GlobalDataContext);

  if (!context) {
    throw new Error(
      'GlobalDataContext is not defined. Make sure to wrap this component in GlobalDataProvider',
    );
  }

  const { globalData, setGlobalData } = context;

  const navigate = useNavigate();

  const handleLogout = (e: React.MouseEvent<HTMLButtonElement>) => {
    e.preventDefault();
    // Log out client-side regardless of how the request goes - if the
    // server is unreachable, the user still expects "Log out" to work.
    fetch('/api/logout')
      .catch((err) => {
        console.log(err);
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
