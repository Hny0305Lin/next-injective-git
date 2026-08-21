import { Buffer } from "buffer";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import { TooltipProvider } from "./components/ui/tooltip";
import { WalletProvider } from "./lib/WalletContext";
import "./index.css";

// isomorphic-git expects a Node-style Buffer global in the browser
(globalThis as Record<string, unknown>).Buffer = Buffer;

// no StrictMode: its double-mounted effects race lightning-fs' file locks
// during pack indexing (AbortError: Lock broken by another request)
ReactDOM.createRoot(document.getElementById("root")!).render(
  <BrowserRouter>
    <WalletProvider>
      <TooltipProvider>
        <App />
      </TooltipProvider>
    </WalletProvider>
  </BrowserRouter>,
);
