import { createContext, useContext } from 'react'

// Internal feature routes render inside the existing RelayFor content region.
// They keep the original route guards without mounting a second application shell.
export const NativeFeatureContext = createContext(false)
export const useNativeFeatureIsland = () => useContext(NativeFeatureContext)
