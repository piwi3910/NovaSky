import { useState, useRef, useEffect } from "react";
import { Stack, Title, Card, Text, NumberInput, Button, Group, Switch } from "@mantine/core";
import { useApi } from "../hooks/useApi";

export function FrameMasking() {
  const { data: configData } = useApi<Record<string, any>>("/api/config");
  const { data: status } = useApi<any>("/api/status", 5000);
  const [centerX, setCenterX] = useState(1776);
  const [centerY, setCenterY] = useState(1776);
  const [radius, setRadius] = useState(1700);
  const [enabled, setEnabled] = useState(false);
  const [saving, setSaving] = useState(false);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const initialized = useRef(false);

  useEffect(() => {
    if (configData && !initialized.current) {
      initialized.current = true;
      const mask = (configData["imaging.mask"] ?? {}) as any;
      setCenterX(mask.centerX ?? 1776);
      setCenterY(mask.centerY ?? 1776);
      setRadius(mask.radius ?? 1700);
      setEnabled(mask.enabled ?? false);
    }
  }, [configData]);

  // Draw mask preview on canvas
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const img = new Image();
    const frameId = status?.frame?.id;
    if (!frameId || !status?.frame?.jpegPath) return;

    img.onload = () => {
      const scale = canvas.width / img.naturalWidth;
      canvas.height = img.naturalHeight * scale;
      ctx.drawImage(img, 0, 0, canvas.width, canvas.height);

      if (enabled) {
        // Draw mask overlay (darken outside circle)
        ctx.fillStyle = "rgba(0,0,0,0.6)";
        ctx.fillRect(0, 0, canvas.width, canvas.height);
        ctx.globalCompositeOperation = "destination-out";
        ctx.beginPath();
        ctx.arc(centerX * scale, centerY * scale, radius * scale, 0, 2 * Math.PI);
        ctx.fill();
        ctx.globalCompositeOperation = "source-over";

        // Draw circle outline
        ctx.strokeStyle = "#00ff00";
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.arc(centerX * scale, centerY * scale, radius * scale, 0, 2 * Math.PI);
        ctx.stroke();
      }
    };
    img.src = `/api/frames/${frameId}/jpeg`;
  }, [status, centerX, centerY, radius, enabled]);

  async function save() {
    setSaving(true);
    await fetch("/api/config/imaging.mask", {
      method: "PUT", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ value: { centerX, centerY, radius, enabled } }),
    });
    setSaving(false);
  }

  return (
    <Stack gap="md">
      <Title order={2}>Frame Masking</Title>
      <Card shadow="sm" padding="lg" withBorder>
        <Switch label="Enable circular mask" checked={enabled} onChange={e => setEnabled(e.currentTarget.checked)} mb="md" />
        <Group grow mb="md">
          <NumberInput label="Center X" value={centerX} onChange={v => setCenterX(Number(v))} min={0} max={7104} />
          <NumberInput label="Center Y" value={centerY} onChange={v => setCenterY(Number(v))} min={0} max={7104} />
          <NumberInput label="Radius" value={radius} onChange={v => setRadius(Number(v))} min={100} max={3552} />
        </Group>
        <Button onClick={save} loading={saving} fullWidth>Save Mask Settings</Button>
      </Card>
      <Card shadow="sm" padding="lg" withBorder>
        <Text fw={500} mb="sm">Preview</Text>
        <canvas ref={canvasRef} width={800} style={{ width: "100%", background: "#000", borderRadius: 8 }} />
      </Card>
    </Stack>
  );
}
