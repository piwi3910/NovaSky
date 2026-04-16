import { useState, useMemo } from "react";
import { Stack, Title, Card, Text, Button, Group, Badge, SimpleGrid } from "@mantine/core";
import { useApi } from "../hooks/useApi";
import { useWebSocket } from "../hooks/useWebSocket";

interface FocusStatus { focusMode: boolean; }
interface StarsData { id: string; data: string; }

export function FocusMode() {
  const { data: focusData } = useApi<FocusStatus>("/api/focus/status", 2000);
  const { data: starsData } = useApi<StarsData>("/api/stars", 3000);
  const { data: status } = useApi<any>("/api/status", 3000);
  const [loading, setLoading] = useState(false);

  const { stars, avgHFR, avgFWHM, peakBrightness } = useMemo(() => {
    const rawData = starsData?.data;
    const s = Array.isArray(rawData) ? rawData : (typeof rawData === "string" ? JSON.parse(rawData) : []);
    return {
      stars: s,
      avgHFR: s.length > 0 ? (s.reduce((sum: number, st: any) => sum + st.hfr, 0) / s.length).toFixed(2) : "—",
      avgFWHM: s.length > 0 ? (s.reduce((sum: number, st: any) => sum + st.fwhm, 0) / s.length).toFixed(2) : "—",
      peakBrightness: s.length > 0 ? Math.max(...s.map((st: any) => st.brightness)).toFixed(0) : "—",
    };
  }, [starsData]);

  async function toggle() {
    setLoading(true);
    try {
      const endpoint = focusData?.focusMode ? "/api/focus/stop" : "/api/focus/start";
      await fetch(endpoint, { method: "POST" });
    } catch (e) {
      alert("Failed to save settings. Please try again.");
    } finally {
      setLoading(false);
    }
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

        <Button onClick={toggle} loading={loading} fullWidth color={focusData?.focusMode ? "red" : "green"} mb="md">
          {focusData?.focusMode ? "Stop Focus Mode" : "Start Focus Mode"}
        </Button>
      </Card>

      <Card shadow="sm" padding="lg" withBorder>
        <Text fw={500} mb="sm">Focus Metrics</Text>
        <SimpleGrid cols={{ base: 2, md: 4 }}>
          <div><Text size="xs" c="dimmed">Stars Detected</Text><Text size="xl" fw={700}>{stars.length}</Text></div>
          <div><Text size="xs" c="dimmed">Avg HFR</Text><Text size="xl" fw={700}>{avgHFR}</Text></div>
          <div><Text size="xs" c="dimmed">Avg FWHM</Text><Text size="xl" fw={700}>{avgFWHM}</Text></div>
          <div><Text size="xs" c="dimmed">Peak Brightness</Text><Text size="xl" fw={700}>{peakBrightness}</Text></div>
        </SimpleGrid>
      </Card>

      {status?.frame?.id && status?.frame?.jpegPath && (
        <Card shadow="sm" padding="lg" withBorder>
          <Text fw={500} mb="sm">Live Frame</Text>
          <img src={`/api/frames/${status.frame.id}/jpeg`} alt="Focus frame"
            style={{ width: "100%", maxHeight: 600, objectFit: "contain", background: "#000", borderRadius: 8 }} />
        </Card>
      )}
    </Stack>
  );
}
