import { BrowserRouter, Routes, Route, Link } from "react-router-dom";
import { AppShell, Group, Title, NavLink, Badge } from "@mantine/core";
import { Dashboard } from "./pages/Dashboard";
import { Frames } from "./pages/Frames";
import { History } from "./pages/History";
import { SettingsCamera } from "./pages/SettingsCamera";
import { SettingsImaging } from "./pages/SettingsImaging";
import { SettingsDetection } from "./pages/SettingsDetection";
import { SettingsAlerts } from "./pages/SettingsAlerts";
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
          <NavLink label="Settings" defaultOpened>
            <NavLink component={Link} to="/settings/camera" label="Camera" />
            <NavLink component={Link} to="/settings/imaging" label="Imaging" />
            <NavLink component={Link} to="/settings/detection" label="Detection" />
            <NavLink component={Link} to="/settings/alerts" label="Alerts" />
          </NavLink>
        </AppShell.Navbar>
        <AppShell.Main>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/frames" element={<Frames />} />
            <Route path="/history" element={<History />} />
            <Route path="/settings/camera" element={<SettingsCamera />} />
            <Route path="/settings/imaging" element={<SettingsImaging />} />
            <Route path="/settings/detection" element={<SettingsDetection />} />
            <Route path="/settings/alerts" element={<SettingsAlerts />} />
          </Routes>
        </AppShell.Main>
      </AppShell>
    </BrowserRouter>
  );
}
