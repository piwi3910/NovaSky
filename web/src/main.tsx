import React from "react";
import ReactDOM from "react-dom/client";
import { MantineProvider } from "@mantine/core";
import "@mantine/core/styles.css";
import "../.ui-design/tokens/tokens.css";
import { novaskyTheme } from "../.ui-design/tokens/theme";
import { WebSocketProvider } from "./hooks/useWebSocket";
import { App } from "./App";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <MantineProvider theme={novaskyTheme} defaultColorScheme="dark">
      <WebSocketProvider>
        <App />
      </WebSocketProvider>
    </MantineProvider>
  </React.StrictMode>,
);
