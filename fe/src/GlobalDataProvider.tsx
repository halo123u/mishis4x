import React, {
  createContext,
  useState,
  useEffect,
  FC,
  ReactNode,
} from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { GlobalData } from './types';

export type GlobalDataContextT = {
  globalData: GlobalData | null;
  setGlobalData: React.Dispatch<React.SetStateAction<GlobalData | null>>;
  refreshGlobalData: () => void;
};

export const GlobalDataContext = createContext<GlobalDataContextT | undefined>(
  undefined,
);

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
          // Don't bounce away from /sign-up (or re-navigate to /login while
          // already there) - an unauthenticated visitor landing directly on
          // either auth page is expected, not an error to redirect out of.
          if (
            location.pathname !== '/login' &&
            location.pathname !== '/sign-up'
          ) {
            navigate('/login');
          }
        }

        if (res.status === 500) {
          console.log('server error');
        }
      })
      .then((res) => {
        // when unathorized this is undefined
        // TODO maybe find a better way to do this
        if (res) {
          let path = location.pathname;
          if (path === '/login' || path === '/sign-up') {
            path = '/';
          }

          setGlobalData(res);
          navigate(path);
        }
      })
      .catch((err) => {
        console.log('error');
        console.log(err);
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
