import { useState, useRef, useEffect, useCallback } from "react";
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
  const containerRef = useRef<HTMLDivElement>(null);
  const initialized = useRef(false);
  const imgRef = useRef<HTMLImageElement | null>(null);
  const scaleRef = useRef(1);
  const dragging = useRef<"center" | "edge" | null>(null);

  // Image dimensions from the camera (pixels)
  const [imgW, setImgW] = useState(3552);
  const [imgH, setImgH] = useState(3552);

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

  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    const img = imgRef.current;
    if (!canvas || !img || !img.complete) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const scale = scaleRef.current;

    // Clear and draw image
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.drawImage(img, 0, 0, canvas.width, canvas.height);

    // Always draw the mask circle so you can see what you're setting
    const cx = centerX * scale;
    const cy = centerY * scale;
    const r = radius * scale;

    if (enabled) {
      // Darken outside circle using clip path inversion
      ctx.save();
      ctx.beginPath();
      ctx.rect(0, 0, canvas.width, canvas.height);
      ctx.arc(cx, cy, r, 0, 2 * Math.PI, true); // counter-clockwise = invert
      ctx.closePath();
      ctx.fillStyle = "rgba(0,0,0,0.6)";
      ctx.fill();
      ctx.restore();
    }

    // Circle outline — always visible
    ctx.strokeStyle = enabled ? "#00ff00" : "rgba(0,255,0,0.5)";
    ctx.lineWidth = 2;
    ctx.setLineDash(enabled ? [] : [8, 4]);
    ctx.beginPath();
    ctx.arc(cx, cy, r, 0, 2 * Math.PI);
    ctx.stroke();
    ctx.setLineDash([]);

    // Center crosshair
    ctx.strokeStyle = "rgba(255,0,0,0.7)";
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(cx - 10, cy);
    ctx.lineTo(cx + 10, cy);
    ctx.moveTo(cx, cy - 10);
    ctx.lineTo(cx, cy + 10);
    ctx.stroke();
  }, [centerX, centerY, radius, enabled]);

  // Load image when frame changes
  useEffect(() => {
    const frameId = status?.frame?.id;
    if (!frameId || !status?.frame?.jpegPath) return;

    const img = new Image();
    img.onload = () => {
      imgRef.current = img;
      setImgW(img.naturalWidth);
      setImgH(img.naturalHeight);

      const canvas = canvasRef.current;
      const container = containerRef.current;
      if (!canvas || !container) return;

      // Fit canvas to container width
      const maxW = container.clientWidth;
      const scale = maxW / img.naturalWidth;
      scaleRef.current = scale;
      canvas.width = Math.round(img.naturalWidth * scale);
      canvas.height = Math.round(img.naturalHeight * scale);
      draw();
    };
    img.src = `/api/frames/${frameId}/jpeg`;
  }, [status]);

  // Redraw when mask params change
  useEffect(() => { draw(); }, [draw]);

  // Mouse/touch handlers for dragging
  const getImageCoords = (e: React.MouseEvent | React.TouchEvent) => {
    const canvas = canvasRef.current;
    if (!canvas) return { x: 0, y: 0 };
    const rect = canvas.getBoundingClientRect();
    const clientX = "touches" in e ? e.touches[0].clientX : e.clientX;
    const clientY = "touches" in e ? e.touches[0].clientY : e.clientY;
    const scale = scaleRef.current;
    return {
      x: (clientX - rect.left) / scale,
      y: (clientY - rect.top) / scale,
    };
  };

  const handlePointerDown = (e: React.MouseEvent) => {
    const { x, y } = getImageCoords(e);
    const distToCenter = Math.sqrt((x - centerX) ** 2 + (y - centerY) ** 2);
    const edgeThreshold = 40 / scaleRef.current; // ~40 screen pixels

    if (Math.abs(distToCenter - radius) < edgeThreshold) {
      dragging.current = "edge";
    } else {
      dragging.current = "center";
    }
    e.preventDefault();
  };

  const handlePointerMove = (e: React.MouseEvent) => {
    if (!dragging.current) return;
    const { x, y } = getImageCoords(e);

    if (dragging.current === "center") {
      setCenterX(Math.round(Math.max(0, Math.min(imgW, x))));
      setCenterY(Math.round(Math.max(0, Math.min(imgH, y))));
    } else if (dragging.current === "edge") {
      const dist = Math.sqrt((x - centerX) ** 2 + (y - centerY) ** 2);
      setRadius(Math.round(Math.max(50, Math.min(Math.max(imgW, imgH) / 2, dist))));
    }
    e.preventDefault();
  };

  const handlePointerUp = () => {
    dragging.current = null;
  };

  async function save() {
    setSaving(true);
    await fetch("/api/config/imaging.mask", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
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
          <NumberInput label="Center X" value={centerX} onChange={v => setCenterX(Number(v))} min={0} max={imgW} />
          <NumberInput label="Center Y" value={centerY} onChange={v => setCenterY(Number(v))} min={0} max={imgH} />
          <NumberInput label="Radius" value={radius} onChange={v => setRadius(Number(v))} min={50} max={Math.max(imgW, imgH) / 2} />
        </Group>
        <Text size="xs" c="dimmed" mb="md">Click and drag on the image to move the center. Drag near the edge of the circle to resize.</Text>
        <Button onClick={save} loading={saving} fullWidth>Save Mask Settings</Button>
      </Card>
      <Card shadow="sm" padding="lg" withBorder>
        <Text fw={500} mb="sm">Preview</Text>
        <div ref={containerRef} style={{ margin: "0 auto" }}>
          <canvas
            ref={canvasRef}
            style={{ display: "block", cursor: dragging.current ? "grabbing" : "crosshair", borderRadius: 8, background: "#000" }}
            onMouseDown={handlePointerDown}
            onMouseMove={handlePointerMove}
            onMouseUp={handlePointerUp}
            onMouseLeave={handlePointerUp}
          />
        </div>
      </Card>
    </Stack>
  );
}
