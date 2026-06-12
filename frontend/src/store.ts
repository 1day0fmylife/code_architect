import { create } from "zustand";

const TOKEN_STORAGE_KEY = "hermes.webAuthToken.v1";
const API_URL_STORAGE_KEY = "hermes.apiUrl.v1";

type AppState = {
  apiUrl: string;
  token: string;
  isAuthenticated: boolean;
  setApiUrl: (apiUrl: string) => void;
  setToken: (token: string) => void;
  logout: () => void;
};

const defaultApiUrl = import.meta.env.VITE_HERMES_API_URL ?? "http://localhost:8088";

function readStorage(key: string, fallback: string) {
  if (typeof window === "undefined") {
    return fallback;
  }
  return window.localStorage.getItem(key) ?? fallback;
}

const useStore = create<AppState>((set) => ({
  apiUrl: readStorage(API_URL_STORAGE_KEY, defaultApiUrl),
  token: readStorage(TOKEN_STORAGE_KEY, ""),
  isAuthenticated: readStorage(TOKEN_STORAGE_KEY, "") !== "",
  setApiUrl: (apiUrl) => {
    const normalized = apiUrl.trim().replace(/\/+$/, "");
    window.localStorage.setItem(API_URL_STORAGE_KEY, normalized);
    set({ apiUrl: normalized });
  },
  setToken: (token) => {
    const normalized = token.trim();
    window.localStorage.setItem(TOKEN_STORAGE_KEY, normalized);
    set({ token: normalized, isAuthenticated: normalized !== "" });
  },
  logout: () => {
    window.localStorage.removeItem(TOKEN_STORAGE_KEY);
    set({ token: "", isAuthenticated: false });
  },
}));

export default useStore;
