import { useState, useEffect, useCallback, FC, ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { GlobalData } from './types';
import { GlobalDataContext } from './globalDataContext';

// Pages an unauthenticated visitor is expected to land on directly -
// never bounce away from these, and treat them as the fallback landing
// spot once actually authenticated.
const publicOnlyPaths = ['/login', '/sign-up', '/request-invite'];

export const GlobalDataProvider: FC<{ children: ReactNode }> = ({
  children,
}) => {
  const [globalData, setGlobalData] = useState<GlobalData | null>(null);
  const navigate = useNavigate();

  // Reads window.location directly rather than react-router's
  // useLocation() - this needs to stay correct no matter when it's
  // called, including from the popstate listener below, which fires
  // *after* the browser has already changed the URL for a back/forward
  // navigation. A location value captured in this closure at mount time
  // would be stale by then; window.location.pathname is always live.
  const refreshGlobalData = useCallback(() => {
    fetch('/api/data')
      .then((res) => {
        if (res.status === 200) {
          return res.json();
        }

        if (res.status === 401) {
          setGlobalData(null);
          if (!publicOnlyPaths.includes(window.location.pathname)) {
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
          let path = window.location.pathname;
          if (publicOnlyPaths.includes(path)) {
            path = '/';
          }

          setGlobalData(res);
          navigate(path);
        }
      })
      .catch((err) => {
        console.error('GET /api/data failed:', err);
      });
  }, [navigate]);

  // The initial page load's own auth check - runs once on mount.
  useEffect(() => {
    refreshGlobalData();
  }, [refreshGlobalData]);

  // Browser back/forward (popstate) restores a previously-rendered route
  // without remounting this provider or re-running the effect above - so
  // navigating back to a protected page after logging out (or after a
  // session expires) used to just show that page's stale shell instead
  // of bouncing to /login, since nothing re-checked auth for that
  // specific navigation. Re-running the same check specifically on
  // popstate catches this without reintroducing the redirect-loop risk
  // the mount-only effect above exists to avoid: navigate() itself uses
  // pushState/replaceState, neither of which ever fires a popstate event,
  // so this can't end up re-triggering itself.
  useEffect(() => {
    window.addEventListener('popstate', refreshGlobalData);
    return () => window.removeEventListener('popstate', refreshGlobalData);
  }, [refreshGlobalData]);

  return (
    <GlobalDataContext.Provider
      value={{ globalData, setGlobalData, refreshGlobalData }}
    >
      {children}
    </GlobalDataContext.Provider>
  );
};
