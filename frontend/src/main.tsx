import { Component, StrictMode, type ErrorInfo, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./styles.css";

class AppErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null; componentStack: string }> {
  constructor(props: { children: ReactNode }) {
    super(props);
    this.state = { error: null, componentStack: "" };
  }

  static getDerivedStateFromError(error: Error) {
    return { error, componentStack: "" };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("Melodex startup crash:", error);
    console.error(info.componentStack);
    this.setState({ componentStack: info.componentStack ?? "" });
  }

  render() {
    if (this.state.error) {
      return (
        <div
          style={{
            minHeight: "100vh",
            background: "#050505",
            color: "#f5f5f7",
            padding: 24,
            fontFamily: "Inter, SF Pro Display, SF Pro Text, system-ui, -apple-system, BlinkMacSystemFont, sans-serif",
          }}
        >
          <div
            style={{
              maxWidth: 760,
              margin: "0 auto",
              border: "1px solid rgba(255,255,255,0.12)",
              borderRadius: 20,
              padding: 24,
              background: "rgba(16,16,20,0.96)",
            }}
          >
            <div
              style={{
                fontSize: 14,
                letterSpacing: "0.14em",
                textTransform: "uppercase",
                color: "#b8b8c0",
                marginBottom: 12,
              }}
            >
              Melodex startup error
            </div>
            <div style={{ fontSize: 28, fontWeight: 700, letterSpacing: "-0.04em", marginBottom: 12 }}>
              {this.state.error.message || "The UI crashed during startup."}
            </div>
            <div style={{ color: "#b8b8c0", lineHeight: 1.5, marginBottom: 16 }}>
              This screen replaces the black window and keeps the underlying JavaScript error visible.
            </div>
            {this.state.componentStack ? (
              <pre
                style={{
                  whiteSpace: "pre-wrap",
                  margin: 0,
                  marginBottom: 12,
                  padding: 16,
                  borderRadius: 14,
                  background: "#0a0a0c",
                  border: "1px solid rgba(255,255,255,0.08)",
                  color: "#b8b8c0",
                  overflow: "auto",
                }}
              >
                {this.state.componentStack}
              </pre>
            ) : null}
            <pre
              style={{
                whiteSpace: "pre-wrap",
                margin: 0,
                padding: 16,
                borderRadius: 14,
                background: "#0a0a0c",
                border: "1px solid rgba(255,255,255,0.08)",
                color: "#f5f5f7",
                overflow: "auto",
              }}
            >
              {this.state.error.stack ?? this.state.error.message}
            </pre>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}

const container = document.getElementById("root");

if (!container) {
  throw new Error('Missing root element: expected <div id="root"></div> in index.html');
}

const root = createRoot(container);

root.render(
  <StrictMode>
    <AppErrorBoundary>
      <App />
    </AppErrorBoundary>
  </StrictMode>,
);
