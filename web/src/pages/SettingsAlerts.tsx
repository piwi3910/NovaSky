import { useState, useEffect, useRef } from "react";
import { Stack, Title, Card, Text, TextInput, Switch, Button } from "@mantine/core";
import { useApi } from "../hooks/useApi";

export function SettingsAlerts() {
  const { data: configData } = useApi<Record<string, any>>("/api/config");
  const [webhookUrl, setWebhookUrl] = useState("");
  const [webhookEnabled, setWebhookEnabled] = useState(false);
  const [saving, setSaving] = useState(false);
  const initialized = useRef(false);

  useEffect(() => {
    if (configData && !initialized.current) {
      initialized.current = true;
      const alerts = (configData["alerts"] ?? {}) as any;
      setWebhookUrl(alerts.webhookUrl ?? "");
      setWebhookEnabled(alerts.webhookEnabled ?? false);
    }
  }, [configData]);

  async function save() {
    setSaving(true);
    await fetch("/api/config/alerts", {
      method: "PUT", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ value: { webhookUrl, webhookEnabled } }),
    });
    setSaving(false);
  }

  return (
    <Stack gap="md">
      <Title order={2}>Alert Settings</Title>
      <Card shadow="sm" padding="lg" withBorder>
        <Text fw={500} mb="sm">Webhook</Text>
        <Switch label="Enable webhook alerts" checked={webhookEnabled}
          onChange={e => setWebhookEnabled(e.currentTarget.checked)} mb="sm" />
        <TextInput label="Webhook URL" value={webhookUrl}
          onChange={e => setWebhookUrl(e.currentTarget.value)}
          placeholder="https://example.com/webhook" disabled={!webhookEnabled} />
      </Card>
      <Button onClick={save} loading={saving} fullWidth>Save Alert Settings</Button>
    </Stack>
  );
}
