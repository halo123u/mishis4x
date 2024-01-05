import { useNavigate } from 'react-router-dom';
import { useContext } from 'react';
// import type { GlobalDataContextT } from '../GlobalDataContext';
import { GlobalDataContext } from '../GlobalDataContext';

const navigation = () => {
  const context = useContext(GlobalDataContext);

  if (!context) {
    throw new Error(
      'GlobalDataContext is not defined. Make sure to wrap this component in GlobalDataProvider'
    );
  }

  const { globalData, setGlobalData } = context;

  const navigate = useNavigate();

  const handleLogout = (e: React.MouseEvent<HTMLButtonElement>) => {
    console.log(e);
    e.preventDefault();
    fetch('/api/logout')
      .then((res) => {
        if (res.status === 200) {
          console.log('logout');
          setGlobalData(null);
          navigate('/login');
        }
      })
      .catch((err) => {
        console.log(err);
      });
  };

  return (
    <header>
      <nav className="row">
        <a href="/">
          <img
            src="https://placehold.co/600x200"
            alt="Company logo"
            id="logo"
          />
        </a>
        <div className="flex-space" />
        {globalData && (
          <ul>
            <li> Hello,{globalData.user.username}</li>
            <li>
              <button className="button secondary" onClick={handleLogout}>
                Logout
              </button>
            </li>
          </ul>
        )}
      </nav>
    </header>
  );
};

export default navigation;
