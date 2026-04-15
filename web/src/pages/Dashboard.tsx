import { useState } from "react";
import { Grid, Card, Text, Title, Stack, Group, Badge, Modal } from "@mantine/core";
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid } from "recharts";
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
  const { data: cloudData } = useApi<Array<{time: string; value: number}>>("/api/charts/cloud-cover?hours=6", 30000);
  const { data: exposureData } = useApi<Array<{time: string; exposure: number; gain: number}>>("/api/charts/exposure?hours=24", 30000);
  const { data: astroData } = useApi<any>("/api/astronomy", 60000);
  const { autoExposure } = useWebSocket();

  const [fullscreen, setFullscreen] = useState(false);
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
            onClick={() => setFullscreen(true)}
            style={{ width: "100%", maxHeight: 500, objectFit: "contain", borderRadius: 8, background: "#000", cursor: "pointer" }} />
        ) : (
          <Text c="dimmed" ta="center" py="xl">Waiting for processed frame...</Text>
        )}
      </Card>

      {fullscreen && frameId && hasJpeg && (
        <div
          ref={(el) => { if (el) { el.scrollLeft = (el.scrollWidth - el.clientWidth) / 2; el.scrollTop = (el.scrollHeight - el.clientHeight) / 2; } }}
          onClick={() => setFullscreen(false)}
          style={{
            position: "fixed", top: 0, left: 0, right: 0, bottom: 0,
            zIndex: 9999, background: "rgba(0,0,0,0.95)",
            overflow: "auto", cursor: "pointer",
          }}
        >
          <img src={`/api/frames/${frameId}/jpeg`} alt="Full size frame"
            style={{ maxWidth: "none", height: "auto", objectFit: "none" }} />
        </div>
      )}

      <PipelineView />

      <Card shadow="sm" padding="lg" withBorder>
        <Text size="sm" c="dimmed" mb="sm">Cloud Cover (last 6 hours)</Text>
        {cloudData && cloudData.length > 0 ? (
          <ResponsiveContainer width="100%" height={200}>
            <LineChart data={cloudData}>
              <XAxis dataKey="time" tick={{ fontSize: 10 }} tickFormatter={t => new Date(t).toLocaleTimeString()} interval="preserveStartEnd" />
              <YAxis domain={[0, 100]} tick={{ fontSize: 10 }} />
              <Tooltip formatter={(v: number) => `${v.toFixed(0)}%`} labelFormatter={t => new Date(t).toLocaleString()} />
              <Line type="monotone" dataKey="value" stroke="#228be6" dot={false} strokeWidth={2} />
            </LineChart>
          </ResponsiveContainer>
        ) : (
          <Text size="sm" c="dimmed">No data yet</Text>
        )}
      </Card>

      <Card shadow="sm" padding="lg" withBorder>
        <Text size="sm" c="dimmed" mb="sm">Exposure &amp; Gain (last 24 hours)</Text>
        {exposureData && exposureData.length > 0 ? (
          <ResponsiveContainer width="100%" height={200}>
            <LineChart data={exposureData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#333" />
              <XAxis dataKey="time" tick={{ fontSize: 10 }} tickFormatter={t => new Date(t).toLocaleTimeString()} interval="preserveStartEnd" />
              <YAxis yAxisId="left" tick={{ fontSize: 10 }} label={{ value: "Exposure (ms)", angle: -90, position: "insideLeft", style: { fontSize: 10 } }} />
              <YAxis yAxisId="right" orientation="right" tick={{ fontSize: 10 }} label={{ value: "Gain", angle: 90, position: "insideRight", style: { fontSize: 10 } }} />
              <Tooltip
                formatter={(v: number, name: string) => [name === "exposure" ? `${v.toFixed(1)} ms` : `${v}`, name === "exposure" ? "Exposure" : "Gain"]}
                labelFormatter={t => new Date(t).toLocaleString()}
              />
              <Line yAxisId="left" type="monotone" dataKey="exposure" stroke="#fab005" dot={false} strokeWidth={2} name="exposure" />
              <Line yAxisId="right" type="monotone" dataKey="gain" stroke="#40c057" dot={false} strokeWidth={2} name="gain" />
            </LineChart>
          </ResponsiveContainer>
        ) : (
          <Text size="sm" c="dimmed">No data yet</Text>
        )}
      </Card>

      {astroData && (
        <Card shadow="sm" padding="lg" withBorder>
          <Text size="sm" c="dimmed" mb="sm">Astronomy</Text>
          <Group gap="xl">
            <Stack gap={2}>
              <Text size="sm" fw={500}>Moon</Text>
              <Text size="sm">{astroData.moon?.phase} ({astroData.moon?.illumination}%)</Text>
            </Stack>
            {astroData.bortle?.class > 0 && (
              <Stack gap={2}>
                <Text size="sm" fw={500}>Bortle Class</Text>
                <Text size="sm">{astroData.bortle?.class} — {astroData.bortle?.description}</Text>
              </Stack>
            )}
            <Stack gap={2}>
              <Text size="sm" fw={500}>Sunset</Text>
              <Text size="sm">{astroData.sun?.sunset ? new Date(astroData.sun.sunset).toLocaleTimeString() : '—'}</Text>
            </Stack>
            <Stack gap={2}>
              <Text size="sm" fw={500}>Astro Twilight</Text>
              <Text size="sm">{astroData.sun?.astronomicalDusk ? new Date(astroData.sun.astronomicalDusk).toLocaleTimeString() : '—'}</Text>
            </Stack>
          </Group>
        </Card>
      )}

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
