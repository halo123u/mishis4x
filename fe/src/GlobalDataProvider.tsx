import { useState, useEffect, FC, ReactNode } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { GlobalData } from './types';
import { GlobalDataContext } from './globalDataContext';

export const GlobalDataProvider: FC<{ children: ReactNode }> = ({
  children,
}) => {
  const [globalData, setGlobalData] = useState<GlobalData | null>(null);
  const navigate = useNavigate();
  const location = useLocation();

  const refreshGlobalData = () => {
    fetch('/api/data')
      .then((res) => {
        if (res.status === 200) {
          return res.json();
        }

        if (res.status === 401) {
          setGlobalData(null);
          // Don't bounce away from /sign-up or /request-invite (or
          // re-navigate to /login while already there) - an
          // unauthenticated visitor landing directly on any of these auth
          // pages is expected, not an error to redirect out of.
          if (
            location.pathname !== '/login' &&
            location.pathname !== '/sign-up' &&
            location.pathname !== '/request-invite'
          ) {
            navigate('/login');
          }
        }

        if (res.status === 500) {
          console.error('GET /api/data returned a server error');
        }
      })
      .then((res) => {
        // undefined here means the response wasn't a 200 (see above) - the
        // redirect/error handling already happened, nothing further to do.
        if (res) {
          let path = location.pathname;
          if (
            path === '/login' ||
            path === '/sign-up' ||
            path === '/request-invite'
          ) {
            path = '/';
          }

          setGlobalData(res);
          navigate(path);
        }
      })
      .catch((err) => {
        console.error('GET /api/data failed:', err);
      });
  };

  // Intentionally run once on mount only (not on every navigation) - this
  // does its own navigate() internally, so including refreshGlobalData in
  // the deps would refire this effect after every redirect it triggers.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => refreshGlobalData(), []);

  return (
    <GlobalDataContext.Provider
      value={{ globalData, setGlobalData, refreshGlobalData }}
    >
      {children}
    </GlobalDataContext.Provider>
  );
};
