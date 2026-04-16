import { Component, ErrorInfo, ReactNode, Suspense, lazy } from "react";
import { BrowserRouter, Routes, Route, Link, useLocation } from "react-router-dom";
import { AppShell, Group, Title, NavLink, Badge, ScrollArea, Burger, Loader, Center } from "@mantine/core";
import { useDisclosure } from "@mantine/hooks";
import { Dashboard } from "./pages/Dashboard";
import { useWebSocket } from "./hooks/useWebSocket";

const Frames = lazy(() => import("./pages/Frames").then(m => ({ default: m.Frames })));
const History = lazy(() => import("./pages/History").then(m => ({ default: m.History })));
const Timelapse = lazy(() => import("./pages/Timelapse").then(m => ({ default: m.Timelapse })));
const OverlayEditor = lazy(() => import("./pages/OverlayEditor").then(m => ({ default: m.OverlayEditor })));
const SettingsCamera = lazy(() => import("./pages/SettingsCamera").then(m => ({ default: m.SettingsCamera })));
const SettingsImaging = lazy(() => import("./pages/SettingsImaging").then(m => ({ default: m.SettingsImaging })));
const SettingsDetection = lazy(() => import("./pages/SettingsDetection").then(m => ({ default: m.SettingsDetection })));
const SettingsAlerts = lazy(() => import("./pages/SettingsAlerts").then(m => ({ default: m.SettingsAlerts })));
const SettingsExport = lazy(() => import("./pages/SettingsExport").then(m => ({ default: m.SettingsExport })));
const SettingsMQTT = lazy(() => import("./pages/SettingsMQTT").then(m => ({ default: m.SettingsMQTT })));
const SettingsYouTube = lazy(() => import("./pages/SettingsYouTube").then(m => ({ default: m.SettingsYouTube })));
const SettingsStorage = lazy(() => import("./pages/SettingsStorage").then(m => ({ default: m.SettingsStorage })));
const FocusMode = lazy(() => import("./pages/FocusMode").then(m => ({ default: m.FocusMode })));
const ProcessingTuner = lazy(() => import("./pages/ProcessingTuner").then(m => ({ default: m.ProcessingTuner })));
const FrameMasking = lazy(() => import("./pages/FrameMasking").then(m => ({ default: m.FrameMasking })));
const SettingsPublicPage = lazy(() => import("./pages/SettingsPublicPage").then(m => ({ default: m.SettingsPublicPage })));
const SettingsDisk = lazy(() => import("./pages/SettingsDisk").then(m => ({ default: m.SettingsDisk })));
const SettingsGPIO = lazy(() => import("./pages/SettingsGPIO").then(m => ({ default: m.SettingsGPIO })));

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
  const [opened, { toggle }] = useDisclosure();
  const location = useLocation();
  const stateColor = safetyState?.state === "SAFE" ? "green" : safetyState?.state === "UNSAFE" ? "red" : "yellow";

  const closeMobileNav = () => { if (window.innerWidth < 768) toggle(); };

  return (
    <AppShell header={{ height: 60 }} navbar={{ width: 220, breakpoint: "sm", collapsed: { mobile: !opened } }} padding="md">
      <AppShell.Header>
        <Group h="100%" px="md" justify="space-between">
          <Group>
            <Burger opened={opened} onClick={toggle} hiddenFrom="sm" size="sm" />
            <Title order={3}>NovaSky</Title>
          </Group>
          <Badge size="lg" color={stateColor} variant="filled">{safetyState?.state ?? "UNKNOWN"}</Badge>
        </Group>
      </AppShell.Header>
      <AppShell.Navbar>
        <ScrollArea style={{ flex: 1 }} type="auto">
          <div style={{ padding: "var(--mantine-spacing-xs)" }}>
            <NavLink component={Link} to="/" label="Dashboard" active={location.pathname === "/"} onClick={closeMobileNav} />
            <NavLink component={Link} to="/frames" label="Frames" active={location.pathname === "/frames"} onClick={closeMobileNav} />
            <NavLink component={Link} to="/history" label="History" active={location.pathname === "/history"} onClick={closeMobileNav} />
            <NavLink component={Link} to="/timelapse" label="Timelapse" active={location.pathname === "/timelapse"} onClick={closeMobileNav} />
            <NavLink component={Link} to="/overlay" label="Overlay Editor" active={location.pathname === "/overlay"} onClick={closeMobileNav} />
            <NavLink label="Settings" defaultOpened={location.pathname.startsWith("/settings")}>
              <NavLink component={Link} to="/settings/camera" label="Camera" active={location.pathname === "/settings/camera"} onClick={closeMobileNav} />
              <NavLink component={Link} to="/settings/imaging" label="Imaging" active={location.pathname === "/settings/imaging"} onClick={closeMobileNav} />
              <NavLink component={Link} to="/settings/focus" label="Focus Mode" active={location.pathname === "/settings/focus"} onClick={closeMobileNav} />
              <NavLink component={Link} to="/settings/processing" label="Processing Tuner" active={location.pathname === "/settings/processing"} onClick={closeMobileNav} />
              <NavLink component={Link} to="/settings/masking" label="Frame Masking" active={location.pathname === "/settings/masking"} onClick={closeMobileNav} />
              <NavLink component={Link} to="/settings/detection" label="Detection" active={location.pathname === "/settings/detection"} onClick={closeMobileNav} />
              <NavLink component={Link} to="/settings/alerts" label="Alerts" active={location.pathname === "/settings/alerts"} onClick={closeMobileNav} />
              <NavLink component={Link} to="/settings/export" label="Export" active={location.pathname === "/settings/export"} onClick={closeMobileNav} />
              <NavLink component={Link} to="/settings/mqtt" label="MQTT" active={location.pathname === "/settings/mqtt"} onClick={closeMobileNav} />
              <NavLink component={Link} to="/settings/youtube" label="YouTube" active={location.pathname === "/settings/youtube"} onClick={closeMobileNav} />
              <NavLink component={Link} to="/settings/storage" label="Storage" active={location.pathname === "/settings/storage"} onClick={closeMobileNav} />
              <NavLink component={Link} to="/settings/gpio" label="GPIO / Sensors" active={location.pathname === "/settings/gpio"} onClick={closeMobileNav} />
              <NavLink component={Link} to="/settings/disk" label="Disk Management" active={location.pathname === "/settings/disk"} onClick={closeMobileNav} />
              <NavLink component={Link} to="/settings/public" label="Public Page" active={location.pathname === "/settings/public"} onClick={closeMobileNav} />
            </NavLink>
          </div>
        </ScrollArea>
      </AppShell.Navbar>
      <AppShell.Main>
        <ErrorBoundary>
          <Suspense fallback={<Center h={200}><Loader /></Center>}>
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
          </Suspense>
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
