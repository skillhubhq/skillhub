import React from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./style.css";

declare const __NATIVE_SMOKE__: boolean;

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);

if (__NATIVE_SMOKE__) {
  void import("./nativeSmoke").then(({ runNativeSmoke }) => runNativeSmoke());
}
