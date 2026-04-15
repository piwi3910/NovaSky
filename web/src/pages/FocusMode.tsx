import { useState } from "react";
import { Stack, Title, Card, Text, Button, Group, Badge, NumberInput, SimpleGrid } from "@mantine/core";
import { useApi } from "../hooks/useApi";
import { useWebSocket } from "../hooks/useWebSocket";

interface FocusStatus { focusMode: boolean; }
interface StarsData { id: string; data: string; }

export function FocusMode() {
  const { data: focusData } = useApi<FocusStatus>("/api/focus/status", 2000);
  const { data: starsData } = useApi<StarsData>("/api/stars", 3000);
  const { data: status } = useApi<any>("/api/status", 3000);
  const [loading, setLoading] = useState(false);
  const [exposure, setExposure] = useState(500);
  const [gain, setGain] = useState(200);

  const rawData = starsData?.data;
  const stars = Array.isArray(rawData) ? rawData : (typeof rawData === "string" ? JSON.parse(rawData) : []);
  const avgHFR = stars.length > 0 ? (stars.reduce((s: number, st: any) => s + st.hfr, 0) / stars.length).toFixed(2) : "—";
  const avgFWHM = stars.length > 0 ? (stars.reduce((s: number, st: any) => s + st.fwhm, 0) / stars.length).toFixed(2) : "—";
  const peakBrightness = stars.length > 0 ? Math.max(...stars.map((s: any) => s.brightness)).toFixed(0) : "—";

  async function toggle() {
    setLoading(true);
    const endpoint = focusData?.focusMode ? "/api/focus/stop" : "/api/focus/start";
    await fetch(endpoint, { method: "POST" });
    setLoading(false);
  }

  return (
    <Stack gap="md">
      <Title order={2}>Focus Mode</Title>
      <Card shadow="sm" padding="lg" withBorder>
        <Group justify="space-between" mb="md">
          <Text fw={500}>Status</Text>
          <Badge size="lg" color={focusData?.focusMode ? "green" : "gray"} variant="filled">
            {focusData?.focusMode ? "ACTIVE" : "INACTIVE"}
          </Badge>
        </Group>

        <Group grow mb="md">
          <NumberInput label="Exposure (ms)" value={exposure} onChange={v => setExposure(Number(v))} min={1} max={30000} />
          <NumberInput label="Gain" value={gain} onChange={v => setGain(Number(v))} min={0} max={600} />
        </Group>

        <Button onClick={toggle} loading={loading} fullWidth color={focusData?.focusMode ? "red" : "green"} mb="md">
          {focusData?.focusMode ? "Stop Focus Mode" : "Start Focus Mode"}
        </Button>
      </Card>

      <Card shadow="sm" padding="lg" withBorder>
        <Text fw={500} mb="sm">Focus Metrics</Text>
        <SimpleGrid cols={4}>
          <div><Text size="xs" c="dimmed">Stars Detected</Text><Text size="xl" fw={700}>{stars.length}</Text></div>
          <div><Text size="xs" c="dimmed">Avg HFR</Text><Text size="xl" fw={700}>{avgHFR}</Text></div>
          <div><Text size="xs" c="dimmed">Avg FWHM</Text><Text size="xl" fw={700}>{avgFWHM}</Text></div>
          <div><Text size="xs" c="dimmed">Peak Brightness</Text><Text size="xl" fw={700}>{peakBrightness}</Text></div>
        </SimpleGrid>
      </Card>

      {status?.frame?.id && status?.frame?.jpegPath && (
        <Card shadow="sm" padding="lg" withBorder>
          <Text fw={500} mb="sm">Live Frame</Text>
          <img src={`/api/frames/${status.frame.id}/jpeg?t=${Date.now()}`} alt="Focus frame"
            style={{ width: "100%", maxHeight: 600, objectFit: "contain", background: "#000", borderRadius: 8 }} />
        </Card>
      )}
    </Stack>
  );
}
