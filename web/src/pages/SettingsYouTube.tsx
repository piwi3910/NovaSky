import { useState, useEffect, useRef } from "react";
import { Stack, Title, Card, TextInput, PasswordInput, Select, Switch, Button } from "@mantine/core";
import { useApi } from "../hooks/useApi";

export function SettingsYouTube() {
  const { data: configData } = useApi<Record<string, any>>("/api/config");
  const [enabled, setEnabled] = useState(false);
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [refreshToken, setRefreshToken] = useState("");
  const [privacy, setPrivacy] = useState("unlisted");
  const [saving, setSaving] = useState(false);
  const initialized = useRef(false);

  useEffect(() => {
    if (configData && !initialized.current) {
      initialized.current = true;
      const yt = (configData["youtube"] ?? {}) as any;
      setEnabled(yt.enabled ?? false);
      setClientId(yt.clientId ?? "");
      setClientSecret(yt.clientSecret ?? "");
      setRefreshToken(yt.refreshToken ?? "");
      setPrivacy(yt.privacy ?? "unlisted");
    }
  }, [configData]);

  async function save() {
    setSaving(true);
    await fetch("/api/config/youtube", {
      method: "PUT", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ value: { enabled, clientId, clientSecret, refreshToken, privacy } }),
    });
    setSaving(false);
  }

  return (
    <Stack gap="md">
      <Title order={2}>YouTube Auto-Publish</Title>
      <Card shadow="sm" padding="lg" withBorder>
        <Switch label="Enable YouTube upload" checked={enabled} onChange={e => setEnabled(e.currentTarget.checked)} mb="md" />
        <TextInput label="OAuth2 Client ID" value={clientId} onChange={e => setClientId(e.currentTarget.value)}
          disabled={!enabled} mb="sm" />
        <PasswordInput label="Client Secret" value={clientSecret} onChange={e => setClientSecret(e.currentTarget.value)}
          disabled={!enabled} mb="sm" />
        <PasswordInput label="Refresh Token" value={refreshToken} onChange={e => setRefreshToken(e.currentTarget.value)}
          disabled={!enabled} mb="sm" />
        <Select label="Privacy" data={[
          { value: "public", label: "Public" },
          { value: "unlisted", label: "Unlisted" },
          { value: "private", label: "Private" },
        ]} value={privacy} onChange={v => setPrivacy(v ?? "unlisted")} disabled={!enabled} />
      </Card>
      <Button onClick={save} loading={saving} fullWidth>Save YouTube Settings</Button>
    </Stack>
  );
}
