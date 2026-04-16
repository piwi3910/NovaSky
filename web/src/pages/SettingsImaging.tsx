import { useState, useEffect, useRef } from "react";
import { Stack, Title, Card, Text, NumberInput, SegmentedControl, Button, Group, Badge, Select, Switch } from "@mantine/core";
import { useApi } from "../hooks/useApi";
import { useWebSocket } from "../hooks/useWebSocket";

interface ImagingProfile { gain: number; minExposureMs: number; maxExposureMs: number; aduTarget: number; stretch: string; stackingEnabled?: boolean; stackingCount?: number; }

export function SettingsImaging() {
  const { data: configData } = useApi<Record<string, any>>("/api/config");
  const { autoExposure } = useWebSocket();
  const [activeTab, setActiveTab] = useState("day");
  const [dayProfile, setDayProfile] = useState<ImagingProfile>({ gain: 0, minExposureMs: 0.032, maxExposureMs: 5000, aduTarget: 30, stretch: "none" });
  const [nightProfile, setNightProfile] = useState<ImagingProfile>({ gain: 300, minExposureMs: 1000, maxExposureMs: 30000, aduTarget: 30, stretch: "auto" });
  const [twilightAngle, setTwilightAngle] = useState(-6);
  const [transitionSpeed, setTransitionSpeed] = useState(25);
  const [saving, setSaving] = useState(false);
  const initialized = useRef(false);

  useEffect(() => {
    if (configData && !initialized.current) {
      initialized.current = true;
      if (configData["imaging.day"]) setDayProfile(configData["imaging.day"] as ImagingProfile);
      if (configData["imaging.night"]) setNightProfile(configData["imaging.night"] as ImagingProfile);
      const tw = configData["imaging.twilight"] as any;
      if (tw?.sunAltitude !== undefined) setTwilightAngle(tw.sunAltitude);
      if (tw?.transitionSpeed !== undefined) setTransitionSpeed(tw.transitionSpeed);
    }
  }, [configData]);

  const profile = activeTab === "day" ? dayProfile : nightProfile;
  const setProfile = activeTab === "day" ? setDayProfile : setNightProfile;
  function updateField(field: keyof ImagingProfile, value: string | number | boolean) { setProfile(prev => ({ ...prev, [field]: value })); }

  async function save() {
    setSaving(true);
    try {
      await Promise.all([
        fetch("/api/config/imaging.day", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ value: dayProfile }) }),
        fetch("/api/config/imaging.night", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ value: nightProfile }) }),
        fetch("/api/config/imaging.twilight", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ value: { sunAltitude: twilightAngle, transitionSpeed } }) }),
      ]);
    } catch (e) {
      alert("Failed to save settings. Please try again.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Stack gap="md">
      <Title order={2}>Imaging Settings</Title>
      {autoExposure && (
        <Card shadow="sm" padding="lg" withBorder>
          <Text fw={500} mb="sm">Live Auto-Exposure</Text>
          <Group>
            <Badge color={autoExposure.mode === "day" ? "yellow" : "blue"} size="lg" variant="filled">{autoExposure.mode}</Badge>
            <Text size="sm">Sun: {autoExposure.sunAltitude.toFixed(1)}°</Text>
            <Text size="sm">Exp: {autoExposure.currentExposureMs.toFixed(1)}ms</Text>
            <Text size="sm">Gain: {autoExposure.currentGain}</Text>
            <Text size="sm">ADU: {(autoExposure.lastMedianAdu / 655.35).toFixed(1)}% / {autoExposure.targetAdu}%</Text>
            <Badge size="sm" variant="outline">{autoExposure.phase}</Badge>
          </Group>
        </Card>
      )}
      <Card shadow="sm" padding="lg" withBorder>
        <Group grow>
          <NumberInput label="Twilight angle (°)" value={twilightAngle} onChange={v => setTwilightAngle(Number(v))} min={-18} max={0} step={1} suffix="°" description="Civil: -6°, Nautical: -12°, Astronomical: -18°" />
          <NumberInput label="Gain ramp speed" value={transitionSpeed} onChange={v => setTransitionSpeed(Number(v))} min={1} max={100} description="Gain steps per frame during twilight" />
        </Group>
      </Card>
      <Card shadow="sm" padding="lg" withBorder>
        <SegmentedControl value={activeTab} onChange={setActiveTab} data={[{ value: "day", label: "Day" }, { value: "night", label: "Night" }]} fullWidth mb="md" />
        <Stack gap="sm">
          <NumberInput label="Gain" value={profile.gain} onChange={v => updateField("gain", Number(v))} min={0} max={600} />
          <Group grow>
            <NumberInput label="Min Exposure (ms)" value={profile.minExposureMs} onChange={v => updateField("minExposureMs", Number(v))} min={0.032} max={60000} decimalScale={3} />
            <NumberInput label="Max Exposure (ms)" value={profile.maxExposureMs} onChange={v => updateField("maxExposureMs", Number(v))} min={1} max={120000} />
          </Group>
          <NumberInput label="ADU Target (%)" value={profile.aduTarget} onChange={v => updateField("aduTarget", Number(v))} min={1} max={100} suffix="%" />
          <Select label="JPEG Stretch" data={[{ value: "none", label: "None" }, { value: "linear", label: "Linear" }, { value: "auto", label: "Auto (per-channel)" }, { value: "adaptive", label: "Adaptive (MTF)" }, { value: "ghs", label: "GHS (Arcsinh)" }]} value={profile.stretch ?? "none"} onChange={v => updateField("stretch", v ?? "none")} />
        </Stack>
      </Card>
      <Card shadow="sm" padding="lg" withBorder>
        <Text fw={500} mb="sm">Frame Stacking ({activeTab})</Text>
        <Switch label="Enable stacking" checked={profile.stackingEnabled ?? false} onChange={e => updateField("stackingEnabled", e.currentTarget.checked)} mb="sm" description="Stack multiple frames before processing — reduces noise, brings out faint stars" />
        {profile.stackingEnabled && (
          <NumberInput label="Frames to stack" value={profile.stackingCount ?? 5} onChange={v => updateField("stackingCount", Number(v))} min={2} max={20} description="Number of consecutive frames averaged together" />
        )}
      </Card>
      <Button onClick={save} loading={saving} fullWidth>Save Imaging Settings</Button>
    </Stack>
  );
}
