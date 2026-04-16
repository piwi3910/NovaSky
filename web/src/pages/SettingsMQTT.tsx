import { useState, useEffect, useRef } from "react";
import { Stack, Title, Card, TextInput, Switch, PasswordInput, Button } from "@mantine/core";
import { useApi } from "../hooks/useApi";

export function SettingsMQTT() {
  const { data: configData } = useApi<Record<string, any>>("/api/config");
  const [broker, setBroker] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [enabled, setEnabled] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const initialized = useRef(false);

  useEffect(() => {
    if (configData && !initialized.current) {
      initialized.current = true;
      const mqtt = (configData["mqtt"] ?? {}) as any;
      setBroker(mqtt.broker ?? "");
      setUsername(mqtt.username ?? "");
      setPassword(mqtt.password ?? "");
      setEnabled(mqtt.enabled ?? false);
    }
  }, [configData]);

  async function save() {
    setSaving(true);
    try {
      await fetch("/api/config/mqtt", {
        method: "PUT", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ value: { broker, username, password, enabled } }),
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
      <Title order={2}>MQTT / Home Assistant</Title>
      <Card shadow="sm" padding="lg" withBorder>
        <Switch label="Enable MQTT" checked={enabled} onChange={e => setEnabled(e.currentTarget.checked)} mb="md" />
        <TextInput label="Broker Address" value={broker} onChange={e => setBroker(e.currentTarget.value)}
          placeholder="192.168.1.100:1883" disabled={!enabled} mb="sm" />
        <TextInput label="Username" value={username} onChange={e => setUsername(e.currentTarget.value)}
          disabled={!enabled} mb="sm" />
        <PasswordInput label="Password" value={password} onChange={e => setPassword(e.currentTarget.value)}
          disabled={!enabled} />
      </Card>
      <Button onClick={save} loading={saving} fullWidth color={saved ? "green" : undefined}>{saved ? "Saved!" : "Save MQTT Settings"}</Button>
    </Stack>
  );
}
