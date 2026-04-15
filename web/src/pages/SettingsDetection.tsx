import { useState, useEffect, useRef } from "react";
import { Stack, Title, Card, Text, NumberInput, Switch, Button } from "@mantine/core";
import { useApi } from "../hooks/useApi";

export function SettingsDetection() {
  const { data: configData } = useApi<Record<string, any>>("/api/config");
  const [cloudThreshold, setCloudThreshold] = useState(80);
  const [sqmEnabled, setSqmEnabled] = useState(true);
  const [saving, setSaving] = useState(false);
  const initialized = useRef(false);

  useEffect(() => {
    if (configData && !initialized.current) {
      initialized.current = true;
      const det = (configData["detection"] ?? {}) as any;
      setCloudThreshold(det.cloudThreshold ?? 80);
      setSqmEnabled(det.sqmEnabled ?? true);
    }
  }, [configData]);

  async function save() {
    setSaving(true);
    await fetch("/api/config/detection", {
      method: "PUT", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ value: { cloudThreshold, sqmEnabled } }),
    });
    setSaving(false);
  }

  return (
    <Stack gap="md">
      <Title order={2}>Detection Settings</Title>
      <Card shadow="sm" padding="lg" withBorder>
        <NumberInput label="Cloud cover threshold (%)" value={cloudThreshold}
          onChange={v => setCloudThreshold(Number(v))} min={0} max={100} suffix="%"
          description="Safety state changes to UNSAFE above this threshold" />
      </Card>
      <Card shadow="sm" padding="lg" withBorder>
        <Switch label="Enable SQM calculation" checked={sqmEnabled}
          onChange={e => setSqmEnabled(e.currentTarget.checked)}
          description="Compute Sky Quality Meter from frame analysis" />
      </Card>
      <Button onClick={save} loading={saving} fullWidth>Save Detection Settings</Button>
    </Stack>
  );
}
