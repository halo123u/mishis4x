import { useContext } from 'react';
import { GlobalDataContext, GlobalDataContextT } from './globalDataContext';

// Every consumer (Navigation, Login, Signup, ...) used to repeat the same
// useContext + "is it undefined" check and throw. Centralized here instead
// of copy-pasted at every call site.
export function useGlobalData(): GlobalDataContextT {
  const context = useContext(GlobalDataContext);
  if (!context) {
    throw new Error('useGlobalData must be used within a GlobalDataProvider');
  }
  return context;
}
