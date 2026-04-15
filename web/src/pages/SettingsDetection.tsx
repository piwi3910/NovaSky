import { useState, useEffect, useRef } from "react";
import { Stack, Title, Card, Text, NumberInput, Switch, Button, TextInput, Group } from "@mantine/core";
import { useApi } from "../hooks/useApi";

export function SettingsDetection() {
  const { data: configData } = useApi<Record<string, any>>("/api/config");
  const [cloudEnabled, setCloudEnabled] = useState(true);
  const [sqmEnabled, setSqmEnabled] = useState(true);
  const [starsEnabled, setStarsEnabled] = useState(true);
  const [starMinBrightness, setStarMinBrightness] = useState(200);
  const [meteorsEnabled, setMeteorsEnabled] = useState(false);
  const [planesEnabled, setPlanesEnabled] = useState(false);
  const [planesUrl, setPlanesUrl] = useState("http://localhost:8080");
  const [satellitesEnabled, setSatellitesEnabled] = useState(false);
  const [constellationsEnabled, setConstellationsEnabled] = useState(true);
  const [planetsEnabled, setPlanetsEnabled] = useState(true);
  const [cloudThreshold, setCloudThreshold] = useState(80);
  const [saving, setSaving] = useState(false);
  const initialized = useRef(false);

  useEffect(() => {
    if (configData && !initialized.current) {
      initialized.current = true;
      const det = (configData["detection"] ?? {}) as any;
      if (det.cloudEnabled !== undefined) setCloudEnabled(det.cloudEnabled);
      if (det.sqmEnabled !== undefined) setSqmEnabled(det.sqmEnabled);
      if (det.starsEnabled !== undefined) setStarsEnabled(det.starsEnabled);
      if (det.starMinBrightness) setStarMinBrightness(det.starMinBrightness);
      if (det.meteorsEnabled !== undefined) setMeteorsEnabled(det.meteorsEnabled);
      if (det.planesEnabled !== undefined) setPlanesEnabled(det.planesEnabled);
      if (det.planesUrl) setPlanesUrl(det.planesUrl);
      if (det.satellitesEnabled !== undefined) setSatellitesEnabled(det.satellitesEnabled);
      if (det.constellationsEnabled !== undefined) setConstellationsEnabled(det.constellationsEnabled);
      if (det.planetsEnabled !== undefined) setPlanetsEnabled(det.planetsEnabled);
      if (det.cloudThreshold) setCloudThreshold(det.cloudThreshold);
    }
  }, [configData]);

  async function save() {
    setSaving(true);
    await fetch("/api/config/detection", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        value: {
          cloudEnabled, sqmEnabled, starsEnabled, starMinBrightness,
          meteorsEnabled, planesEnabled, planesUrl, satellitesEnabled,
          constellationsEnabled, planetsEnabled, cloudThreshold,
        },
      }),
    });
    setSaving(false);
  }

  return (
    <Stack gap="md">
      <Title order={2}>Detection Settings</Title>

      <Card shadow="sm" padding="lg" withBorder>
        <Text fw={500} mb="sm">Sky Analysis</Text>
        <Stack gap="xs">
          <Switch label="Cloud detection" checked={cloudEnabled} onChange={e => setCloudEnabled(e.currentTarget.checked)} description="Analyze frame brightness and contrast to estimate cloud cover" />
          {cloudEnabled && (
            <NumberInput label="Cloud cover safety threshold (%)" value={cloudThreshold} onChange={v => setCloudThreshold(Number(v))} min={0} max={100} suffix="%" description="Mark UNSAFE above this cloud cover percentage" />
          )}
          <Switch label="SQM calculation" checked={sqmEnabled} onChange={e => setSqmEnabled(e.currentTarget.checked)} description="Compute Sky Quality Meter value from background brightness" />
        </Stack>
      </Card>

      <Card shadow="sm" padding="lg" withBorder>
        <Text fw={500} mb="sm">Object Detection</Text>
        <Stack gap="xs">
          <Switch label="Star detection" checked={starsEnabled} onChange={e => setStarsEnabled(e.currentTarget.checked)} description="Detect point sources, compute HFR/FWHM and star count" />
          {starsEnabled && (
            <NumberInput label="Star min brightness (0-255)" value={starMinBrightness} onChange={v => setStarMinBrightness(Number(v))} min={10} max={255} description="Threshold for star detection — lower = more sensitive" />
          )}
          <Switch label="Meteor detection" checked={meteorsEnabled} onChange={e => setMeteorsEnabled(e.currentTarget.checked)} description="Frame differencing + Hough line detection for fast transients" />
          <Switch label="Plane detection (ADS-B)" checked={planesEnabled} onChange={e => setPlanesEnabled(e.currentTarget.checked)} description="Query local tar1090/dump1090 for aircraft positions" />
          {planesEnabled && (
            <TextInput label="ADS-B URL" value={planesUrl} onChange={e => setPlanesUrl(e.currentTarget.value)} description="tar1090 or dump1090 JSON endpoint" />
          )}
          <Switch label="Satellite tracking (TLE)" checked={satellitesEnabled} onChange={e => setSatellitesEnabled(e.currentTarget.checked)} description="Fetch TLE data from CelesTrak and predict satellite passes" />
        </Stack>
      </Card>

      <Card shadow="sm" padding="lg" withBorder>
        <Text fw={500} mb="sm">Astronomy</Text>
        <Stack gap="xs">
          <Switch label="Constellation projection" checked={constellationsEnabled} onChange={e => setConstellationsEnabled(e.currentTarget.checked)} description="Project constellation lines onto the all-sky image" />
          <Switch label="Planet positions" checked={planetsEnabled} onChange={e => setPlanetsEnabled(e.currentTarget.checked)} description="Compute positions of Mercury through Saturn" />
        </Stack>
      </Card>

      <Button onClick={save} loading={saving} fullWidth>Save Detection Settings</Button>
    </Stack>
  );
}
