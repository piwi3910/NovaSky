import { useApi } from "../hooks/useApi";
import { Card, Text, Group, Badge, Stack } from "@mantine/core";

interface ServiceNode {
  name: string;
  status: string;
  queueDepth: number;
  latency: number;
}

interface PipelineResponse {
  services: ServiceNode[];
}

const STATUS_COLORS: Record<string, string> = {
  running: "#4caf50",
  unknown: "#9e9e9e",
  stale: "#ff9800",
  stopped: "#f44336",
};

function queueColor(depth: number): string {
  if (depth <= 1) return "#4caf50"; // green
  if (depth === 2) return "#ffeb3b"; // yellow
  if (depth === 3) return "#ff9800"; // orange
  return "#f44336"; // red
}

function ServiceBox({ node }: { node: ServiceNode }) {
  const dotColor = STATUS_COLORS[node.status] ?? "#9e9e9e";

  return (
    <div style={{
      background: "#2a2a2a",
      border: `2px solid ${dotColor}`,
      borderRadius: 8,
      padding: "8px 12px",
      minWidth: 100,
      textAlign: "center",
      position: "relative",
    }}>
      <div style={{
        position: "absolute", top: -4, right: -4,
        width: 10, height: 10, borderRadius: "50%",
        background: dotColor,
      }} />
      <Text size="xs" fw={600} c="white">{node.name}</Text>
      {node.queueDepth > 0 && (
        <Badge size="xs" color={node.queueDepth > 3 ? "red" : node.queueDepth > 1 ? "yellow" : "green"} variant="filled" mt={2}>
          Q: {node.queueDepth}
        </Badge>
      )}
      {node.latency > 0 && (
        <Text size="xs" c="dimmed" mt={2}>{node.latency.toFixed(1)}s</Text>
      )}
    </div>
  );
}

function Arrow({ depth = 0 }: { depth?: number }) {
  const color = queueColor(depth);
  return (
    <div style={{ display: "flex", alignItems: "center", margin: "0 4px" }}>
      <div style={{
        width: 30, height: 3,
        background: color,
        position: "relative",
      }}>
        <div style={{
          position: "absolute", right: -4, top: -4,
          width: 0, height: 0,
          borderLeft: `8px solid ${color}`,
          borderTop: "5px solid transparent",
          borderBottom: "5px solid transparent",
        }} />
      </div>
    </div>
  );
}

function VerticalArrow({ depth = 0 }: { depth?: number }) {
  const color = queueColor(depth);
  return (
    <div style={{ display: "flex", justifyContent: "center", margin: "2px 0" }}>
      <div style={{
        width: 3, height: 20,
        background: color,
        position: "relative",
      }}>
        <div style={{
          position: "absolute", bottom: -4, left: -4,
          width: 0, height: 0,
          borderTop: `8px solid ${color}`,
          borderLeft: "5px solid transparent",
          borderRight: "5px solid transparent",
        }} />
      </div>
    </div>
  );
}

export function PipelineView() {
  const { data } = useApi<PipelineResponse>("/api/pipeline", 3000);

  if (!data) return <Text c="dimmed">Loading pipeline...</Text>;

  const svc = (name: string) =>
    data.services.find(s => s.name === name) ?? { name, status: "unknown", queueDepth: 0, latency: 0 };

  const processing = svc("processing");
  const detection = svc("detection");
  const overlay = svc("overlay");
  const exportSvc = svc("export");
  const timelapse = svc("timelapse");

  return (
    <Card shadow="sm" padding="lg" withBorder>
      <Text size="sm" c="dimmed" mb="md">Pipeline Status</Text>

      {/* Main pipeline row */}
      <div style={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 4, marginBottom: 12 }}>
        <ServiceBox node={svc("ingest-camera")} />
        <Arrow depth={Number(processing.queueDepth)} />
        <ServiceBox node={processing} />
        <Arrow depth={Number(detection.queueDepth)} />
        <ServiceBox node={detection} />
        <Arrow depth={Number(svc("policy").queueDepth)} />
        <ServiceBox node={svc("policy")} />
        <Arrow depth={Number(svc("alerts").queueDepth)} />
        <ServiceBox node={svc("alerts")} />
      </div>

      {/* Branch row — services fed from processing, aligned under "processing" node */}
      <div style={{ display: "flex", gap: 16, paddingLeft: "calc(100px + 24px + 38px)", marginBottom: 12 }}>
        <Stack gap={0} align="center">
          <VerticalArrow depth={Number(overlay.queueDepth)} />
          <ServiceBox node={overlay} />
        </Stack>
        <Stack gap={0} align="center">
          <VerticalArrow depth={Number(exportSvc.queueDepth)} />
          <ServiceBox node={exportSvc} />
        </Stack>
        <Stack gap={0} align="center">
          <VerticalArrow depth={Number(timelapse.queueDepth)} />
          <ServiceBox node={timelapse} />
        </Stack>
      </div>

      {/* Standalone services */}
      <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
        <ServiceBox node={svc("api")} />
        <ServiceBox node={svc("alpaca")} />
        <ServiceBox node={svc("stream")} />
      </div>

      {/* Legend */}
      <Group mt="md" gap="lg">
        <Group gap={4}><div style={{ width: 8, height: 8, borderRadius: "50%", background: "#4caf50" }} /><Text size="xs" c="dimmed">Running</Text></Group>
        <Group gap={4}><div style={{ width: 8, height: 8, borderRadius: "50%", background: "#ff9800" }} /><Text size="xs" c="dimmed">Stale</Text></Group>
        <Group gap={4}><div style={{ width: 8, height: 8, borderRadius: "50%", background: "#f44336" }} /><Text size="xs" c="dimmed">Stopped</Text></Group>
        <Group gap={4}><div style={{ width: 20, height: 3, background: "#4caf50" }} /><Text size="xs" c="dimmed">Clear</Text></Group>
        <Group gap={4}><div style={{ width: 20, height: 3, background: "#ffeb3b" }} /><Text size="xs" c="dimmed">Pressure</Text></Group>
        <Group gap={4}><div style={{ width: 20, height: 3, background: "#f44336" }} /><Text size="xs" c="dimmed">Jammed</Text></Group>
      </Group>
    </Card>
  );
}
