import { Grid, Card, Text, Title, Stack, Group, Badge } from "@mantine/core";
import { useApi } from "../hooks/useApi";
import { useWebSocket } from "../hooks/useWebSocket";
import { PipelineView } from "../components/PipelineView";

interface StatusResponse {
  safety: { state: string; imagingQuality: string; reason: string | null } | null;
  analysis: { cloudCover: number; brightness: number; skyQuality: string } | null;
  frame: { id: string; capturedAt: string; exposureMs: number; jpegPath: string | null } | null;
}

export function Dashboard() {
  const { data: status } = useApi<StatusResponse>("/api/status", 5000);
  const { autoExposure } = useWebSocket();

  const frameId = status?.frame?.id;
  const hasJpeg = status?.frame?.jpegPath;

  return (
    <Stack gap="md">
      <Title order={2}>Observatory Dashboard</Title>

      <Card shadow="sm" padding="lg" withBorder>
        <Group justify="space-between" mb="sm">
          <Text size="sm" c="dimmed">Latest Frame</Text>
          {autoExposure && (
            <Group gap="xs">
              <Badge color={autoExposure.mode === "day" ? "yellow" : "blue"} size="sm" variant="filled">{autoExposure.mode}</Badge>
              <Text size="xs" c="dimmed">
                {autoExposure.currentExposureMs.toFixed(1)}ms | G{autoExposure.currentGain} | ADU {(autoExposure.lastMedianAdu / 655.35).toFixed(1)}% / {autoExposure.targetAdu}% | {autoExposure.phase}
              </Text>
            </Group>
          )}
        </Group>
        {frameId && hasJpeg ? (
          <img src={`/api/frames/${frameId}/jpeg`} alt="Latest sky frame"
            style={{ width: "100%", maxHeight: 500, objectFit: "contain", borderRadius: 8, background: "#000" }} />
        ) : (
          <Text c="dimmed" ta="center" py="xl">Waiting for processed frame...</Text>
        )}
      </Card>

      <PipelineView />

      <Grid>
        <Grid.Col span={{ base: 12, md: 4 }}>
          <Card shadow="sm" padding="lg" withBorder>
            <Text size="sm" c="dimmed">Safety Status</Text>
            <Group mt="sm">
              <Badge size="xl" color={status?.safety?.state === "SAFE" ? "green" : status?.safety?.state === "UNSAFE" ? "red" : "yellow"} variant="filled">
                {status?.safety?.state ?? "UNKNOWN"}
              </Badge>
              <Stack gap={2}>
                <Text size="sm">Quality: {status?.safety?.imagingQuality ?? "—"}</Text>
                {status?.safety?.reason && <Text size="xs" c="dimmed">{status.safety.reason}</Text>}
              </Stack>
            </Group>
          </Card>
        </Grid.Col>
        <Grid.Col span={{ base: 12, md: 4 }}>
          <Card shadow="sm" padding="lg" withBorder>
            <Text size="sm" c="dimmed">Sky Conditions</Text>
            <Stack gap={4} mt="sm">
              <Text size="sm">Cloud Cover: {((status?.analysis?.cloudCover ?? 0) * 100).toFixed(0)}%</Text>
              <Text size="sm">Brightness: {((status?.analysis?.brightness ?? 0) * 100).toFixed(1)}%</Text>
              <Text size="sm">Sky: {status?.analysis?.skyQuality ?? "—"}</Text>
            </Stack>
          </Card>
        </Grid.Col>
        <Grid.Col span={{ base: 12, md: 4 }}>
          <Card shadow="sm" padding="lg" withBorder>
            <Text size="sm" c="dimmed">Frame Info</Text>
            <Stack gap={4} mt="sm">
              <Text size="sm">Exposure: {status?.frame?.exposureMs?.toFixed(3) ?? "—"} ms</Text>
              <Text size="xs" c="dimmed">{status?.frame?.capturedAt ? new Date(status.frame.capturedAt).toLocaleString() : "—"}</Text>
            </Stack>
          </Card>
        </Grid.Col>
      </Grid>
    </Stack>
  );
}
