import { useState, useEffect, useRef } from "react";
import { Stack, Title, Card, Text, NumberInput, Button, Progress, Group } from "@mantine/core";
import { useApi } from "../hooks/useApi";

interface DiskInfo { totalGB: number; usedGB: number; freeGB: number; path: string; }

export function SettingsDisk() {
  const { data: disk } = useApi<DiskInfo>("/api/disk", 10000);
  const { data: configData } = useApi<Record<string, any>>("/api/config");
  const [retainDays, setRetainDays] = useState(30);
  const [minFreeGB, setMinFreeGB] = useState(5);
  const [saving, setSaving] = useState(false);
  const initialized = useRef(false);

  useEffect(() => {
    if (configData && !initialized.current) {
      initialized.current = true;
      const d = (configData["disk"] ?? {}) as any;
      setRetainDays(d.retainDays ?? 30);
      setMinFreeGB(d.minFreeGB ?? 5);
    }
  }, [configData]);

  async function save() {
    setSaving(true);
    await fetch("/api/config/disk", {
      method: "PUT", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ value: { retainDays, minFreeGB } }),
    });
    setSaving(false);
  }

  const usedPct = disk ? (disk.usedGB / disk.totalGB * 100) : 0;

  return (
    <Stack gap="md">
      <Title order={2}>Disk Management</Title>
      <Card shadow="sm" padding="lg" withBorder>
        <Text fw={500} mb="sm">Disk Usage</Text>
        <Progress value={usedPct} color={usedPct > 90 ? "red" : usedPct > 70 ? "yellow" : "green"} size="xl" mb="sm" />
        <Group justify="space-between">
          <Text size="sm">{disk?.usedGB.toFixed(1)} GB used</Text>
          <Text size="sm">{disk?.freeGB.toFixed(1)} GB free</Text>
          <Text size="sm">{disk?.totalGB.toFixed(1)} GB total</Text>
        </Group>
        <Text size="xs" c="dimmed" mt="xs">Path: {disk?.path}</Text>
      </Card>
      <Card shadow="sm" padding="lg" withBorder>
        <Text fw={500} mb="sm">Retention Policy</Text>
        <NumberInput label="Keep frames for (days)" value={retainDays} onChange={v => setRetainDays(Number(v))} min={1} max={365} mb="sm" />
        <NumberInput label="Minimum free space (GB)" value={minFreeGB} onChange={v => setMinFreeGB(Number(v))} min={1} max={100} />
      </Card>
      <Button onClick={save} loading={saving} fullWidth>Save Disk Settings</Button>
    </Stack>
  );
}
