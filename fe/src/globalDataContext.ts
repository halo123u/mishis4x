import React, { createContext } from 'react';
import { GlobalData } from './types';

export type GlobalDataContextT = {
  globalData: GlobalData | null;
  setGlobalData: React.Dispatch<React.SetStateAction<GlobalData | null>>;
  refreshGlobalData: () => void;
};

export const GlobalDataContext = createContext<GlobalDataContextT | undefined>(
  undefined,
);
