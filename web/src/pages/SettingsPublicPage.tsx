import { useState, useEffect, useRef } from "react";
import { Stack, Title, Card, Text, TextInput, Switch, Button } from "@mantine/core";
import { useApi } from "../hooks/useApi";

export function SettingsPublicPage() {
  const { data: configData } = useApi<Record<string, any>>("/api/config");
  const [name, setName] = useState("NovaSky Observatory");
  const [showSQM, setShowSQM] = useState(true);
  const [showCloud, setShowCloud] = useState(true);
  const [showSensors, setShowSensors] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const initialized = useRef(false);

  useEffect(() => {
    if (configData && !initialized.current) {
      initialized.current = true;
      const pub = (configData["public"] ?? {}) as any;
      setName(pub.name ?? "NovaSky Observatory");
      setShowSQM(pub.showSQM ?? true);
      setShowCloud(pub.showCloud ?? true);
      setShowSensors(pub.showSensors ?? true);
    }
  }, [configData]);

  async function save() {
    setSaving(true);
    try {
      await fetch("/api/config/public", {
        method: "PUT", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ value: { name, showSQM, showCloud, showSensors } }),
      });
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (e) {
      alert("Failed to save settings. Please try again.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Stack gap="md">
      <Title order={2}>Public Sharing Page</Title>
      <Card shadow="sm" padding="lg" withBorder>
        <TextInput label="Observatory Name" value={name} onChange={e => setName(e.currentTarget.value)} mb="md" />
        <Switch label="Show SQM data" checked={showSQM} onChange={e => setShowSQM(e.currentTarget.checked)} mb="sm" />
        <Switch label="Show cloud cover" checked={showCloud} onChange={e => setShowCloud(e.currentTarget.checked)} mb="sm" />
        <Switch label="Show sensor readings" checked={showSensors} onChange={e => setShowSensors(e.currentTarget.checked)} />
      </Card>
      <Button onClick={save} loading={saving} fullWidth color={saved ? "green" : undefined}>{saved ? "Saved!" : "Save Public Page Settings"}</Button>
      <Card shadow="sm" padding="lg" withBorder>
        <Text size="sm" c="dimmed">Public page URL: <a href="/public" target="_blank">{window.location.origin}/public</a></Text>
      </Card>
    </Stack>
  );
}
