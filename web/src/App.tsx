import { BrowserRouter, Routes, Route, Link } from "react-router-dom";
import { AppShell, Group, Title, NavLink, Badge } from "@mantine/core";
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
import { SettingsPublicPage } from "./pages/SettingsPublicPage";
import { SettingsDisk } from "./pages/SettingsDisk";
import { SettingsGPIO } from "./pages/SettingsGPIO";
import { useWebSocket } from "./hooks/useWebSocket";

export function App() {
  const { safetyState } = useWebSocket();
  const stateColor = safetyState?.state === "SAFE" ? "green" : safetyState?.state === "UNSAFE" ? "red" : "yellow";

  return (
    <BrowserRouter>
      <AppShell header={{ height: 60 }} navbar={{ width: 200, breakpoint: "sm" }} padding="md">
        <AppShell.Header>
          <Group h="100%" px="md" justify="space-between">
            <Title order={3}>NovaSky</Title>
            <Badge size="lg" color={stateColor} variant="filled">{safetyState?.state ?? "UNKNOWN"}</Badge>
          </Group>
        </AppShell.Header>
        <AppShell.Navbar p="xs">
          <NavLink component={Link} to="/" label="Dashboard" />
          <NavLink component={Link} to="/frames" label="Frames" />
          <NavLink component={Link} to="/history" label="History" />
          <NavLink component={Link} to="/timelapse" label="Timelapse" />
          <NavLink component={Link} to="/focus" label="Focus Mode" />
          <NavLink component={Link} to="/processing" label="Processing Tuner" />
          <NavLink component={Link} to="/masking" label="Frame Masking" />
          <NavLink label="Settings" defaultOpened>
            <NavLink component={Link} to="/settings/camera" label="Camera" />
            <NavLink component={Link} to="/settings/imaging" label="Imaging" />
            <NavLink component={Link} to="/settings/detection" label="Detection" />
            <NavLink component={Link} to="/settings/alerts" label="Alerts" />
            <NavLink component={Link} to="/settings/export" label="Export" />
            <NavLink component={Link} to="/settings/mqtt" label="MQTT" />
            <NavLink component={Link} to="/settings/youtube" label="YouTube" />
            <NavLink component={Link} to="/settings/storage" label="Storage" />
            <NavLink component={Link} to="/settings/gpio" label="GPIO / Sensors" />
            <NavLink component={Link} to="/settings/disk" label="Disk Management" />
            <NavLink component={Link} to="/settings/public" label="Public Page" />
          </NavLink>
        </AppShell.Navbar>
        <AppShell.Main>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/frames" element={<Frames />} />
            <Route path="/history" element={<History />} />
            <Route path="/timelapse" element={<Timelapse />} />
            <Route path="/focus" element={<FocusMode />} />
            <Route path="/settings/camera" element={<SettingsCamera />} />
            <Route path="/settings/imaging" element={<SettingsImaging />} />
            <Route path="/settings/detection" element={<SettingsDetection />} />
            <Route path="/settings/alerts" element={<SettingsAlerts />} />
            <Route path="/settings/export" element={<SettingsExport />} />
            <Route path="/settings/mqtt" element={<SettingsMQTT />} />
            <Route path="/settings/youtube" element={<SettingsYouTube />} />
            <Route path="/settings/storage" element={<SettingsStorage />} />
            <Route path="/processing" element={<ProcessingTuner />} />
            <Route path="/masking" element={<FrameMasking />} />
            <Route path="/settings/gpio" element={<SettingsGPIO />} />
            <Route path="/settings/disk" element={<SettingsDisk />} />
            <Route path="/settings/public" element={<SettingsPublicPage />} />
          </Routes>
        </AppShell.Main>
      </AppShell>
    </BrowserRouter>
  );
}
