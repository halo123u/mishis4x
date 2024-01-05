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
  undefined
);

export const GlobalDataProvider: FC<{ children: ReactNode }> = ({
  children,
}) => {
  const [globalData, setGlobalData] = useState<GlobalData | null>(null);
  const navigate = useNavigate();
  const location = useLocation();

  useEffect(() => refreshGlobalData(), []);

  const refreshGlobalData = () => {
    fetch('/api/data')
      .then((res) => {
        console.log(res);
        if (res.status === 200) {
          console.log('getting data');
          return res.json();
        }

        if (res.status === 401) {
          console.log('unauthorized');
          setGlobalData(null);
          navigate('/login');
        }

        if (res.status === 500) {
          console.log('server error');
        }
      })
      .then((res) => {
        // when unathorized this is undefined
        // TODO maybe find a better way to do this
        if (!!res) {
          let path = location.pathname;
          if (path === '/login' || path === '/signup') {
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

  return (
    <GlobalDataContext.Provider
      value={{ globalData, setGlobalData, refreshGlobalData }}
    >
      {children}
    </GlobalDataContext.Provider>
  );
};
