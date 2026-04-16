import { Component, ErrorInfo, ReactNode } from "react";
import { BrowserRouter, Routes, Route, Link, useLocation } from "react-router-dom";
import { AppShell, Group, Title, NavLink, Badge, ScrollArea } from "@mantine/core";
import { Dashboard } from "./pages/Dashboard";
import { Frames } from "./pages/Frames";
import { History } from "./pages/History";
import { SettingsCamera } from "./pages/SettingsCamera";
import { SettingsImaging } from "./pages/SettingsImaging";
import { SettingsDetection } from "./pages/SettingsDetection";
import { SettingsAlerts } from "./pages/SettingsAlerts";
import { SettingsExport } from "./pages/SettingsExport";
import { SettingsMQTT } from "./pages/SettingsMQTT";
import { SettingsYouTube } from "./pages/SettingsYouTube";
import { SettingsStorage } from "./pages/SettingsStorage";
import { Timelapse } from "./pages/Timelapse";
import { FocusMode } from "./pages/FocusMode";
import { ProcessingTuner } from "./pages/ProcessingTuner";
import { FrameMasking } from "./pages/FrameMasking";
import { OverlayEditor } from "./pages/OverlayEditor";
import { SettingsPublicPage } from "./pages/SettingsPublicPage";
import { SettingsDisk } from "./pages/SettingsDisk";
import { SettingsGPIO } from "./pages/SettingsGPIO";
import { useWebSocket } from "./hooks/useWebSocket";

class ErrorBoundary extends Component<{children: ReactNode}, {hasError: boolean; error: string}> {
  state = { hasError: false, error: "" };
  static getDerivedStateFromError(error: Error) { return { hasError: true, error: error.message }; }
  componentDidCatch(error: Error, info: ErrorInfo) { console.error("UI Error:", error, info); }
  render() {
    if (this.state.hasError) {
      return <div style={{padding: 40, textAlign: "center"}}>
        <h2>Something went wrong</h2>
        <p>{this.state.error}</p>
        <button onClick={() => { this.setState({hasError: false}); window.location.reload(); }}>Reload</button>
      </div>;
    }
    return this.props.children;
  }
}

function AppContent() {
  const { safetyState } = useWebSocket();
  const location = useLocation();
  const stateColor = safetyState?.state === "SAFE" ? "green" : safetyState?.state === "UNSAFE" ? "red" : "yellow";

  return (
    <AppShell header={{ height: 60 }} navbar={{ width: 220, breakpoint: "sm" }} padding="md">
      <AppShell.Header>
        <Group h="100%" px="md" justify="space-between">
          <Title order={3}>NovaSky</Title>
          <Badge size="lg" color={stateColor} variant="filled">{safetyState?.state ?? "UNKNOWN"}</Badge>
        </Group>
      </AppShell.Header>
      <AppShell.Navbar>
        <ScrollArea style={{ flex: 1 }} type="auto">
          <div style={{ padding: "var(--mantine-spacing-xs)" }}>
            <NavLink component={Link} to="/" label="Dashboard" active={location.pathname === "/"} />
            <NavLink component={Link} to="/frames" label="Frames" active={location.pathname === "/frames"} />
            <NavLink component={Link} to="/history" label="History" active={location.pathname === "/history"} />
            <NavLink component={Link} to="/timelapse" label="Timelapse" active={location.pathname === "/timelapse"} />
            <NavLink component={Link} to="/overlay" label="Overlay Editor" active={location.pathname === "/overlay"} />
            <NavLink label="Settings" defaultOpened={location.pathname.startsWith("/settings")}>
              <NavLink component={Link} to="/settings/camera" label="Camera" active={location.pathname === "/settings/camera"} />
              <NavLink component={Link} to="/settings/imaging" label="Imaging" active={location.pathname === "/settings/imaging"} />
              <NavLink component={Link} to="/settings/focus" label="Focus Mode" active={location.pathname === "/settings/focus"} />
              <NavLink component={Link} to="/settings/processing" label="Processing Tuner" active={location.pathname === "/settings/processing"} />
              <NavLink component={Link} to="/settings/masking" label="Frame Masking" active={location.pathname === "/settings/masking"} />
              <NavLink component={Link} to="/settings/detection" label="Detection" active={location.pathname === "/settings/detection"} />
              <NavLink component={Link} to="/settings/alerts" label="Alerts" active={location.pathname === "/settings/alerts"} />
              <NavLink component={Link} to="/settings/export" label="Export" active={location.pathname === "/settings/export"} />
              <NavLink component={Link} to="/settings/mqtt" label="MQTT" active={location.pathname === "/settings/mqtt"} />
              <NavLink component={Link} to="/settings/youtube" label="YouTube" active={location.pathname === "/settings/youtube"} />
              <NavLink component={Link} to="/settings/storage" label="Storage" active={location.pathname === "/settings/storage"} />
              <NavLink component={Link} to="/settings/gpio" label="GPIO / Sensors" active={location.pathname === "/settings/gpio"} />
              <NavLink component={Link} to="/settings/disk" label="Disk Management" active={location.pathname === "/settings/disk"} />
              <NavLink component={Link} to="/settings/public" label="Public Page" active={location.pathname === "/settings/public"} />
            </NavLink>
          </div>
        </ScrollArea>
      </AppShell.Navbar>
      <AppShell.Main>
        <ErrorBoundary>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/frames" element={<Frames />} />
            <Route path="/history" element={<History />} />
            <Route path="/timelapse" element={<Timelapse />} />
            <Route path="/overlay" element={<OverlayEditor />} />
            <Route path="/settings/camera" element={<SettingsCamera />} />
            <Route path="/settings/imaging" element={<SettingsImaging />} />
            <Route path="/settings/focus" element={<FocusMode />} />
            <Route path="/settings/processing" element={<ProcessingTuner />} />
            <Route path="/settings/masking" element={<FrameMasking />} />
            <Route path="/settings/detection" element={<SettingsDetection />} />
            <Route path="/settings/alerts" element={<SettingsAlerts />} />
            <Route path="/settings/export" element={<SettingsExport />} />
            <Route path="/settings/mqtt" element={<SettingsMQTT />} />
            <Route path="/settings/youtube" element={<SettingsYouTube />} />
            <Route path="/settings/storage" element={<SettingsStorage />} />
            <Route path="/settings/gpio" element={<SettingsGPIO />} />
            <Route path="/settings/disk" element={<SettingsDisk />} />
            <Route path="/settings/public" element={<SettingsPublicPage />} />
          </Routes>
        </ErrorBoundary>
      </AppShell.Main>
    </AppShell>
  );
}

export function App() {
  return (
    <BrowserRouter>
      <AppContent />
    </BrowserRouter>
  );
}
