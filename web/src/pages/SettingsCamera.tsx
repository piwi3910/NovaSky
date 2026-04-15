import { useState, useEffect, useRef } from "react";
import { Stack, Title, Select, NumberInput, Switch, Button, Group, Card, Text, Loader } from "@mantine/core";
import { useApi } from "../hooks/useApi";

export function SettingsCamera() {
  const { data: configData } = useApi<Record<string, any>>("/api/config");
  const [driver, setDriver] = useState(""); const [device, setDevice] = useState("");
  const [latitude, setLatitude] = useState(0); const [longitude, setLongitude] = useState(0); const [elevation, setElevation] = useState(0);
  const [gpsdEnabled, setGpsdEnabled] = useState(false);
  const [devices, setDevices] = useState<string[]>([]); const [saving, setSaving] = useState(false);
  const [loadingGps, setLoadingGps] = useState(false);
  const initialized = useRef(false);

  useEffect(() => {
    if (configData && !initialized.current) {
      initialized.current = true;
      setDriver((configData["camera.driver"] as string) ?? "indi_asi_ccd");
      const savedDevice = (configData["camera.device"] as string) ?? "";
      setDevice(savedDevice);
      const loc = (configData["location"] ?? {}) as any;
      setLatitude(loc.latitude ?? 0); setLongitude(loc.longitude ?? 0); setElevation(loc.elevation ?? 0);
      const gpsd = (configData["location.gpsd"] ?? {}) as any;
      setGpsdEnabled(gpsd.enabled ?? false);
      fetch("/api/devices").then(r => r.json()).then(d => {
        const list: string[] = d.devices ?? [];
        if (savedDevice && !list.includes(savedDevice)) list.unshift(savedDevice);
        setDevices(list);
      }).catch(() => { if (savedDevice) setDevices([savedDevice]); });
    }
  }, [configData]);

  async function handleGpsdToggle(enabled: boolean) {
    setGpsdEnabled(enabled);
    await fetch("/api/config/location.gpsd", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ value: { enabled } }) });
    if (enabled) {
      setLoadingGps(true);
      try {
        const res = await fetch("/api/gpsd"); const data = await res.json();
        if (data.available) { setLatitude(data.latitude); setLongitude(data.longitude); setElevation(data.elevation);
          await fetch("/api/config/location", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ value: { latitude: data.latitude, longitude: data.longitude, elevation: data.elevation } }) });
        }
      } catch {} setLoadingGps(false);
    }
  }

  async function save() {
    setSaving(true);
    await Promise.all([
      fetch("/api/config/camera.driver", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ value: driver }) }),
      fetch("/api/config/camera.device", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ value: device }) }),
      fetch("/api/config/location", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ value: { latitude, longitude, elevation } }) }),
    ]); setSaving(false);
  }

  const DRIVERS = [
    { value: "indi_asi_ccd", label: "ZWO ASI (indi_asi_ccd)" },
    { value: "indi_asi_single_ccd", label: "ZWO ASI Single" },
    { value: "indi_qhy_ccd", label: "QHY" },
    { value: "indi_webcam", label: "V4L2 Webcam" },
  ];

  return (
    <Stack gap="md">
      <Title order={2}>Camera Settings</Title>
      <Card shadow="sm" padding="lg" withBorder>
        <Text fw={500} mb="sm">INDI Driver</Text>
        <Select data={DRIVERS} value={driver} onChange={(v) => setDriver(v ?? "")} />
      </Card>
      <Card shadow="sm" padding="lg" withBorder>
        <Text fw={500} mb="sm">Camera Device</Text>
        <Group>
          <Select data={devices.map(d => ({ value: d, label: d }))} value={device} onChange={(v) => setDevice(v ?? "")} placeholder="Select..." style={{ flex: 1 }} />
          <Button onClick={() => fetch("/api/devices").then(r=>r.json()).then(d=>setDevices(d.devices??[]))} variant="outline">Refresh</Button>
        </Group>
      </Card>
      <Card shadow="sm" padding="lg" withBorder>
        <Group justify="space-between" mb="sm">
          <Text fw={500}>Location</Text>
          <Switch label="Use GPSD" checked={gpsdEnabled} onChange={(e) => handleGpsdToggle(e.currentTarget.checked)} />
        </Group>
        {loadingGps && <Loader size="sm" mb="sm" />}
        <Group grow>
          <NumberInput label="Latitude" value={latitude} onChange={v => setLatitude(Number(v))} decimalScale={6} disabled={gpsdEnabled} styles={gpsdEnabled ? { input: { opacity: 0.7 } } : undefined} />
          <NumberInput label="Longitude" value={longitude} onChange={v => setLongitude(Number(v))} decimalScale={6} disabled={gpsdEnabled} styles={gpsdEnabled ? { input: { opacity: 0.7 } } : undefined} />
          <NumberInput label="Elevation (m)" value={elevation} onChange={v => setElevation(Number(v))} decimalScale={1} disabled={gpsdEnabled} styles={gpsdEnabled ? { input: { opacity: 0.7 } } : undefined} />
        </Group>
      </Card>
      <Button onClick={save} loading={saving} fullWidth>Save Camera Settings</Button>
    </Stack>
  );
}
